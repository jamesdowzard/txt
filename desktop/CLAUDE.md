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

## Auto-update

`tauri-plugin-updater` is registered in `src-tauri/src/lib.rs` and runs once on
launch in release builds (skipped in `cargo tauri dev`). Endpoint + pubkey live
in `tauri.conf.json` under `plugins.updater`.

**One-time key setup** (run once, not per release):

```bash
PW="$(openssl rand -base64 24)"
mkdir -p ~/.tauri
cargo tauri signer generate --ci -p "$PW" -w ~/.tauri/textbridge-updater.key

security add-generic-password -a textbridge-updater -s textbridge-updater \
  -w "$(cat ~/.tauri/textbridge-updater.key)"
security add-generic-password -a textbridge-updater-pub -s textbridge-updater-pub \
  -w "$(cat ~/.tauri/textbridge-updater.key.pub)"
security add-generic-password -a textbridge-updater-password -s textbridge-updater-password \
  -w "$PW"

# Paste the pubkey into tauri.conf.json (replacing the placeholder):
./scripts/updater-key pubkey
```

Then move `~/.tauri/textbridge-updater.key{,.pub}` + `$PW` to Vaultwarden (or
copy to `~/.credentials/`) and delete from `~/.tauri/` — the keychain copies
are the source of truth at build time. `./scripts/updater-key check` verifies
all three entries are present.

**Per-release** (handled by `./scripts/release`):

1. `./scripts/updater-key check` — gates the build; errors with setup
   instructions if the key isn't in keychain.
2. `TAURI_SIGNING_PRIVATE_KEY=$(./scripts/updater-key export)` exported for the build.
3. After codesign + notarize + staple, the script re-tars the final stapled
   `.app` into `Textbridge.app.tar.gz`, signs it (`tauri signer sign`), and
   writes `latest.json` referencing both `darwin-aarch64` + `darwin-x86_64`
   (the universal build is one artifact for both arches).
4. Upload the three files (`Textbridge.app.tar.gz`, `Textbridge.app.tar.gz.sig`,
   `latest.json`) to the GitHub Release the script tagged as `v$NEW_VERSION`.

The updater endpoint in `tauri.conf.json` points at
`https://github.com/jamesdowzard/txt/releases/latest/download/latest.json`,
so installed apps automatically pick up whatever release is marked "latest" on
GitHub.

Skip the updater pipeline (no `latest.json`, build still signs the bundle) with
`SKIP_UPDATER=1 ./scripts/release patch`.

## Notarization

`./scripts/release` notarizes + staples after signing when the keychain profile `Textbridge` is present. One-time setup:

```bash
xcrun notarytool store-credentials Textbridge \
  --apple-id <your apple id> \
  --team-id G54DLMPV94 \
  --password <app-specific password from appleid.apple.com>
```

Override profile name with `NOTARY_KEYCHAIN_PROFILE=other`. Skip entirely with `SKIP_NOTARIZE=1` for fast dev iterations. Verify after release with `spctl -a -vv /Applications/Textbridge.app` — expect `accepted, source=Notarized Developer ID`.

## Version-bump commit

The harness pre-commit hook blocks direct commits to `main` / `develop`. When `./scripts/release` is invoked on a protected branch it auto-opens a `release/v<NEW_VERSION>` branch, commits the `tauri.conf.json` bump there, pushes, and `gh pr merge --admin --squash --delete-branch` routes it back to `main`. The script then pulls the squash-merge and tags `v<NEW_VERSION>` locally + remotely. Requires `gh` authenticated as `jamesdowzard`.

Run from a feature branch instead and the script commits + pushes to that branch (no PR), assuming the caller owns the merge.

## macOS Shortcuts / automation

The `textbridge://` URL scheme is the Shortcuts entrypoint. Recipes and
action reference live in `../docs/shortcuts.md`. Handled in
`desktop/src-tauri/src/lib.rs::run` via `tauri_plugin_deep_link`; the
WebView side parses the URL in `web/src/legacy.js::handleDeepLink`.

Supported actions today: `compose`, `search`, `conversation/<id>`, `open`.
Adding a new action is two edits — the `handleDeepLink` switch statement
+ a row in `docs/shortcuts.md`.

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
- `../web/` — Vite + Preact source for the UI (built into `../internal/web/static/dist/` and embedded into the Go binary)
