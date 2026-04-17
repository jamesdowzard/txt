# Textbridge × macOS Shortcuts

Textbridge exposes its actions to macOS Shortcuts (and any other URL-capable
automation tool) via the `textbridge://` URL scheme. There's no App Intents
bundle — Shortcuts.app's built-in **Open URLs** action is wired to the
scheme handler in `desktop/src-tauri/src/lib.rs`, so every Shortcut below is
a one-action recipe.

## Actions

| URL | Effect |
|-----|--------|
| `textbridge://compose?to=+61412345678&body=optional+text` | Opens Textbridge, shows the new-message overlay with the phone number prefilled. |
| `textbridge://search?q=dinner` | Opens Textbridge, focuses the search bar, types the query. |
| `textbridge://conversation/<conversation-id>` | Opens Textbridge and reveals a specific thread. |
| `textbridge://open` | Just brings Textbridge to the front. Useful as a global hotkey target. |

`body` on `compose` is accepted but not yet wired through to the composer
input — the overlay currently only prefills the phone. The scheme reserves
the key for a later pass.

## Recipes

### Send a text message to a contact

1. Open Shortcuts.app → **File → New Shortcut**.
2. Drop in these actions in order:
   - **Ask for Input** (prompt: "Phone number", type: Text)
   - **URL** → `textbridge://compose?to=[Provided Input]`
   - **Open URLs**
3. Name it "Text via Textbridge". It now appears in Spotlight, the menu bar,
   and can be triggered from Siri ("Text via Textbridge").

### Search all messages for a term

1. New Shortcut.
2. Actions:
   - **Ask for Input** (prompt: "Search Textbridge for…", type: Text)
   - **URL Encode**
   - **Text** → `textbridge://search?q=[URL-Encoded Text]`
   - **Open URLs**
3. Name it "Search Textbridge".

### Jump to a pinned conversation

1. Find the conversation ID — either via the MCP `list_conversations` tool
   or by opening the conversation in Textbridge and copying the
   `?conversation=…` value from the in-app URL bar (⌘L).
2. New Shortcut → **URL** → `textbridge://conversation/<paste-id-here>`
   → **Open URLs**.
3. Bind to a hotkey in **System Settings → Keyboard → Keyboard Shortcuts
   → Services** for a one-key jump.

## Testing from the shell

```bash
open 'textbridge://compose?to=+61412345678'
open 'textbridge://search?q=groceries'
open 'textbridge://conversation/conv-abc123'
open 'textbridge://open'
```

Each `open` hands the URL off to `LaunchServices`, which resolves
`CFBundleURLTypes` (registered via `plugins.deep-link.desktop.schemes` in
`desktop/src-tauri/tauri.conf.json`) and fires the Tauri deep-link event
the Rust shell listens on.

## Why URL-scheme instead of App Intents

Tauri 2 macOS apps don't have a first-class Swift target; embedding an
App Intents bundle requires either a separate Xcode project or a
substantial `build.rs` Swift-toolchain integration. The URL scheme covers
the same user-facing ground — Shortcuts.app's **Open URLs** is equivalent
in power to a shallow `@AppIntent`, and third-party automation (Raycast,
Alfred, keyboard-driven launchers) pick it up for free.

Moving to a proper App Intents bundle is tracked as a follow-up once the
build-system surgery is worth the lift.
