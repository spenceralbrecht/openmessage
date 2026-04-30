# OpenMessage

OpenMessage is a local-first messaging workspace for Google Messages, WhatsApp, and Signal. Use it from the native macOS app, the localhost web UI, or any MCP-compatible client.

Built on [mautrix/gmessages](https://github.com/mautrix/gmessages) (libgm) for the Google Messages protocol and [mcp-go](https://github.com/mark3labs/mcp-go) for the MCP server.

## What it does

- **Google Messages for Mac** — pair your Android phone and read/send SMS + RCS locally
- **Live WhatsApp support** — link WhatsApp as a live companion device on your machine
- **Live Signal support** — link Signal locally and keep its threads in the same inbox
- **One local inbox** — search, route-aware threads, media, reactions, drafts, and grouped contacts
- **macOS app + web UI** — native wrapper with notifications and contact photos, plus a localhost UI
- **MCP-ready** — expose the same local inbox to Claude Code and other MCP clients

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

By default, a QR code appears in your terminal. On your phone, open **Google Messages > Settings > Device pairing > Pair a device** and scan it. The session saves to `~/.local/share/openmessage/session.json`.

If Google only offers account pairing, you can also pair with Google account cookies copied from browser devtools:

```bash
pbpaste | ./openmessage pair --google
```

The CLI accepts either a JSON cookie object or a full `curl` command for `messages.google.com/web/config`, then prompts you to confirm an emoji on your phone.

### 3. Start the server

```bash
./openmessage serve
```

This starts both:
- **Web UI** at [http://127.0.0.1:7007](http://127.0.0.1:7007)
- **MCP SSE endpoint** at `http://127.0.0.1:7007/mcp/sse`

The server now creates or reads an auth token on startup. Use the authenticated URL printed in the logs, or set `OPENMESSAGES_AUTH_TOKEN` yourself and pass it as `Authorization: Bearer <token>` or `X-OpenMessage-Token` for API/MCP clients.

When `serve` is launched by an MCP client over pipes, it also serves MCP on stdio automatically.

### 3a. CLI-first agent workflow

Prefer the bounded JSON CLI for agent access to private messages:

```bash
./openmessage conversations --limit 20
./openmessage messages <conversation_id> --limit 20
./openmessage search "notion" --limit 10
./openmessage draft <conversation_id> "Reply text to review"
./openmessage send-draft <draft_id> --confirm <draft_id>
```

Direct `send` and `send-group` commands are disabled by default. Set `OPENMESSAGES_ALLOW_DIRECT_SEND=1` only when you intentionally want the old direct-send behavior.

### 3b. Optional: link WhatsApp or Signal

After `serve` is running, open the local UI and link WhatsApp or Signal from the Connections surface. OpenMessage keeps those bridges local and syncs them into the same inbox as Google Messages.

### 3c. Safe demo mode for screenshots and recordings

```bash
./openmessage demo
```

or:

```bash
./openmessage serve --demo
```

Demo mode starts the same local UI on the normal port, but:
- uses a fresh temporary data directory instead of your real message store
- seeds fake SMS and WhatsApp conversations for screenshots and demos
- disables live Google Messages, WhatsApp, and local desktop import sync

This is the safest way to capture website screenshots, App Store assets, or demo recordings without real messages bleeding back in.

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

Restart Claude Code. By default, MCP exposes read-only tools only. Extra tool groups are opt-in:

- `OPENMESSAGES_MCP_DRAFTS=1` exposes `draft_message`
- `OPENMESSAGES_MCP_WRITES=1` exposes direct send, media send, group send, and reactions
- `OPENMESSAGES_MCP_IMPORTS=1` exposes import and media-download tools
- `OPENMESSAGES_MCP_EXTERNAL_LLM=1` exposes story/viz tools that may send message samples to external LLM providers

## Features

- **Read messages** — full conversation history, search, media, replies, reactions
- **Send messages** — SMS/RCS plus live WhatsApp and Signal text, media, and reactions
- **Live WhatsApp sync** — pair a local WhatsApp companion device for inbound messages, typing indicators, read state, and media
- **Live Signal sync** — pair a local Signal linked device for inbound messages, media, reactions, and group threads
- **React to messages** — emoji reactions on any message
- **Image/media display** — inline images, video, audio, and fullscreen viewer
- **Desktop notifications** — native macOS notifications for fresh inbound messages
- **Web UI + macOS app** — real-time conversation view at localhost:7007 and a native wrapper
- **MCP tools** — read-only conversation lookup by default, with write/import/external-LLM tools available only through explicit env opt-ins
- **Local storage** — SQLite database, your data stays on your machine

## MCP tools

| Tool | Description |
|------|-------------|
| `get_messages` | Recent messages with filters (phone, date range, limit) |
| `get_conversation` | Messages in a specific conversation |
| `search_messages` | Full-text search across all messages |
| `list_conversations` | List recent conversations |
| `list_contacts` | List/search contacts |
| `get_status` | Google Messages, WhatsApp, and Signal connection status |
| `draft_message` | Save a draft for review/send later; requires `OPENMESSAGES_MCP_DRAFTS=1` |
| `send_message`, `send_to_conversation`, `send_media_to_conversation`, `send_group_message`, `react_to_message` | Direct writes; require `OPENMESSAGES_MCP_WRITES=1` |
| `download_media`, `import_messages` | Local file/import helpers; require `OPENMESSAGES_MCP_IMPORTS=1` |
| Story/viz tools | External LLM and rendering tools; require `OPENMESSAGES_MCP_EXTERNAL_LLM=1` |

## MCP examples

- List recent Signal threads: `list_conversations(source_platform="signal")`
- Search WhatsApp for a keyword: `search_messages(query="airbnb")`
- Create a draft for review: `draft_message(conversation_id="whatsapp:15551234567@s.whatsapp.net", message="On my way")`
- React to a message after enabling writes: `react_to_message(conversation_id="signal-group:abc123", message_id="signal:...", emoji="🔥")`
- Import Signal Desktop history after enabling imports: `import_messages(source="signal", path="$HOME/Library/Application Support/Signal", name="Your Name", address="+15551230000")`

## Web UI

The web UI runs at `http://localhost:7007` when the server is started. It provides:

- Conversation list with search and grouped multi-route contacts
- Message view with images, video, audio, reactions, and reply threads
- Route-aware compose and send
- Google Messages + WhatsApp + Signal connection controls
- Live typing indicators, read-state rendering, and notifications

## Native macOS app

The repo also includes a native Swift wrapper around the same local backend:

- embedded local OpenMessage backend
- native notifications
- contact photos
- the same Google Messages, WhatsApp, and Signal pairing/runtime model as the web UI

The macOS app target lives under `OpenMessage/`.

## Configuration

| Env var | Default | Purpose |
|---------|---------|---------|
| `OPENMESSAGES_DATA_DIR` | `~/.local/share/openmessage` | Data directory (DB + session) |
| `OPENMESSAGES_LOG_LEVEL` | `info` | Log level (debug/info/warn/error/trace) |
| `OPENMESSAGES_PORT` | `7007` | Web UI port |
| `OPENMESSAGES_HOST` | `127.0.0.1` | Host/interface to bind the local web server to |
| `OPENMESSAGES_AUTH_TOKEN` | random per launch | Required bearer/header/query token for `/api/*` and `/mcp/*` |
| `OPENMESSAGES_UNSAFE_NETWORK` | unset | Must be `1` before binding `OPENMESSAGES_HOST` to a non-loopback interface |
| `OPENMESSAGES_MCP_DRAFTS` | unset | Set to `1` to expose the MCP draft tool |
| `OPENMESSAGES_MCP_WRITES` | unset | Set to `1` to expose MCP send/media/reaction tools |
| `OPENMESSAGES_MCP_IMPORTS` | unset | Set to `1` to expose MCP import/media-download tools |
| `OPENMESSAGES_MCP_EXTERNAL_LLM` | unset | Set to `1` to expose MCP story/viz tools |
| `OPENMESSAGES_ALLOW_EXTERNAL_LLM` | unset | Set to `1` to enable the web `/api/story/*` external LLM endpoint |
| `OPENMESSAGES_ATTACHMENT_DIR` | unset | Required sandbox directory for MCP media-send file reads |
| `OPENMESSAGES_ALLOW_DIRECT_SEND` | unset | Set to `1` to restore unsafe direct CLI send commands |
| `OPENMESSAGES_MY_NAME` | system user name | Display name for outgoing imported iMessage/WhatsApp messages |
| `OPENMESSAGES_STARTUP_BACKFILL` | `auto` | Startup history sync mode: `auto`, `shallow`, `deep`, or `off` |
| `OPENMESSAGES_MACOS_NOTIFICATIONS` | interactive macOS `serve` sessions only | Enable/disable native macOS notifications for fresh inbound live messages (`1`/`0`). Click-through opens the matching thread when `terminal-notifier` is available. |
| `OPENMESSAGE_TELEMETRY` | unset (off) | Set to `1` to send one anonymous heartbeat per launch (max one per 24h). Reports only: random install ID, version, OS/arch, and which platforms are paired (Google Messages / WhatsApp / Signal). No message content, no contact info, no IP-based identity. See `internal/telemetry/`. |

## Architecture

- **libgm** handles the Google Messages protocol (pairing, encryption, long-polling)
- **whatsmeow** handles live WhatsApp pairing, sync, text/media send, receipts, typing, and avatars through a separate local session store
- **signal-cli** powers the local Signal linked-device bridge, message sync, media, and reactions
- **SQLite** (WAL mode, pure Go) stores messages, conversations, and contacts locally
- Real-time events from the phone are written to SQLite as they arrive
- The native macOS app and the localhost web UI run against the same local backend
- WhatsApp Desktop import remains as a fallback/repair path when the live bridge is not active
- Signal Desktop history can be imported into the same local store for backfill and repair workflows
- On first run, a deep backfill fetches full SMS/RCS history in the background; later runs do a lighter incremental sync by default
- MCP tool handlers read from SQLite for queries and route sends through the same local runtime
- Auth tokens auto-refresh and persist to `session.json`

## Development

```bash
go test ./...        # Run all tests
go build .           # Build binary
npm install          # Install Playwright test runner
npx playwright install chromium
npm run test:e2e     # Run browser-level web UI tests
./openmessage pair  # Pair with phone
./openmessage serve # Start server
./openmessage demo  # Start isolated fake-data demo mode
```

Before publishing a build or website update, run through [the release checklist](docs/release-checklist.md).

## License

Unlicense. See `LICENSE`.
