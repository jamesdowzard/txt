export const downloadUrl =
  "https://github.com/MaxGhenis/openmessage/releases/latest/download/OpenMessage.dmg";
export const repoUrl = "https://github.com/MaxGhenis/openmessage";
export const mcpSseUrl = "http://127.0.0.1:7007/mcp/sse";
export const claudeMcpCommand = `claude mcp add -s user --transport sse openmessage ${mcpSseUrl}`;

export const productSignals = [
  {
    title: "Google Messages + WhatsApp",
    body: "Live SMS, RCS, and WhatsApp in one local workspace, with route-aware replies and media support."
  },
  {
    title: "Runs locally by default",
    body: "Messages, contacts, search, and MCP access stay on your machine. There is no required cloud account."
  },
  {
    title: "AI-native control layer",
    body: "OpenMessage exposes a local MCP server so assistants can search, draft, summarize, and send through the same client."
  }
];

export const workflowSteps = [
  {
    number: "01",
    title: "Pair your transports",
    body: "Connect Google Messages today and add live WhatsApp inside the same app instead of juggling browser tabs and phone mirrors."
  },
  {
    number: "02",
    title: "Work from one thread workspace",
    body: "Search, read, reply, review media, and switch routes without leaving the desktop surface."
  },
  {
    number: "03",
    title: "Let AI operate with context",
    body: "Expose the local MCP endpoint to Claude Code or any MCP client when you want drafts, triage, or message automation."
  }
];

export const setupColumns = [
  {
    title: "macOS app",
    eyebrow: "Fastest path",
    body: "Use the native Swift wrapper with notifications, contacts, and an embedded local backend.",
    bullets: [
      "Download the latest DMG and drag OpenMessage to Applications.",
      "Pair Google Messages from the in-app setup flow.",
      "Add WhatsApp live sync from the Connections surface once the app is open."
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
    body: "Run the Go binary directly if you want the local web UI, MCP server, and pairing flow without the native wrapper.",
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
    body: "OpenMessage serves a local MCP endpoint whenever the app is running, so Claude Code, Cursor, and custom agents can connect without wrappers or cloud relays.",
    command: mcpSseUrl
  },
  {
    title: "Built for message operations",
    body: "List conversations, search history, inspect contacts, open threads, draft responses, and send messages through one local tool surface.",
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
