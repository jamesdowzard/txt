# Textbridge

Read and reply to Google Messages (SMS + RCS) from your Mac. Native Tauri app, local-first, no cloud dependency beyond Google's own Messages Web protocol.

Forked from [MaxGhenis/openmessage](https://github.com/MaxGhenis/openmessage) and retargeted for a single-user workflow: Google Messages only, frosted-dark aesthetic, signed Tauri desktop app.

## What it does

- **Google Messages for Mac** — pair your Android phone once, read/send SMS + RCS locally
- **Native Tauri app** — lives in `/Applications/Textbridge.app`, backend Go binary bundled inside
- **Local-first** — messages cached in SQLite at `~/Library/Application Support/ai.james-is-an.textbridge/`
- **MCP-ready** — optional MCP SSE + stdio endpoints for Claude Code integration

## Built on

- [mautrix/gmessages](https://github.com/mautrix/gmessages) `libgm` — Google Messages Web protocol client
- [Tauri 2](https://tauri.app/) — native desktop shell (Rust + TypeScript)
- [mcp-go](https://github.com/mark3labs/mcp-go) — MCP server

## Repo layout

```
textbridge/
├── main.go, cmd/, internal/   # Go backend (libgm, SQLite, HTTP + MCP)
│                              # Produces the textbridge-backend binary
├── desktop/                   # Tauri 2 macOS app
│   ├── src/                   # TypeScript frontend (minimal loader)
│   ├── src-tauri/             # Rust shell — spawns sidecar, owns window
│   │   └── binaries/          # Sidecar Go binary (gitignored, built via script)
│   ├── scripts/
│   │   ├── build-sidecar      # Compile Go backend for current arch
│   │   ├── dev                # Deploy dev variant → /Applications/Textbridge-dev.app
│   │   └── release            # Deploy stable → /Applications/Textbridge.app
│   └── release/               # Built app bundles (gitignored)
└── internal/web/static/       # Web UI loaded inside the Tauri WebView
```

## Quick start

### Prerequisites

- macOS 14+ on Apple Silicon (arm64)
- [Go 1.22+](https://go.dev/dl/)
- [Rust](https://rustup.rs/) + [Bun](https://bun.sh/)
- [Tauri CLI 2](https://tauri.app/start/prerequisites/): `cargo install tauri-cli@2`
- [fileicon](https://github.com/mklement0/fileicon): `brew install fileicon`
- Google Messages installed on an Android phone

### 1. Pair with your phone (one-time)

```bash
go build -o textbridge .
./textbridge pair
```

Open Google Messages on Android → profile → **Device pairing** → **Pair a device** → scan the terminal QR.

Session saves to `~/.local/share/openmessage/session.json`. Copy it to the app's data dir before first launch:

```bash
mkdir -p "$HOME/Library/Application Support/ai.james-is-an.textbridge"
cp "$HOME/.local/share/openmessage/session.json" \
   "$HOME/Library/Application Support/ai.james-is-an.textbridge/session.json"
```

### 2. Build and install the Tauri app

```bash
cd desktop
bun install
./scripts/build-sidecar      # Compile Go backend into src-tauri/binaries/
./scripts/release patch      # Build, sign, install to /Applications/Textbridge.app
```

Launch from `/Applications/Textbridge.app` or via Spotlight.

### Dev loop

```bash
cd desktop
./scripts/dev --launch       # Build orange dev variant, deploy, launch
```

Stable and dev can coexist — different bundle IDs, separate data dirs.

## Development

- **Web UI**: `web/` (Vite + Preact + HTM). `bun run build` writes to `internal/web/static/dist/`, which is embedded into the Go binary via `go:embed`. Legacy single-file UI is being peeled into components — see `docs/plans/2026-04-17-textbridge-roadmap.md`.
- **Rust shell** (window + sidecar lifecycle): `desktop/src-tauri/src/lib.rs`
- **Go backend** (libgm, HTTP, MCP): `main.go`, `cmd/`, `internal/`
- **Feature branches only** — use `/f:new` and `/f:ship` for all changes

See `desktop/CLAUDE.md` for Tauri-specific gotchas (xattr bundle issue, CORS probe trick).

## License

Inherits from upstream openmessage (Unlicense / public domain). `libgm` dependency is AGPL-3.0; we run it as a separate sidecar process, which keeps the Rust/TypeScript shell unencumbered by its copyleft.
