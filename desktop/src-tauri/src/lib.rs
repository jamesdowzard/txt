use std::sync::Mutex;
use tauri::{Manager, RunEvent};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

#[cfg(feature = "single-instance")]
use tauri_plugin_single_instance;

struct BackendChild(Mutex<Option<CommandChild>>);

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default()
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_shell::init());

    #[cfg(feature = "single-instance")]
    let builder = builder.plugin(tauri_plugin_single_instance::init(|_app, _args, _cwd| {}));

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
                .env("OPENMESSAGES_MACOS_NOTIFICATIONS", "1");

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
