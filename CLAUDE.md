# Textbridge

Native Tauri macOS app wrapping a Go backend (libgm) for reading and sending Google Messages from the Mac. Forked from [MaxGhenis/openmessage](https://github.com/MaxGhenis/openmessage) and retargeted for Google Messages only.

## Architecture

```
textbridge/
├── main.go                # Go entrypoint (CLI: pair, serve, send, import)
├── cmd/                   # Go command implementations
├── internal/
│   ├── app/               # Bootstrap, data dir, backfill
│   ├── client/            # libgm Google Messages protocol
│   ├── db/                # SQLite store (conversations, messages, contacts, drafts)
│   ├── tools/             # MCP tools
│   └── web/static/        # Embedded web UI (single HTML file loaded in WebView)
├── desktop/               # Tauri 2 macOS app
│   ├── src-tauri/         # Rust shell: spawns sidecar, owns window
│   │   ├── src/lib.rs     # Sidecar lifecycle
│   │   └── binaries/      # Go backend binary (gitignored)
│   ├── src/               # TypeScript loader (image-probe then navigate to backend)
│   └── scripts/           # build-sidecar, dev, release
└── site/                  # (legacy) upstream's marketing site — not used here
```

## Build + deploy

```bash
# Go backend (produces ./textbridge)
go build -o textbridge .

# Tauri app
cd desktop
./scripts/build-sidecar      # required before cargo tauri build
./scripts/release patch      # build, sign, install to /Applications/Textbridge.app
./scripts/dev --launch       # orange dev variant alongside stable
```

Signing: `Developer ID Application: James Dowzard (G54DLMPV94)`. Bundle ID: `ai.james-is-an.textbridge`.

## Data locations

| Path | Contents |
|------|----------|
| `~/Library/Application Support/ai.james-is-an.textbridge/` | Active app data (session, messages.db) |
| `~/.local/share/openmessage/` | Legacy location (`./textbridge pair` writes here) |

The Tauri shell passes `OPENMESSAGES_DATA_DIR` → Go sidecar, pointing at the first path.

## Known issues

- **`cargo tauri build` ends with xattr error on macOS 26+** — bundle IS created; the `scripts/dev` and `scripts/release` scripts grep-filter the error. Just use those.
- **WebView CORS blocks `fetch()` to localhost backend** — frontend probes readiness via `<img>` load (bypasses CORS), then `window.location.replace()` navigates.
- **Sandbox is disabled in `entitlements.plist`** — enabling it breaks the Go sidecar subprocess spawn.
- **Universal `lipo` binary silently crashes at runtime** — we ship arm64-only via `build-sidecar`, which uses `rustc -vV` host to pick the right GOARCH.

## MCP endpoints (optional)

Backend exposes MCP at `http://127.0.0.1:7007/mcp/sse` and stdio transport when launched by an MCP client. Useful for hooking Claude Code into your messages (search, stats, etc). The upstream openmessage tools are still compiled in but WhatsApp/Signal/gchat imports are effectively dormant (UI hidden, session files absent).

## Testing

```bash
go test ./cmd/ -v
go test ./... -v
```

## Upstream sync

Upstream is `MaxGhenis/openmessage` (fetched as remote `upstream`). To pull upstream fixes:

```bash
git fetch upstream
git merge upstream/main   # expect conflicts on README, CLAUDE.md, internal/web/static/index.html
```

## Key files

- `main.go`, `cmd/pair.go`, `cmd/serve.go` — Go entrypoints
- `internal/web/static/index.html` — web UI (frosted-dark CSS tokens at `:root`, WhatsApp/Signal hidden)
- `desktop/src-tauri/src/lib.rs` — Tauri setup, sidecar lifecycle
- `desktop/src-tauri/tauri.conf.json` — bundle config, `externalBin` sidecar reference
- `desktop/src-tauri/capabilities/default.json` — Tauri permissions (shell:allow-execute, shell:allow-spawn)
