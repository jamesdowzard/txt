# txt × macOS Shortcuts

txt exposes its actions to macOS Shortcuts (and any other URL-capable
automation tool) via the `txt://` URL scheme. `textbridge://` is still
registered as a legacy alias so old shortcuts and recipes keep working —
use `txt://` in anything new.

There's no dedicated App Intents bundle. Shortcuts.app's built-in **Open
URLs** action is wired to the scheme handler in
`desktop/src-tauri/src/lib.rs`, so every Shortcut below is a one-action
recipe. See [Design rationale](#design-rationale-url-scheme-vs-app-intents)
at the bottom of this doc for why.

## Actions

| URL | Effect |
|-----|--------|
| `txt://compose?to=+61412345678&body=hello` | If both `to` **and** `body` are present, sends the message in the background (no window focus, no overlay). If `body` is missing, opens txt with the new-message overlay and the handle prefilled. |
| `txt://compose?to=+61412345678` | Opens txt, shows the new-message overlay with the recipient prefilled. User types + sends. |
| `txt://search?q=dinner` | Opens txt, focuses the search bar, types the query. |
| `txt://conversation/<conversation-id>` | Opens txt and reveals a specific thread. |
| `txt://open` | Just brings txt to the front. Useful as a global hotkey target. |

### Handle routing

`to` is a phone number or an email address.

- **Phone number** (`+61412345678`, `0412 345 678`, etc.) → routed to
  Google Messages. The Rust shell calls `POST /api/new-conversation` on
  the Go backend, which is the same get-or-create path the in-app New
  Message overlay uses.
- **Email address** (`someone@example.com`) → routed to iMessage. The
  conversation ID is `imessage:<handle>`; the Go backend's iMessage
  sender picks it up directly via `osascript`.

Other handle shapes are rejected at the Rust layer and logged to the app
console (`[deep-link] …`). Open an issue if you need another route.

### Auto-send vs. overlay

The `compose` action has two modes depending on the query string:

| Query shape | Behaviour | Focus-stealing? |
|-------------|-----------|----|
| `?to=X&body=Y` (both set) | Resolve handle → POST `/api/send` in the background | No — app stays in the background. |
| `?to=X` (body missing/empty) | Open the new-message overlay with `to` prefilled, focus the window | Yes — user wanted to review. |
| `?body=Y` (to missing) | Open the overlay with `body` prefilled, focus the window | Yes. |

If the auto-send path fails (e.g. backend not running, recipient can't
be resolved), it falls through to the overlay so the user gets a visible
retry affordance.

## Recipes

### Send a text message to a contact (auto-send)

1. Open Shortcuts.app → **File → New Shortcut**.
2. Drop in these actions in order:
   - **Ask for Input** (prompt: "Phone number", type: Text)
   - **Ask for Input** (prompt: "Message", type: Text)
   - **URL** → `txt://compose?to=[Provided Input 1]&body=[URL-Encoded Input 2]`
   - **Open URLs**
3. Name it "Send a Message With txt". It now appears in Spotlight, the
   menu bar, and can be triggered from Siri ("Send a Message With txt").

This recipe corresponds 1:1 to the `Send a Message With txt` App Intent
mentioned in the codebase roadmap — we achieve the intent's behaviour
through the URL scheme handler instead of shipping a Swift App Intents
target.

### Open the composer with a contact prefilled (no auto-send)

1. New Shortcut.
2. Actions:
   - **Ask for Input** (prompt: "Phone number", type: Text)
   - **URL** → `txt://compose?to=[Provided Input]`
   - **Open URLs**
3. Name it "Text via txt".

### Search all messages for a term

1. New Shortcut.
2. Actions:
   - **Ask for Input** (prompt: "Search txt for…", type: Text)
   - **URL Encode**
   - **Text** → `txt://search?q=[URL-Encoded Text]`
   - **Open URLs**
3. Name it "Search txt".

### Jump to a pinned conversation

1. Find the conversation ID — either via the MCP `list_conversations` tool
   or by opening the conversation in txt and copying the
   `?conversation=…` value from the in-app URL bar (⌘L).
2. New Shortcut → **URL** → `txt://conversation/<paste-id-here>`
   → **Open URLs**.
3. Bind to a hotkey in **System Settings → Keyboard → Keyboard Shortcuts
   → Services** for a one-key jump.

## Testing from the shell

```bash
# Auto-send (Google Messages, phone)
open 'txt://compose?to=+61412345678&body=smoke%20test'

# Auto-send (iMessage, email)
open 'txt://compose?to=someone@example.com&body=smoke%20test'

# Open overlay only (no body)
open 'txt://compose?to=+61412345678'

# Search
open 'txt://search?q=groceries'

# Specific conversation
open 'txt://conversation/imessage:+61412345678'

# Just focus the window
open 'txt://open'

# Legacy scheme still works
open 'textbridge://compose?to=+61412345678'
```

There's an end-to-end smoke test at
`desktop/scripts/smoke-url-scheme` that fires an `open txt://compose?…`
and asserts a message landed in the outgoing queue. Run it manually
after launching the app:

```bash
./desktop/scripts/dev --launch
./desktop/scripts/smoke-url-scheme +61412345678 "smoke from $(date +%s)"
```

Each `open` hands the URL off to `LaunchServices`, which resolves
`CFBundleURLTypes` (registered via `plugins.deep-link.desktop.schemes` in
`desktop/src-tauri/tauri.conf.json`) and dispatches into
`dispatch_deep_link` in the Rust shell. `compose` with both `to` and
`body` runs the send path directly; everything else is emitted as a
Tauri event the WebView listens to.

## Design rationale: URL scheme vs. App Intents

Tauri 2 macOS apps don't have a first-class Swift target. Shipping a
proper App Intents bundle (`@AppIntent` + `AppShortcutsProvider`)
requires one of:

1. A separate Xcode project producing a `.appex` that we embed under
   `Contents/PlugIns/` and sign alongside the Tauri bundle.
2. A substantial `build.rs` integration that drives `xcrun swiftc` +
   `actool` from Cargo and stitches the resulting binary into the bundle.

Both are doable but non-trivial, and there's no user-facing win for the
extra surface: Shortcuts.app's **Open URLs** action is equivalent in
power to a shallow `@AppIntent`. A one-action shortcut named "Send a
Message With txt" is indistinguishable from a proper App Intent as far
as the Shortcuts library, Siri, and Spotlight are concerned. Third-party
launchers (Raycast, Alfred) also pick the URL scheme up for free.

The trade-off we accept: Shortcut parameters are typed as `Text` and
filled via `Ask for Input` actions rather than surfaced as native
parameter editors in the Shortcuts parameter list. The Shortcuts UX is
marginally less polished. If that becomes a real gripe we'll lift the
existing `compose` URL into a Swift App Intents bundle, keep the URL
scheme as the transport, and get the native UI for free — the backend
plumbing won't change.
