# Textbridge — Tauri Desktop App

Native macOS wrapper around the Go backend. Tauri 2 + TypeScript + Rust. The Rust side spawns `textbridge-backend` (the Go binary) as a sidecar subprocess and the WebView loads its web UI at `http://127.0.0.1:7007`.

## Quick Start

```bash
# 1. Build the Go sidecar (once, or when Go code changes)
./scripts/build-sidecar

# 2. Dev loop (hot reload frontend, backend sidecar running)
cargo tauri dev

# 3. Deploy to /Applications/Textbridge-dev.app
./scripts/dev --launch

# 4. Release to /Applications/Textbridge.app
./scripts/release patch
```

## Architecture

```
┌─────────────────────────────────────┐
│ Textbridge.app                      │
│ ├── MacOS/textbridge (Rust/Tauri)   │  ← Window, menu bar, lifecycle
│ │     │ spawns on startup           │
│ │     ▼                             │
│ ├── MacOS/textbridge-backend (Go)   │  ← libgm, SQLite, MCP, HTTP server
│ │     │ listens on 127.0.0.1:7007   │
│ │     ▼                             │
│ └── WKWebView → http://127.0.0.1:7007
└─────────────────────────────────────┘
```

## Data locations

| Path | Contents |
|------|----------|
| `~/Library/Application Support/ai.james-is-an.textbridge/` | Session, messages.db (Tauri default) |
| `~/.local/share/openmessage/` | Legacy location from `openmessage pair` CLI |

The Rust side sets `OPENMESSAGES_DATA_DIR` env var to the first path before launching the sidecar.

## Gotchas

- **First build needs the sidecar** — `cargo tauri build` fails if `src-tauri/binaries/textbridge-backend-<triple>` doesn't exist. Run `./scripts/build-sidecar` first.
- **xattr bundling error on Tahoe** — `cargo tauri build` ends with "failed to run xattr" on macOS 26+. The bundle IS created before the error; just ignore or use `./scripts/release` which handles it.
- **Backend probe uses image load, not fetch** — WebView CORS blocks `fetch()` to `127.0.0.1:7007` from the Tauri origin. We probe via `<img src=/favicon.svg>` which bypasses CORS, then `window.location.replace()` navigates to the backend.
- **CSP is `null`** — allows loading localhost content. Tighten if you stop loading from the backend.

## Code signing

- Debug: ad-hoc
- Release: `Developer ID Application: James Dowzard (G54DLMPV94)` (via `./scripts/release`)

## Adding Rust commands

```rust
// src-tauri/src/lib.rs
#[tauri::command]
fn my_command(arg: &str) -> String {
    format!("Got: {}", arg)
}

// Register in run():
.invoke_handler(tauri::generate_handler![my_command])
```

```typescript
// src/main.ts
import { invoke } from "@tauri-apps/api/core";
const result = await invoke<string>("my_command", { arg: "hello" });
```

## See Also

- `../` — Go backend (libgm, MCP, web UI)
- `../internal/web/static/index.html` — the styled web UI loaded inside the WebView
