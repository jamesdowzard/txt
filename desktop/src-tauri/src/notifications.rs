//! Native macOS notifications with inline reply for incoming messages.
//!
//! Flow:
//!   1. `init()` registers this process's bundle identifier with the macOS
//!      notification system. Called once at launch.
//!   2. `show_with_reply()` displays a notification and **blocks the current
//!      thread** until the user interacts. Callers must spawn this on a
//!      blocking task (see `sse::run`).
//!   3. On reply, the helper POSTs the reply to the Go backend's
//!      `/api/send` endpoint, reusing the same route the WebView uses.
//!
//! Why `mac-notification-sys` (NSUserNotification) over
//! `objc2-user-notifications` (UNUserNotificationCenter):
//!   - The latter needs a Rust-implemented ObjC delegate class via
//!     `declare_class!` to receive reply text, which is non-trivial.
//!   - NSUserNotification is deprecated on macOS 11+ but still works — its
//!     `hasReplyButton` → stdout round-trip maps cleanly onto our
//!     blocking-call-returns-reply-string model. Good enough for shipping;
//!     a follow-up PR can migrate to UN* when the delegate plumbing is worth
//!     the churn.

use mac_notification_sys::{
    send_notification, set_application, MainButton, Notification, NotificationResponse,
};
use serde::Serialize;

/// Call once at app start. Idempotent. If the bundle identifier can't be
/// resolved (e.g., running un-bundled during `cargo run`), notifications are
/// silently disabled — we must never crash the shell over a desktop-nicety.
pub fn init(bundle_id: &str) {
    if let Err(err) = set_application(bundle_id) {
        log::warn!("notifications: set_application({bundle_id}) failed: {err}");
    }
}

/// Show a notification with an inline reply field. **Blocks** the calling
/// thread until the user clicks, replies, or the notification times out. The
/// returned reply text (if any) should be POSTed to `/api/send`.
///
/// `title` is typically the sender/conversation display name; `body` is the
/// message text. `placeholder` shows in the reply input.
pub fn show_with_reply(title: &str, body: &str, placeholder: &str) -> Option<String> {
    let mut n = Notification::new();
    n.main_button(MainButton::Response(placeholder))
        .close_button("Dismiss")
        .wait_for_click(true);

    match send_notification(title, None, body, Some(&n)) {
        Ok(NotificationResponse::Reply(text)) => {
            let trimmed = text.trim();
            if trimmed.is_empty() {
                None
            } else {
                Some(trimmed.to_string())
            }
        }
        // ActionButton / CloseButton / Click / None — user saw it but didn't
        // reply. Nothing to post.
        Ok(_) => None,
        Err(err) => {
            log::warn!("notifications: send_notification failed: {err}");
            None
        }
    }
}

#[derive(Serialize)]
struct SendRequest<'a> {
    conversation_id: &'a str,
    message: &'a str,
}

/// POST the reply back to the Go backend's `/api/send` endpoint. The backend
/// resolves routing (SMS/RCS/platform) from the conversation record; we just
/// hand it {conversation_id, body}. Runs on the caller's task — short HTTP
/// round trip, no need to spawn further.
pub async fn send_reply(
    backend_origin: &str,
    conversation_id: &str,
    body: &str,
) -> Result<(), reqwest::Error> {
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(30))
        .build()?;
    let url = format!("{backend_origin}/api/send");
    let resp = client
        .post(&url)
        .json(&SendRequest {
            conversation_id,
            message: body,
        })
        .send()
        .await?;
    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await.unwrap_or_default();
        log::warn!("notifications: reply POST {url} returned {status}: {text}");
    }
    Ok(())
}
