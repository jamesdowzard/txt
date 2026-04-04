export const downloadUrl =
  "https://github.com/MaxGhenis/openmessage/releases/latest/download/OpenMessage.dmg";
export const repoUrl = "https://github.com/MaxGhenis/openmessage";
export const mcpSseUrl = "http://127.0.0.1:7007/mcp/sse";
export const claudeMcpCommand = `claude mcp add -s user --transport sse openmessage ${mcpSseUrl}`;

export const productSignals = [
  {
    title: "Google Messages and WhatsApp both feel native",
    body: "SMS, RCS, and WhatsApp already ship inside the same local workspace, with grouped routes, media, typing, and read-state support."
  },
  {
    title: "The message store stays on your machine",
    body: "Messages, contacts, search, diagnostics, and bridge sessions live locally. There is no required OpenMessage cloud account."
  },
  {
    title: "MCP is part of the runtime, not a demo wrapper",
    body: "Assistants can search, draft, summarize, and send through the same local client state you are already using in the app."
  }
];

export const workflowSteps = [
  {
    number: "01",
    title: "Pair the routes you already use",
    body: "Connect Google Messages, then link WhatsApp in the same app instead of living in browser mirrors and companion tabs."
  },
  {
    number: "02",
    title: "Stay in one thread workspace",
    body: "Search, read, reply, review media, and switch between SMS and WhatsApp lanes without leaving the desktop surface."
  },
  {
    number: "03",
    title: "Let AI use the same local context",
    body: "Expose the built-in MCP endpoint to Claude Code or any MCP client when you want drafts, triage, or route-aware message automation."
  }
];

export const setupColumns = [
  {
    title: "macOS app",
    eyebrow: "Fastest path",
    body: "Use the native Swift wrapper with notifications, contact photos, and an embedded local backend that already handles Google Messages and live WhatsApp.",
    bullets: [
      "Download the latest DMG and drag OpenMessage to Applications.",
      "Pair Google Messages from the in-app setup flow.",
      "Turn on WhatsApp live sync from the same Connections surface."
    ],
    commands: [
      {
        label: "Download",
        code: downloadUrl
      },
      {
        label: "Claude Code MCP",
        code: claudeMcpCommand
      }
    ]
  },
  {
    title: "CLI and local web app",
    eyebrow: "Any platform",
    body: "Run the Go binary directly if you want the same local web UI, MCP server, and pairing flow without the native wrapper.",
    bullets: [
      "Install the release binary for your platform or build from source.",
      "Pair with your phone using the local pairing command.",
      "Start the web UI and MCP server on localhost."
    ],
    commands: [
      {
        label: "Pair",
        code: "openmessage pair"
      },
      {
        label: "Serve",
        code: "openmessage serve"
      }
    ]
  }
];

export const aiBlocks = [
  {
    title: "Standard MCP over SSE",
    body: "Whenever OpenMessage is running, Claude Code, Cursor, and custom agents can connect to the same local inbox without wrappers or hosted relays.",
    command: mcpSseUrl
  },
  {
    title: "Built for real message operations",
    body: "List conversations, search history, inspect contacts, open threads, draft responses, and send through the same local route state the UI uses.",
    command: "/messages"
  }
];

export const thesisStats = [
  { value: "$175M+", label: "Unified messaging acquisitions" },
  { value: "$120/yr", label: "Beeper premium pricing" },
  { value: "0", label: "Competitors with native MCP support" }
];

export const expansionRows = [
  ["Google Messages (SMS/RCS)", "mautrix/gmessages", "Shipped", "Core local messaging route"],
  ["WhatsApp", "whatsmeow", "Shipped", "Largest global consumer network"],
  ["Signal", "signal-cli / bridge", "Next", "Privacy-conscious users"],
  ["Telegram", "mautrix-telegram", "Planned", "Large cross-platform network"],
  ["Discord", "mautrix-discord", "Planned", "Community and developer use"],
  ["Slack", "mautrix-slack", "Planned", "Work messaging"],
  ["iMessage", "local importer / bridge", "Longer-term", "Mac-bound but strategically important"]
];

export const competitionRows = [
  ["OpenMessage", "Yes", "Yes", "Yes", "Free / premium later"],
  ["Beeper", "Partial", "No", "No", "$10/month"],
  ["Franz / Ferdi", "Partial", "Mostly", "No", "Free / paid"],
  ["Google Messages Web", "No", "N/A", "No", "Free"]
];
