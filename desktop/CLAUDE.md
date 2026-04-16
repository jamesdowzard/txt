# Textbridge — Tauri Desktop App

Native macOS wrapper around the Go backend. Tauri 2 + TypeScript + Rust. The Rust side spawns `textbridge-backend` (the Go binary) as a sidecar subprocess and the WebView loads its web UI at `http://127.0.0.1:7007`.

## Quick Start

```bash
# 1. Build the Go sidecar (both arm64 + x86_64; set BUILD_SIDECAR_HOST_ONLY=1 for host-only)
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

## Notarization

`./scripts/release` notarizes + staples after signing when the keychain profile `Textbridge` is present. One-time setup:

```bash
xcrun notarytool store-credentials Textbridge \
  --apple-id <your apple id> \
  --team-id G54DLMPV94 \
  --password <app-specific password from appleid.apple.com>
```

Override profile name with `NOTARY_KEYCHAIN_PROFILE=other`. Skip entirely with `SKIP_NOTARIZE=1` for fast dev iterations. Verify after release with `spctl -a -vv /Applications/Textbridge.app` — expect `accepted, source=Notarized Developer ID`.

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
