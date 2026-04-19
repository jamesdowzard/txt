mod notifications;
mod sse;

use std::sync::{Arc, Mutex};
use std::time::Instant;
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Emitter, Manager, RunEvent};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};
use tauri_plugin_positioner::{Position, WindowExt};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

#[cfg(not(debug_assertions))]
use tauri_plugin_updater::UpdaterExt;

#[cfg(feature = "single-instance")]
use tauri_plugin_single_instance;

// Tauri event name used to deliver `textbridge://…` URLs to the WebView.
// Matches the subscriber wired in web/src/legacy.js.
const DEEP_LINK_EVENT: &str = "textbridge://deep-link";
const COMPOSE_HOTKEY_EVENT: &str = "textbridge://focus-compose";
const BACKEND_ORIGIN: &str = "http://127.0.0.1:7007";
const BUNDLE_ID: &str = "ai.james-is-an.textbridge";

struct BackendChild(Mutex<Option<CommandChild>>);

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_positioner::init())
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_handler(|app, shortcut, event| {
                    if event.state() == ShortcutState::Pressed
                        && shortcut.matches(Modifiers::ALT | Modifiers::SHIFT, Code::KeyT)
                    {
                        if let Some(main) = app.get_webview_window("main") {
                            let _ = main.show();
                            let _ = main.unminimize();
                            let _ = main.set_focus();
                        }
                        let _ = app.emit_to("main", COMPOSE_HOTKEY_EVENT, ());
                    }
                })
                .build(),
        );

    // On Linux/Windows the OS re-launches the app with the URL as argv[1];
    // on macOS it fires the NSAppDelegate URL event which the deep-link
    // plugin already handles. Forward any URL-shaped argv entry to the
    // running instance so the behaviour is consistent across platforms.
    // Matches both `txt://` (primary) and `textbridge://` (legacy).
    #[cfg(feature = "single-instance")]
    let builder = builder.plugin(tauri_plugin_single_instance::init(|app, args, _cwd| {
        if let Some(url) = args
            .iter()
            .find(|a| a.starts_with("txt://") || a.starts_with("textbridge://"))
        {
            dispatch_deep_link(app, url.clone());
        } else if let Some(main) = app.get_webview_window("main") {
            let _ = main.set_focus();
        }
    }));

    builder
        .manage(BackendChild(Mutex::new(None)))
        .setup(|app| {
            let data_dir = app
                .path()
                .app_data_dir()
                .expect("app data dir resolvable");
            std::fs::create_dir_all(&data_dir).ok();

            let sidecar = app
                .shell()
                .sidecar("textbridge-backend")
                .expect("sidecar textbridge-backend missing")
                .args(["serve"])
                .env("OPENMESSAGES_DATA_DIR", data_dir.to_string_lossy().to_string())
                .env("OPENMESSAGES_PORT", "7007")
                .env("OPENMESSAGES_LOG_LEVEL", "info")
                // Tauri shell subscribes to the SSE bus and shows native
                // notifications with inline reply (see sse.rs). Disable the
                // Go-side terminal-notifier path so we don't double-notify.
                .env("OPENMESSAGES_MACOS_NOTIFICATIONS", "0")
                .env("TEXTBRIDGE_GOOGLE_ONLY", "1");

            let (mut rx, child) = sidecar.spawn().expect("spawn backend sidecar");

            if let Some(state) = app.try_state::<BackendChild>() {
                *state.0.lock().unwrap() = Some(child);
            }

            tauri::async_runtime::spawn(async move {
                while let Some(event) = rx.recv().await {
                    match event {
                        CommandEvent::Stdout(line) | CommandEvent::Stderr(line) => {
                            eprintln!("[backend] {}", String::from_utf8_lossy(&line));
                        }
                        CommandEvent::Terminated(_) => break,
                        _ => {}
                    }
                }
            });

            // Forward txt://… / textbridge://… URLs to either the background
            // auto-send path (compose with both `to` + `body`) or the WebView
            // (everything else — opens overlay, focuses window). Fires on
            // every macOS NSAppDelegate "open URLs" event (cold + hot).
            let deep_link_handle = app.handle().clone();
            app.deep_link().on_open_url(move |event| {
                for url in event.urls() {
                    dispatch_deep_link(&deep_link_handle, url.to_string());
                }
            });

            // Global compose hotkey (P3 #32). ⌥⇧T focuses the main window and
            // emits `textbridge://focus-compose` for the WebView to handle.
            let shortcut = Shortcut::new(Some(Modifiers::ALT | Modifiers::SHIFT), Code::KeyT);
            if let Err(err) = app.global_shortcut().register(shortcut) {
                eprintln!("[global-shortcut] failed to register compose hotkey: {err}");
            }

            // Native-notifications pipeline (P1 #9). Registers the bundle
            // with NSUserNotificationCenter and spawns a background SSE
            // subscriber that shows a notification per inbound message with
            // an inline reply action.
            notifications::init(BUNDLE_ID);
            tauri::async_runtime::spawn(async move {
                sse::run(BACKEND_ORIGIN.to_string()).await;
            });

            // Menu-bar popover (#20 item 1 / #15 #5b). NSStatusItem tray
            // with unread-count title; click opens a 320×480 popover window
            // listing VIPs + a quick-reply composer. Non-fatal if it fails —
            // the main window still functions without a tray.
            if let Err(err) = setup_tray(app.handle()) {
                eprintln!("[tray] setup failed: {err}");
            }

            // Auto-update check at launch. Skip in dev (debug) builds —
            // updater pubkey is a placeholder until the user runs
            // `tauri signer generate` once. The release build embeds the
            // real key via tauri.conf.json + TAURI_SIGNING_PRIVATE_KEY.
            #[cfg(not(debug_assertions))]
            {
                let handle = app.handle().clone();
                tauri::async_runtime::spawn(async move {
                    if let Err(err) = check_for_update(handle).await {
                        eprintln!("[updater] {err}");
                    }
                });
            }

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![open_conversation_window])
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app_handle, event| {
            if let RunEvent::ExitRequested { .. } | RunEvent::Exit = event {
                if let Some(state) = app_handle.try_state::<BackendChild>() {
                    if let Some(child) = state.0.lock().unwrap().take() {
                        let _ = child.kill();
                    }
                }
            }
        });
}

/// Open (or focus) a detached window showing a single conversation (P2 #22).
///
/// Labels are derived from the conversation ID with non-alphanumerics stripped
/// so Tauri's window-label validator accepts them. If a detached window for the
/// same conversation already exists, we show/unminimize/focus it instead of
/// spawning a duplicate.
#[tauri::command]
fn open_conversation_window(
    app: tauri::AppHandle,
    conversation_id: String,
) -> Result<(), String> {
    let label = format!(
        "detached-{}",
        conversation_id
            .bytes()
            .map(|b| format!("{b:02x}"))
            .collect::<String>()
    );

    if let Some(existing) = app.get_webview_window(&label) {
        existing.show().map_err(|e| e.to_string())?;
        existing.unminimize().map_err(|e| e.to_string())?;
        existing.set_focus().map_err(|e| e.to_string())?;
        return Ok(());
    }

    let url = format!(
        "{}/?mode=detached&conversation={}",
        BACKEND_ORIGIN,
        urlencoding::encode(&conversation_id),
    );

    tauri::WebviewWindowBuilder::new(
        &app,
        &label,
        tauri::WebviewUrl::External(url.parse().map_err(|e: url::ParseError| e.to_string())?),
    )
    .title("Textbridge — Conversation")
    .inner_size(640.0, 760.0)
    .min_inner_size(480.0, 500.0)
    .resizable(true)
    .build()
    .map_err(|e| e.to_string())?;

    Ok(())
}

/// Route a `txt://…` or `textbridge://…` URL to either the background
/// auto-send path or the WebView. A `compose` URL with both `to` and `body`
/// is treated as "send this now" and bypasses the WebView entirely — the app
/// window is NOT brought to the front (Shortcuts-style background send).
/// Everything else is emitted to the WebView, which opens the compose
/// overlay and focuses the window.
fn dispatch_deep_link(app: &AppHandle, raw: String) {
    let parsed = match url::Url::parse(&raw) {
        Ok(u) => u,
        Err(err) => {
            eprintln!("[deep-link] invalid URL {raw}: {err}");
            return;
        }
    };

    let action = parsed
        .host_str()
        .map(|s| s.to_ascii_lowercase())
        .unwrap_or_default();

    if action == "compose" {
        let mut to = String::new();
        let mut body = String::new();
        for (k, v) in parsed.query_pairs() {
            match k.as_ref() {
                "to" => to = v.into_owned(),
                "body" => body = v.into_owned(),
                _ => {}
            }
        }
        if !to.is_empty() && !body.is_empty() {
            // Background send — don't steal focus. On failure, fall back
            // to the WebView path so the user still gets the overlay with
            // their inputs prefilled and can retry.
            let fallback = app.clone();
            let raw_for_fallback = raw.clone();
            tauri::async_runtime::spawn(async move {
                if let Err(err) = send_via_handle(&to, &body).await {
                    eprintln!(
                        "[deep-link] compose send failed ({err}); falling back to WebView overlay"
                    );
                    if let Some(main) = fallback.get_webview_window("main") {
                        let _ = main.show();
                        let _ = main.unminimize();
                        let _ = main.set_focus();
                    }
                    let _ = fallback.emit(DEEP_LINK_EVENT, raw_for_fallback);
                }
            });
            return;
        }
    }

    // Default: focus the window and hand off to the WebView.
    if let Some(main) = app.get_webview_window("main") {
        let _ = main.show();
        let _ = main.unminimize();
        let _ = main.set_focus();
    }
    let _ = app.emit(DEEP_LINK_EVENT, raw);
}

/// Resolve a recipient handle to a conversation ID and POST the body to
/// `/api/send`. For `@`-shaped handles we assume iMessage (`imessage:<handle>`);
/// for everything else we call `/api/new-conversation` to get-or-create a
/// Google Messages conversation. Errors bubble up to the caller (logged).
async fn send_via_handle(to: &str, body: &str) -> Result<(), String> {
    let handle = to.trim();
    if handle.is_empty() {
        return Err("empty recipient handle".to_string());
    }

    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(15))
        .build()
        .map_err(|e| format!("build http client: {e}"))?;

    // Resolve handle → conversation_id.
    let conversation_id = if handle.contains('@') {
        // Email → iMessage. The Go backend keys iMessage conversations by
        // `imessage:<handle>` directly.
        format!("imessage:{handle}")
    } else {
        let digits_only = handle
            .chars()
            .filter(|c| c.is_ascii_digit() || *c == '+')
            .collect::<String>();
        let is_phone = !digits_only.is_empty()
            && digits_only
                .chars()
                .filter(|c| c.is_ascii_digit())
                .count()
                >= 5;

        if !is_phone {
            return Err(format!("unrecognised handle shape: {handle}"));
        }

        // Phone → get-or-create a Google Messages conversation via the
        // existing resolver. This is the same endpoint the New Message
        // overlay in the WebView uses.
        let url = format!("{}/api/new-conversation", BACKEND_ORIGIN);
        let resp = client
            .post(&url)
            .json(&serde_json::json!({ "phone_number": handle }))
            .send()
            .await
            .map_err(|e| format!("POST /api/new-conversation: {e}"))?;

        if !resp.status().is_success() {
            let status = resp.status();
            let text = resp.text().await.unwrap_or_default();
            return Err(format!(
                "/api/new-conversation returned {status}: {text}"
            ));
        }

        let parsed: serde_json::Value = resp
            .json()
            .await
            .map_err(|e| format!("parse /api/new-conversation response: {e}"))?;

        parsed
            .get("conversation_id")
            .and_then(|v| v.as_str())
            .map(|s| s.to_string())
            .ok_or_else(|| {
                format!(
                    "/api/new-conversation returned no conversation_id (got {parsed})"
                )
            })?
    };

    // POST the body to /api/send.
    let url = format!("{}/api/send", BACKEND_ORIGIN);
    let resp = client
        .post(&url)
        .json(&serde_json::json!({
            "conversation_id": conversation_id,
            "message": body,
        }))
        .send()
        .await
        .map_err(|e| format!("POST /api/send: {e}"))?;

    if !resp.status().is_success() {
        let status = resp.status();
        let text = resp.text().await.unwrap_or_default();
        return Err(format!("/api/send returned {status}: {text}"));
    }

    eprintln!(
        "[deep-link] compose sent: conversation_id={conversation_id} (handle={handle})"
    );
    Ok(())
}

/// Check the configured updater endpoint for a newer release. If one is
/// available, download it (verifying the signature against the embedded
/// pubkey), install it, and relaunch the app.
///
/// The check runs once on launch. Failures are logged and swallowed —
/// users without internet, or a 404 from a release without `latest.json`,
/// must not block the app from starting.
#[cfg(not(debug_assertions))]
async fn check_for_update(app: AppHandle) -> tauri_plugin_updater::Result<()> {
    let updater = app.updater()?;
    let Some(update) = updater.check().await? else {
        return Ok(());
    };

    eprintln!(
        "[updater] update available: {} (current {})",
        update.version, update.current_version
    );

    let mut downloaded: usize = 0;
    update
        .download_and_install(
            |chunk_length, content_length| {
                downloaded += chunk_length;
                if let Some(total) = content_length {
                    eprintln!("[updater] downloaded {downloaded}/{total} bytes");
                }
            },
            || {
                eprintln!("[updater] download finished, installing");
            },
        )
        .await?;

    eprintln!("[updater] installed update; relaunching");
    app.restart();
}

/// Register the `NSStatusItem` tray and wire tray-click → popover window.
/// Also spawns a background poller that refreshes the tray title with the
/// total unread count every 30 s. Non-fatal — errors log and propagate so
/// the caller can decide whether to continue without a tray.
fn setup_tray(app: &AppHandle) -> tauri::Result<()> {
    let last_hidden: Arc<Mutex<Option<Instant>>> = Arc::new(Mutex::new(None));
    let lh_tray = last_hidden.clone();

    TrayIconBuilder::with_id("main-tray")
        .icon(
            app.default_window_icon()
                .cloned()
                .expect("default window icon configured in tauri.conf.json"),
        )
        .icon_as_template(true)
        .on_tray_icon_event(move |tray, event| {
            tauri_plugin_positioner::on_tray_event(tray.app_handle(), &event);
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                // Guard against the tray-click that just caused the popover
                // to blur re-opening it again. Window-focus-loss records an
                // Instant; any tray click within 250 ms is treated as the
                // dismiss-click and dropped.
                let recently_hidden = lh_tray
                    .lock()
                    .ok()
                    .and_then(|g| *g)
                    .map(|t| t.elapsed().as_millis() < 250)
                    .unwrap_or(false);
                if recently_hidden {
                    return;
                }
                toggle_popover(tray.app_handle(), &lh_tray);
            }
        })
        .build(app)?;

    spawn_unread_poller(app.clone());
    Ok(())
}

/// Show-or-hide the popover window, creating it lazily on first use.
/// Eager creation in `setup()` races the Go sidecar's bind on port 7007
/// and the WebView navigates to a refused connection.
fn toggle_popover(app: &AppHandle, last_hidden: &Arc<Mutex<Option<Instant>>>) {
    if let Some(popover) = app.get_webview_window("popover") {
        if popover.is_visible().unwrap_or(false) {
            let _ = popover.hide();
        } else {
            let _ = popover.move_window(Position::TrayCenter);
            let _ = popover.show();
            let _ = popover.set_focus();
        }
        return;
    }

    let url = format!("{}/popover.html", BACKEND_ORIGIN);
    let parsed = match url.parse() {
        Ok(u) => u,
        Err(err) => {
            eprintln!("[popover] invalid URL {url}: {err}");
            return;
        }
    };
    let popover = match tauri::WebviewWindowBuilder::new(
        app,
        "popover",
        tauri::WebviewUrl::External(parsed),
    )
    .title("txt")
    .inner_size(320.0, 480.0)
    .resizable(false)
    .decorations(false)
    .always_on_top(true)
    .skip_taskbar(true)
    .focused(true)
    .visible(false)
    .build()
    {
        Ok(w) => w,
        Err(err) => {
            eprintln!("[popover] failed to create window: {err}");
            return;
        }
    };

    // Click-outside-dismiss: hide whenever the popover loses focus, and
    // stamp the hide time so the tray-click debounce can distinguish the
    // dismiss-click from a fresh re-open click.
    let lh = last_hidden.clone();
    let pop = popover.clone();
    popover.on_window_event(move |event| {
        if let tauri::WindowEvent::Focused(false) = event {
            if let Ok(mut slot) = lh.lock() {
                *slot = Some(Instant::now());
            }
            let _ = pop.hide();
        }
    });

    let _ = popover.move_window(Position::TrayCenter);
    let _ = popover.show();
    let _ = popover.set_focus();
}

/// Poll `/api/conversations` every 30 s and update the tray title with the
/// total unread count. Empty title when zero so the menu-bar icon stays
/// clean. Poll failures are silent — the tray is a nice-to-have, not a
/// critical path.
fn spawn_unread_poller(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        let client = match reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(5))
            .build()
        {
            Ok(c) => c,
            Err(err) => {
                eprintln!("[tray-poller] build http client: {err}");
                return;
            }
        };
        // Let the Go sidecar bind :7007 before the first poll.
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
        loop {
            if let Ok(resp) = client
                .get(format!("{BACKEND_ORIGIN}/api/conversations?limit=500"))
                .send()
                .await
            {
                if let Ok(convos) = resp.json::<serde_json::Value>().await {
                    let total: i64 = convos
                        .as_array()
                        .map(|arr| {
                            arr.iter()
                                .filter_map(|c| {
                                    c.get("UnreadCount").and_then(|v| v.as_i64())
                                })
                                .sum()
                        })
                        .unwrap_or(0);
                    if let Some(tray) = app.tray_by_id("main-tray") {
                        let title = if total > 0 {
                            Some(total.to_string())
                        } else {
                            None
                        };
                        let _ = tray.set_title(title);
                    }
                }
            }
            tokio::time::sleep(std::time::Duration::from_secs(30)).await;
        }
    });
}

