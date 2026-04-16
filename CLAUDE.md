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

## Google-only mode

The Tauri shell launches the sidecar with `TEXTBRIDGE_GOOGLE_ONLY=1`, which skips the WhatsApp + Signal live-bridge connects and the WhatsApp Native / Signal Desktop importers. iMessage sync still runs. CLI `./textbridge serve` without the flag retains upstream multi-platform behaviour.

**One-off cleanup** after first upgrade: any WhatsApp/Signal conversations previously imported still live in `~/Library/Application Support/ai.james-is-an.textbridge/messages.db`. Move the file to Trash and re-pair Google Messages for a clean list. Bridge-era session leftovers (`signal-cli/`, `whatsapp-session.db`) and the 75 MB legacy `~/.local/share/openmessage/` dir can be cleared with `desktop/scripts/cleanup-bridges` (dry-run by default; pass `--apply` to commit).

**Session migration**: on first launch the Go backend auto-copies `session.json` from `~/.local/share/openmessage/` into the active dir if one is missing, so re-pairing isn't needed after wiping `messages.db`.

## Known issues

- **`cargo tauri build` ends with xattr error on macOS 26+** — bundle IS created; the `scripts/dev` and `scripts/release` scripts grep-filter the error. Just use those.
- **WebView CORS blocks `fetch()` to localhost backend** — frontend probes readiness via `<img>` load (bypasses CORS), then `window.location.replace()` navigates.
- **Sandbox is disabled in `entitlements.plist`** — enabling it breaks the Go sidecar subprocess spawn.
- **Universal sidecar is lipo'd by `build-sidecar`** — Tauri 2's `externalBin` with `--target universal-apple-darwin` looks for a single file at `binaries/textbridge-backend-universal-apple-darwin`, not two arch-specific files. `build-sidecar` now produces the per-arch binaries plus a lipo'd universal. Set `BUILD_SIDECAR_HOST_ONLY=1` for a host-only binary during dev.

## MCP endpoints (optional)

Backend exposes MCP at `http://127.0.0.1:7007/mcp/sse` and stdio transport when launched by an MCP client.

**Default tool set (4):** `send_message`, `search_messages`, `list_conversations`, `conversation_stats` — kept tight so Claude Code's tool palette stays focused.

Override via `TEXTBRIDGE_MCP_TOOLS`:

| Value | Result |
|-------|--------|
| unset / `minimal` | the 4 above |
| `all` | every registered tool (20) |
| `name1,name2,…` | exactly those tools |

Register in Claude Code (user scope):

```bash
claude mcp add --transport sse --scope user textbridge http://127.0.0.1:7007/mcp/sse
```

The Tauri app must be running (it owns the backend on port 7007). MCP config lives in `~/.claude.json`, not `~/.claude/settings.json`.

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
