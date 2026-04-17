//! SSE client that listens to the Go backend's `/api/events` stream and
//! shows a native notification (with inline reply) for each
//! `incoming_message` event.
//!
//! The Tauri shell starts the sidecar with
//! `OPENMESSAGES_MACOS_NOTIFICATIONS=0`, so the Go-side `terminal-notifier`
//! path is a no-op in this process tree. All notifications flow through
//! here instead.

use std::time::Duration;

use futures_util::StreamExt;
use serde::Deserialize;

use crate::notifications;

const RECONNECT_DELAY: Duration = Duration::from_secs(3);
const NOTIFY_PLACEHOLDER: &str = "Reply…";

/// Minimal subset of the Go `web.StreamEvent` struct needed for
/// `EventTypeIncomingMessage`. Unknown fields are ignored — every other
/// stream event type (messages/conversations/status/typing/heartbeat)
/// deserialises to `ty` only and we bail early.
#[derive(Debug, Deserialize)]
#[serde(rename_all = "snake_case")]
struct IncomingEvent {
    #[serde(rename = "type")]
    ty: String,
    conversation_id: Option<String>,
    message_id: Option<String>,
    sender_name: Option<String>,
    sender_number: Option<String>,
    body: Option<String>,
    conversation_name: Option<String>,
    notification_mode: Option<String>,
}

/// Run the SSE loop forever. Reconnects with a short delay on any error so a
/// backend restart or transient network blip doesn't leave the shell silent.
/// Spawned once from `lib.rs::run` after the sidecar is up.
pub async fn run(backend_origin: String) {
    loop {
        if let Err(err) = stream_once(&backend_origin).await {
            log::warn!("sse: stream ended: {err}; reconnecting in 3s");
        }
        tokio::time::sleep(RECONNECT_DELAY).await;
    }
}

async fn stream_once(backend_origin: &str) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let url = format!("{backend_origin}/api/events");
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(0)) // 0 = no overall timeout; long-poll
        .build()?;
    let resp = client
        .get(&url)
        .header("Accept", "text/event-stream")
        .send()
        .await?;
    if !resp.status().is_success() {
        return Err(format!("sse: {url} returned {}", resp.status()).into());
    }

    let mut buf = String::new();
    let mut stream = resp.bytes_stream();
    while let Some(chunk) = stream.next().await {
        let bytes = chunk?;
        buf.push_str(&String::from_utf8_lossy(&bytes));

        // SSE frames are delimited by a blank line (double \n). Parse each
        // completed frame and leave any partial tail in the buffer.
        while let Some(idx) = buf.find("\n\n") {
            let frame: String = buf.drain(..idx + 2).collect();
            if let Some(data) = extract_data(&frame) {
                handle_data(backend_origin, &data);
            }
        }
    }
    Ok(())
}

fn extract_data(frame: &str) -> Option<String> {
    let mut data_lines = Vec::new();
    for line in frame.lines() {
        if let Some(rest) = line.strip_prefix("data:") {
            data_lines.push(rest.trim_start());
        }
    }
    if data_lines.is_empty() {
        None
    } else {
        Some(data_lines.join("\n"))
    }
}

fn handle_data(backend_origin: &str, data: &str) {
    let evt: IncomingEvent = match serde_json::from_str(data) {
        Ok(v) => v,
        Err(_) => return,
    };
    if evt.ty != "incoming_message" {
        return;
    }
    if matches!(evt.notification_mode.as_deref(), Some("muted")) {
        return;
    }

    let conversation_id = match evt.conversation_id {
        Some(v) if !v.is_empty() => v,
        _ => return,
    };
    log::debug!(
        "sse: incoming_message conv={conversation_id} msg={}",
        evt.message_id.as_deref().unwrap_or("<?>")
    );
    let title = evt
        .conversation_name
        .clone()
        .filter(|s| !s.is_empty())
        .or_else(|| evt.sender_name.clone().filter(|s| !s.is_empty()))
        .or_else(|| evt.sender_number.clone().filter(|s| !s.is_empty()))
        .unwrap_or_else(|| "New message".to_string());
    let body = evt.body.clone().unwrap_or_default();
    let body = if body.is_empty() {
        "New message".to_string()
    } else {
        body
    };

    let origin = backend_origin.to_string();
    // send_with_reply blocks until the user interacts; keep it off the main
    // tokio reactor. spawn_blocking is sufficient — these are short-lived
    // per notification.
    tokio::task::spawn_blocking(move || {
        if let Some(reply) = notifications::show_with_reply(&title, &body, NOTIFY_PLACEHOLDER) {
            // Hand the reply back to the backend on the runtime. Spawn a new
            // blocking-to-async trampoline by building a tiny runtime — the
            // alternative (using tokio::runtime::Handle::current()) doesn't
            // work inside spawn_blocking when the outer runtime shuts down.
            let rt = match tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
            {
                Ok(rt) => rt,
                Err(err) => {
                    log::warn!("sse: tokio runtime for reply POST failed: {err}");
                    return;
                }
            };
            rt.block_on(async move {
                if let Err(err) = notifications::send_reply(&origin, &conversation_id, &reply).await
                {
                    log::warn!("sse: reply POST failed: {err}");
                }
            });
        }
    });
}
