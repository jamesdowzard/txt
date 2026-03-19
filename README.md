# OpenMessage

An open-source Google Messages client with MCP support. Read and send SMS/RCS from Claude Code, a web UI, or any MCP-compatible tool.

Built on [mautrix/gmessages](https://github.com/mautrix/gmessages) (libgm) for the Google Messages protocol and [mcp-go](https://github.com/mark3labs/mcp-go) for the MCP server.

## Quick start

### Prerequisites

- **Go 1.22+** ([install](https://go.dev/dl/))
- **Google Messages** on your Android phone

### 1. Clone and build

```bash
git clone https://github.com/MaxGhenis/openmessage.git
cd openmessage
go build -o openmessage .
```

### 2. Pair with your phone

```bash
./openmessage pair
```

A QR code appears in your terminal. On your phone, open **Google Messages > Settings > Device pairing > Pair a device** and scan it. The session saves to `~/.local/share/openmessage/session.json`.

### 3. Start the server

```bash
./openmessage serve
```

This starts both:
- **Web UI** at [http://127.0.0.1:7007](http://127.0.0.1:7007)
- **MCP SSE endpoint** at `http://127.0.0.1:7007/mcp/sse`

When `serve` is launched by an MCP client over pipes, it also serves MCP on stdio automatically.

### 4. Connect to Claude Code

Add to `~/.mcp.json`:

```json
{
  "mcpServers": {
    "openmessage": {
      "command": "/path/to/openmessage",
      "args": ["serve"]
    }
  }
}
```

Restart Claude Code. The 7 tools appear automatically.

## Features

- **Read messages** — full conversation history, search, media
- **Send messages** — SMS and RCS, including replies
- **React to messages** — emoji reactions on any message
- **Image/media display** — inline images with fullscreen viewer
- **Web UI** — real-time conversation view at localhost:7007
- **MCP tools** — 7 tools for Claude Code integration
- **Local storage** — SQLite database, your data stays on your machine

## MCP tools

| Tool | Description |
|------|-------------|
| `get_messages` | Recent messages with filters (phone, date range, limit) |
| `get_conversation` | Messages in a specific conversation |
| `search_messages` | Full-text search across all messages |
| `send_message` | Send SMS/RCS to a phone number |
| `list_conversations` | List recent conversations |
| `list_contacts` | List/search contacts |
| `get_status` | Connection status and paired phone info |

## Web UI

The web UI runs at `http://localhost:7007` when the server is started. It provides:

- Conversation list with search
- Message view with images, reactions, and reply threads
- Compose and send messages
- React to messages (right-click)
- Reply to messages (double-click)

## Configuration

| Env var | Default | Purpose |
|---------|---------|---------|
| `OPENMESSAGES_DATA_DIR` | `~/.local/share/openmessage` | Data directory (DB + session) |
| `OPENMESSAGES_LOG_LEVEL` | `info` | Log level (debug/info/warn/error/trace) |
| `OPENMESSAGES_PORT` | `7007` | Web UI port |
| `OPENMESSAGES_HOST` | `127.0.0.1` | Host/interface to bind the local web server to |
| `OPENMESSAGES_MY_NAME` | system user name | Display name for outgoing imported iMessage/WhatsApp messages |
| `OPENMESSAGES_STARTUP_BACKFILL` | `auto` | Startup history sync mode: `auto`, `shallow`, `deep`, or `off` |

## Architecture

- **libgm** handles the Google Messages protocol (pairing, encryption, long-polling)
- **SQLite** (WAL mode, pure Go) stores messages, conversations, and contacts locally
- Real-time events from the phone are written to SQLite as they arrive
- On first run, a deep backfill fetches full SMS/RCS history in the background; later runs do a lighter incremental sync by default
- MCP tool handlers read from SQLite for queries, call libgm for sends
- Auth tokens auto-refresh and persist to `session.json`

## Development

```bash
go test ./...        # Run all tests
go build .           # Build binary
./openmessage pair  # Pair with phone
./openmessage serve # Start server
```

## License

MIT
