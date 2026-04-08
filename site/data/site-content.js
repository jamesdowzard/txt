export const downloadUrl =
  "https://github.com/MaxGhenis/openmessage/releases/latest/download/OpenMessage.dmg";
export const repoUrl = "https://github.com/MaxGhenis/openmessage";
export const mcpSseUrl = "http://127.0.0.1:7007/mcp/sse";
export const claudeMcpCommand = `claude mcp add -s user --transport sse openmessage ${mcpSseUrl}`;

export const productSignals = [
  {
    title: "Google Messages, WhatsApp, and Signal share one inbox",
    body: "SMS, RCS, WhatsApp, and Signal all land inside the same local workspace, with grouped people, route tabs, media, and notifications."
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
    body: "Connect Google Messages, then add WhatsApp and Signal from the same desktop surface instead of living in browser mirrors and companion tabs."
  },
  {
    number: "02",
    title: "Stay in one thread workspace",
    body: "Search, read, reply, review media, and switch between routes as tabs without leaving the same thread surface."
  },
  {
    number: "03",
    title: "Let AI use the same local context",
    body: "Expose the built-in MCP endpoint to Claude Code or any MCP client when you want drafts, triage, or route-aware message automation."
  }
];

export const howItWorksPoints = [
  {
    title: "Live local bridges",
    body: "Google Messages, WhatsApp, and Signal each connect locally on your own machine, so messages, media, and route state flow into the app without a required hosted OpenMessage account."
  },
  {
    title: "One local message store",
    body: "All routes normalize into the same local inbox, search index, notifications, and people-first UI instead of living behind separate browser mirrors."
  },
  {
    title: "Same runtime for the app and MCP",
    body: "The local backend that powers the app is also what Claude and other MCP clients connect to, so the assistant sees the same thread state you do."
  }
];

export const faqItems = [
  {
    question: "How is WhatsApp support possible?",
    answer:
      "OpenMessage uses a live linked-device architecture on your Mac. WhatsApp connects locally, then syncs messages, media, typing, and receipts into the same local store as your other routes."
  },
  {
    question: "What works today across Google Messages, WhatsApp, and Signal?",
    answer:
      "Google Messages, WhatsApp, and Signal all run in the current app. Google Messages covers SMS and RCS, WhatsApp supports live text and media, and Signal supports live text, groups, reactions, and attachments, with history transfer available during pairing."
  },
  {
    question: "Does OpenMessage proxy my messages through your servers?",
    answer:
      "No required OpenMessage cloud account is involved in normal use. Messages, contacts, diagnostics, and bridge sessions stay local-first on your machine."
  },
  {
    question: "Where can I read the technical details?",
    answer:
      "There are technical write-ups covering both WhatsApp and Google Messages, including the local runtime, pairing model, and why OpenMessage keeps these routes inside one shared local inbox."
  }
];

export const blogPosts = [
  {
    slug: "how-openmessage-added-google-messages",
    title: "How OpenMessage added Google Messages",
    description:
      "The pairing model, live event path, local inbox, and MCP runtime behind Google Messages in OpenMessage.",
    eyebrow: "Engineering note"
  },
  {
    slug: "how-openmessage-added-whatsapp",
    title: "How OpenMessage added live WhatsApp support",
    description:
      "The local bridge, linked-device model, shared inbox, and MCP runtime behind WhatsApp in OpenMessage.",
    eyebrow: "Engineering note"
  }
];

export const setupColumns = [
  {
    title: "macOS app",
    eyebrow: "Fastest path",
    body: "Use the native Swift wrapper with notifications, contact photos, and an embedded local backend that already handles Google Messages, WhatsApp, and Signal.",
    bullets: [
      "Download the latest DMG and drag OpenMessage to Applications.",
      "Pair Google Messages from the in-app setup flow.",
      "Add WhatsApp and Signal from the same Platforms surface."
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
  ["Signal", "signal-cli / bridge", "Shipped", "Privacy-conscious users"],
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
