use std::sync::Mutex;
use tauri::{Emitter, Manager, RunEvent};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

#[cfg(not(debug_assertions))]
use tauri::AppHandle;
#[cfg(not(debug_assertions))]
use tauri_plugin_updater::UpdaterExt;

#[cfg(feature = "single-instance")]
use tauri_plugin_single_instance;

// Tauri event name used to deliver `textbridge://…` URLs to the WebView.
// Matches the subscriber wired in web/src/legacy.js.
const DEEP_LINK_EVENT: &str = "textbridge://deep-link";

struct BackendChild(Mutex<Option<CommandChild>>);

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_deep_link::init());

    // On Linux/Windows the OS re-launches the app with the URL as argv[1];
    // on macOS it fires the NSAppDelegate URL event which the deep-link
    // plugin already handles. Forward any URL-shaped argv entry to the
    // running instance so the behaviour is consistent across platforms.
    #[cfg(feature = "single-instance")]
    let builder = builder.plugin(tauri_plugin_single_instance::init(|app, args, _cwd| {
        if let Some(url) = args.iter().find(|a| a.starts_with("textbridge://")) {
            let _ = app.emit(DEEP_LINK_EVENT, url.clone());
        }
        if let Some(main) = app.get_webview_window("main") {
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
                .env("OPENMESSAGES_MACOS_NOTIFICATIONS", "1")
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

            // Forward textbridge://… URLs to the WebView. Fires on every
            // macOS NSAppDelegate "open URLs" event (cold launch + hot).
            let deep_link_handle = app.handle().clone();
            app.deep_link().on_open_url(move |event| {
                for url in event.urls() {
                    let _ = deep_link_handle.emit(DEEP_LINK_EVENT, url.to_string());
                }
            });

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

