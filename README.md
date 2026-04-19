# txt

Personal multi-platform messages app for macOS. Native Tauri shell + Go backend reading from Google Messages, iMessage (chat.db), and legacy Signal/WhatsApp importers. Local-first, single-user.

## What it does

- **Google Messages for Mac** — pair an Android phone once, read/send SMS + RCS locally (via mautrix `libgm`)
- **iMessage two-way** — reads `~/Library/Messages/chat.db`, sends via osascript to `Messages.app`
- **Local-first** — everything cached in SQLite at `~/Library/Application Support/ai.james-is-an.textbridge/`
- **MCP-ready** — optional MCP SSE + stdio endpoints for Claude Code

Bundle is `/Applications/txt.app` (bundle ID `ai.james-is-an.textbridge`, sidecar binary `textbridge-backend` — both kept for codesign/data continuity).

## Repo layout

```
txt/
├── main.go, cmd/, internal/   # Go backend (libgm, SQLite, HTTP + MCP)
│                              # Produces the textbridge-backend binary
├── web/                       # Vite + Preact + HTM UI (embedded via go:embed)
├── desktop/                   # Tauri 2 macOS app
│   ├── src-tauri/             # Rust shell — spawns sidecar, owns window
│   └── scripts/               # build-sidecar, dev, release
└── internal/web/static/dist/  # Vite build output served by Go
```

## Quick start

### Prerequisites

- macOS 14+ on Apple Silicon (arm64)
- [Go 1.22+](https://go.dev/dl/)
- [Rust](https://rustup.rs/) + [Bun](https://bun.sh/)
- [Tauri CLI 2](https://tauri.app/start/prerequisites/): `cargo install tauri-cli@2`
- [fileicon](https://github.com/mklement0/fileicon): `brew install fileicon`
- Google Messages on an Android phone (optional, for that route)

### 1. Pair with your phone (one-time, Google Messages only)

```bash
go build -o txt .
./txt pair
```

Open Google Messages on Android → profile → **Device pairing** → **Pair a device** → scan the terminal QR.

### 2. Build and install

```bash
cd desktop
bun install
./scripts/build-sidecar      # Compile Go backend into src-tauri/binaries/
./scripts/release patch      # Build, sign, install to /Applications/txt.app
```

### Dev loop

```bash
cd desktop
./scripts/dev --launch       # Build orange dev variant, deploy, launch
```

Stable and dev coexist via different bundle IDs + data dirs.

## Development

- **Web UI**: `web/` (Vite + Preact + HTM). `bun run build` writes to `internal/web/static/dist/`, embedded via `go:embed`.
- **Rust shell**: `desktop/src-tauri/src/lib.rs`
- **Go backend**: `main.go`, `cmd/`, `internal/`
- **Feature branches only** — use `/f:new` and `/f:ship`.

See `CLAUDE.md` for iMessage internals and Tauri gotchas.

## License

Unlicense / public domain. `libgm` (AGPL-3.0) runs as a separate sidecar process to keep the shell free of copyleft.
