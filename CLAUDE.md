# txt

Native Tauri macOS app (bundle name `txt.app`, bundle ID `ai.james-is-an.textbridge`) wrapping a Go backend that reads + sends across Google Messages, iMessage, and the legacy Signal/WhatsApp importers. Forked from [MaxGhenis/openmessage](https://github.com/MaxGhenis/openmessage) and retargeted as a personal multi-platform reader. The Go binary is still named `textbridge-backend` for historical/codesign continuity — only the visible app name changed to `txt`.

## Architecture

```
textbridge/
├── main.go                # Go entrypoint (CLI: pair, serve, send, import)
├── cmd/                   # Go command implementations
├── internal/
│   ├── app/               # Bootstrap, data dir, backfill
│   ├── client/            # libgm Google Messages protocol
│   ├── contacts/          # macOS Contacts loader (phone+email → name)
│   ├── db/                # SQLite store (conversations, messages, contacts, drafts)
│   ├── importer/          # Per-platform importers (gchat, imessage, signal_desktop, whatsapp_native)
│   ├── tools/             # MCP tools
│   └── web/static/dist/   # Embedded web UI (Vite build output — source lives in web/)
├── web/                   # Vite + Preact + HTM source for the UI
│   ├── index.html         # HTML shell
│   ├── public/            # Static assets copied verbatim to dist/
│   └── src/               # main.ts + legacy.{js,css} + styles/
├── desktop/               # Tauri 2 macOS app
│   ├── src-tauri/         # Rust shell: spawns sidecar, owns window
│   │   ├── src/lib.rs     # Sidecar lifecycle
│   │   └── binaries/      # Go backend binary (gitignored)
│   ├── src/               # TypeScript loader (image-probe then navigate to backend)
│   └── scripts/           # build-sidecar, dev, release
└── site/                  # (legacy) upstream's marketing site — not used here
```

## iMessage support

`internal/importer/imessage.go` reads `~/Library/Messages/chat.db` every 30s (requires Full Disk Access on `/Applications/txt.app`). Features:

- **Body extraction** from `m.text` AND `m.attributedBody` (NSKeyedArchiver typedstream blob — modern Messages.app leaves `text` NULL).
- **Attachments** stored as `MediaID` = path relative to `~/Library/Messages/Attachments`. `/api/media/<msgid>` serves the file with the correct MIME type, sandboxed to that root.
- **Reactions** (Tapbacks) folded from `associated_message_type` 2000-3005 rows into the existing JSON shape the UI renders.
- **Read receipts** map `is_read=1` → `status='read'` on outgoing messages.
- **Contact resolution** via `internal/contacts`, which walks every `~/Library/Application Support/AddressBook/Sources/*/AddressBook-v22.abcddb` and indexes both phone numbers (last-9-digit normalization) and email addresses (lowercased). Names propagate to conversation `name`, message `sender_name`, and reaction actors.
- **Sending** via `osascript`: `/api/send` for any `imessage:` conversation calls `tell application "Messages" ... send ... to buddy <handle>` without `activate`. 1:1 only — group chats blocked at the buddy-picker.

`loadChats` MUST drain `*sql.Rows` before issuing nested participant/message queries — chat.db is opened with `SetMaxOpenConns(1)` and a nested query inside `rows.Next()` deadlocks. See PR #52.

## Build + deploy

```bash
# Web UI (Vite build output goes to internal/web/static/dist/)
cd web && bun install && bun run build

# Go backend (produces ./textbridge; embeds the Vite dist)
go build -o textbridge .

# Tauri app — build-sidecar runs the web build automatically
cd desktop
./scripts/build-sidecar      # required before cargo tauri build
./scripts/release patch      # build, sign, install to /Applications/txt.app
./scripts/dev --launch       # orange dev variant alongside stable
```

Skip the web rebuild during tight iteration with `SKIP_WEB_BUILD=1 ./scripts/build-sidecar`.

Signing: `Developer ID Application: James Dowzard (G54DLMPV94)`. Bundle ID: `ai.james-is-an.textbridge`.

## Data locations

| Path | Contents |
|------|----------|
| `~/Library/Application Support/ai.james-is-an.textbridge/` | Active app data (session, messages.db) |
| `~/.local/share/openmessage/` | Legacy location (`./textbridge pair` writes here) |

The Tauri shell passes `OPENMESSAGES_DATA_DIR` → Go sidecar, pointing at the first path.

## Google-only mode (sender-side only)

The Tauri shell launches the sidecar with `TEXTBRIDGE_GOOGLE_ONLY=1`, which skips the WhatsApp + Signal live-bridge connects and the WhatsApp Native / Signal Desktop importers. iMessage sync (read + send) still runs unconditionally. CLI `./textbridge serve` without the flag retains upstream multi-platform behaviour.

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
git merge upstream/main   # expect conflicts on README, CLAUDE.md. Upstream's internal/web/static/index.html
                          # no longer exists on this fork — the source of truth is web/ (Vite + Preact).
                          # Port upstream UI changes by patching web/src/legacy.{js,css} or the relevant component.
```

## Key files

- `main.go`, `cmd/pair.go`, `cmd/serve.go` — Go entrypoints
- `web/index.html` + `web/src/main.ts` — web UI entry (Vite + Preact + HTM)
- `web/src/styles/tokens.css` — design tokens (`:root` vars)
- `web/src/legacy.{js,css}` — lifted-and-shifted monolith, being peeled into components per `docs/plans/2026-04-17-textbridge-roadmap.md` Task 0.4
- `internal/web/api.go` — HTTP handler + `//go:embed static/dist` for serving the Vite build
- `desktop/src-tauri/src/lib.rs` — Tauri setup, sidecar lifecycle
- `desktop/src-tauri/tauri.conf.json` — bundle config, `externalBin` sidecar reference
- `desktop/src-tauri/capabilities/default.json` — Tauri permissions (shell:allow-execute, shell:allow-spawn)
