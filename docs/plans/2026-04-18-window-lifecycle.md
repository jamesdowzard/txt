# Window Lifecycle Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship the window-lifecycle chain from the Textbridge roadmap — P2 #22 detached conversation windows, P3 #35 reopen-last-conversation, P3 #32 global compose hotkey — and cut release v0.7.0.

**Architecture:** All three features orbit Tauri's window layer. Detached windows spawn a second `WebviewWindow` pointed at the existing Go backend with a `?mode=detached&conversation=<id>` URL; the frontend already has a deep-link path that selects a conversation, so "detached" is a thin CSS mode that hides the sidebar. Reopen-last stores the active conversation ID in `localStorage` (per-origin, persists across WKWebView launches) and replays it after initial conversation load. Global hotkey uses `tauri-plugin-global-shortcut` to register `⌥⇧T` at launch; on fire it focuses the main window and emits a `focus-compose` event the WebView reacts to.

**Tech Stack:** Tauri 2 (Rust + JS bridge), `tauri-plugin-global-shortcut`, existing Vite/Preact/HTM WebView (`web/src/legacy.js`), `localStorage` for state persistence, `tauri::WebviewWindowBuilder` for second window.

**Working branch:** `feature/window-lifecycle` (already created via `/f:new`). Every task commits here; `/f:ship` at the end.

---

## Phase 1 — Reopen-last-conversation (P3 #35)

Cheapest slice. No Rust changes. Two small edits to `web/src/legacy.js`. Ships standalone.

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 1. Persist `activeConvoId` on select | sonnet | sequential | none |
| 2. Restore on boot | sonnet | sequential | Task 1 |

### Task 1: Persist activeConvoId on select

**Files:**
- Modify: `web/src/legacy.js:2596-2610` (function `selectConversation`)
- Modify: `web/src/legacy.js:4941-4945` (function `deselectConversation`)

**Step 1: Add the persist helper near the top of `legacy.js`**

Insert after the existing `let activeConvoId = '';` declaration (near line 27):

```js
const LAST_CONVO_KEY = 'textbridge:last-convo-id';
function persistLastConvoId(id) {
  try {
    if (id) localStorage.setItem(LAST_CONVO_KEY, id);
    else localStorage.removeItem(LAST_CONVO_KEY);
  } catch (err) {
    // Private-mode / storage-disabled — feature is best-effort.
  }
}
```

**Step 2: Call `persistLastConvoId` from both selection entry points**

In `selectConversation` (line 2605), after `activeConvoId = convo.ConversationID;`:

```js
activeConvoId = convo.ConversationID;
persistLastConvoId(activeConvoId);
```

In `deselectConversation` (line 4944), after `activeConversation = null;`:

```js
activeConversation = null;
activeConvoId = '';
persistLastConvoId('');
```

**Step 3: Manual verification**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop
./scripts/dev --launch
```

Open a conversation. In the webview devtools console (⌥⌘I):
```js
localStorage.getItem('textbridge:last-convo-id')
```
Expected: the active conversation's ID. Switch conversation, re-check — value changes. Deselect (Esc) — value clears.

**Step 4: Commit**

```bash
git add web/src/legacy.js
git commit -m "persist activeConvoId to localStorage for reopen-last"
```

---

### Task 2: Restore last conversation on boot

**Files:**
- Modify: `web/src/legacy.js` — the initial-load site. Search for the first `loadConversations()` in the startup bootstrap (not the ones inside menu handlers or refresh paths).

**Step 1: Add a `restoreLastConversation` helper**

Alongside `persistLastConvoId`:

```js
function restoreLastConversation() {
  let saved = '';
  try {
    saved = localStorage.getItem(LAST_CONVO_KEY) || '';
  } catch (err) {
    return;
  }
  if (!saved) return;
  const match = conversationByID(saved);
  if (match) {
    revealAndSelectConversation(match);
  }
  // If not found, leave the key alone — the conversation may appear on
  // a later load (e.g. pagination). Don't clear.
}
```

**Step 2: Call after initial load, guarded against deep-link precedence**

At the startup `loadConversations()` site:

```js
await loadConversations();
if (!pendingDeepLinkConversationID && !activeConvoId) {
  restoreLastConversation();
}
```

Rationale: if launched via `textbridge://conversation/<id>`, the deep-link path has already set `pendingDeepLinkConversationID` and should win.

**Step 3: Manual verification**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop
./scripts/dev --launch
```

1. Open conversation A. Quit (⌘Q).
2. Relaunch. Expected: conversation A auto-selects after load.
3. Quit, relaunch with `open "textbridge-dev://conversation/<other-id>"` — deep-linked conversation wins.

**Step 4: Commit**

```bash
git add web/src/legacy.js
git commit -m "restore last-viewed conversation on launch (P3 #35)"
```

---

## Phase 2 — Global compose hotkey (P3 #32)

Adds a Tauri plugin + two Rust edits + one WebView listener. Independent of Phase 1 but reuses main-window focus patterns — keep Phase 1 merged in the branch first for a clean baseline.

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 3. Add plugin dependency + capability | sonnet | sequential | Phase 1 done |
| 4. Register hotkey in Rust | sonnet | sequential | Task 3 |
| 5. Handle `focus-compose` event in WebView | sonnet | sequential | Task 4 |

### Task 3: Add tauri-plugin-global-shortcut

**Files:**
- Modify: `desktop/src-tauri/Cargo.toml`
- Modify: `desktop/src-tauri/capabilities/default.json`

**Step 1: Add the dependency**

Append under `[dependencies]` in `desktop/src-tauri/Cargo.toml`:

```toml
tauri-plugin-global-shortcut = "2"
```

**Step 2: Grant the capability**

In `desktop/src-tauri/capabilities/default.json`, append to `permissions`:

```json
"global-shortcut:default",
"global-shortcut:allow-register",
"global-shortcut:allow-unregister"
```

**Step 3: Verify compilation**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop/src-tauri
cargo check
```
Expected: `Finished` (no errors).

**Step 4: Commit**

```bash
git add desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock desktop/src-tauri/capabilities/default.json
git commit -m "add tauri-plugin-global-shortcut dependency + capability"
```

---

### Task 4: Register ⌥⇧T hotkey in Rust

**Files:**
- Modify: `desktop/src-tauri/src/lib.rs`

**Step 1: Constants + imports**

At the top of `lib.rs`, alongside other `const` declarations:

```rust
const COMPOSE_HOTKEY_EVENT: &str = "textbridge://focus-compose";
```

Add to imports:

```rust
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};
```

**Step 2: Register the plugin in the builder chain**

Modify the `tauri::Builder::default()` chain (line 28-32):

```rust
let builder = tauri::Builder::default()
    .plugin(tauri_plugin_process::init())
    .plugin(tauri_plugin_shell::init())
    .plugin(tauri_plugin_updater::Builder::new().build())
    .plugin(tauri_plugin_deep_link::init())
    .plugin(
        tauri_plugin_global_shortcut::Builder::new()
            .with_handler(|app, shortcut, event| {
                if event.state() == ShortcutState::Pressed
                    && shortcut.matches(Modifiers::ALT | Modifiers::SHIFT, Code::KeyT)
                {
                    if let Some(main) = app.get_webview_window("main") {
                        let _ = main.show();
                        let _ = main.unminimize();
                        let _ = main.set_focus();
                    }
                    let _ = app.emit(COMPOSE_HOTKEY_EVENT, ());
                }
            })
            .build(),
    );
```

**Step 3: Register the shortcut in `setup`**

Inside `.setup(|app| { … })`, after the deep-link handler and before `notifications::init`:

```rust
let shortcut = Shortcut::new(Some(Modifiers::ALT | Modifiers::SHIFT), Code::KeyT);
if let Err(err) = app.global_shortcut().register(shortcut) {
    eprintln!("[global-shortcut] failed to register compose hotkey: {err}");
}
```

**Step 4: Verify compilation**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop/src-tauri
cargo check
```
Expected: `Finished`. If API drift, consult `cargo doc --open -p tauri-plugin-global-shortcut`.

**Step 5: Live verification**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop
./scripts/dev --launch
```
From another app, press ⌥⇧T. Expected: Textbridge-dev raises + focuses. WebView reaction comes in Task 5.

**Step 6: Commit**

```bash
git add desktop/src-tauri/src/lib.rs
git commit -m "register global compose hotkey Alt+Shift+T (P3 #32)"
```

---

### Task 5: WebView reacts to focus-compose event

**Files:**
- Modify: `web/src/legacy.js` — near the existing `textbridge://deep-link` listener (line 5194)

**Step 1: Confirm the compose input selector**

```bash
grep -nE "id=\"compose|composeInput|\\$compose" web/src/legacy.js | head -5
```
Update the selector in Step 2 to whatever matches.

**Step 2: Subscribe to the event**

Immediately after the deep-link `.listen(...)` block:

```js
if (window.__TAURI__ && window.__TAURI__.event) {
  window.__TAURI__.event
    .listen('textbridge://focus-compose', () => {
      if (activeConvoId) {
        const $compose = document.querySelector('#composeInput'); // replace with confirmed selector
        if ($compose) {
          $compose.focus();
          return;
        }
      }
      openNewMsg({ to: '', body: '' });
    })
    .catch(err => console.warn('textbridge: focus-compose listen failed', err));
}
```

**Step 3: Live verification**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop
./scripts/dev --launch
```
1. Focus another app.
2. Press ⌥⇧T.
3. Expected: Textbridge-dev focuses + compose input gets focus (or new-message overlay opens if no conversation active).

**Step 4: Commit**

```bash
git add web/src/legacy.js
git commit -m "handle focus-compose event in WebView (P3 #32)"
```

---

## Phase 3 — Detached conversation windows (P2 #22)

Touches Rust (spawn window), JS (context-menu + palette entries), CSS (detached mode).

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 6. Rust command to spawn detached window | sonnet | sequential | Phase 2 done |
| 7. Context-menu entry + invoke | sonnet | sequential | Task 6 |
| 8. Detached-mode CSS + auto-select | sonnet | sequential | Task 7 |
| 9. Cmd+K palette entry | haiku | sequential | Task 8 |

### Task 6: Rust command to open a detached window

**Files:**
- Modify: `desktop/src-tauri/src/lib.rs`
- Modify: `desktop/src-tauri/Cargo.toml`

**Step 1: Add dependencies**

In `Cargo.toml`:

```toml
urlencoding = "2"
url = "2"
```

**Step 2: Add the command function**

Near the bottom of `lib.rs` (above `check_for_update`):

```rust
#[tauri::command]
fn open_conversation_window(
    app: tauri::AppHandle,
    conversation_id: String,
) -> Result<(), String> {
    let label = format!(
        "detached-{}",
        conversation_id.chars().filter(|c| c.is_ascii_alphanumeric()).collect::<String>()
    );

    if let Some(existing) = app.get_webview_window(&label) {
        existing.show().map_err(|e| e.to_string())?;
        existing.unminimize().map_err(|e| e.to_string())?;
        existing.set_focus().map_err(|e| e.to_string())?;
        return Ok(());
    }

    let url = format!(
        "{}/?mode=detached&conversation={}",
        BACKEND_ORIGIN,
        urlencoding::encode(&conversation_id),
    );

    tauri::WebviewWindowBuilder::new(
        &app,
        &label,
        tauri::WebviewUrl::External(url.parse().map_err(|e: url::ParseError| e.to_string())?),
    )
    .title("Textbridge — Conversation")
    .inner_size(640.0, 760.0)
    .min_inner_size(480.0, 500.0)
    .resizable(true)
    .build()
    .map_err(|e| e.to_string())?;

    Ok(())
}
```

**Step 3: Register the command in the builder**

Before `.build(tauri::generate_context!())`:

```rust
.invoke_handler(tauri::generate_handler![open_conversation_window])
```

**Step 4: Verify compilation**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop/src-tauri
cargo check
```
Expected: `Finished` clean.

**Step 5: Commit**

```bash
git add desktop/src-tauri/src/lib.rs desktop/src-tauri/Cargo.toml desktop/src-tauri/Cargo.lock
git commit -m "add open_conversation_window Tauri command (P2 #22)"
```

---

### Task 7: Context-menu entry "Open in new window"

**Files:**
- Modify: `web/src/legacy.js` — `conversationContextMenuItems` factory (around line 1960)
- Modify: the context-menu action dispatcher (around line 4580)

**Step 1: Add the menu item**

Append to `conversationContextMenuItems`:

```js
{
  label: 'Open in new window',
  action: 'detach',
  handler: async (convo) => {
    if (!window.__TAURI__) return;
    try {
      await window.__TAURI__.core.invoke('open_conversation_window', {
        conversationId: convo.ConversationID,
      });
    } catch (err) {
      console.warn('textbridge: open_conversation_window failed', err);
    }
  },
},
```

Match the shape to the existing items (pin/mute/archive). If the factory uses a different shape, copy that one's structure.

**Step 2: Wire the dispatcher**

In the `switch(action)` near line 4580, add a case that calls the handler (if the factory's items don't already self-dispatch).

**Step 3: Live verification**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop
./scripts/build-sidecar && ./scripts/dev --launch
```
Right-click a conversation → "Open in new window". Expected: second window opens with that conversation. Right-click same conversation again → focuses existing detached window.

**Step 4: Commit**

```bash
git add web/src/legacy.js
git commit -m "context-menu: open conversation in new window (P2 #22)"
```

---

### Task 8: Detached-mode CSS + JS scoping

**Files:**
- Modify: `web/src/legacy.js` — boot path (near top-of-file init)
- Modify: `web/src/legacy.css`

**Step 1: Detect detached mode on boot**

Near top of `legacy.js`:

```js
const urlParams = new URLSearchParams(window.location.search);
const DETACHED_MODE = urlParams.get('mode') === 'detached';
const DETACHED_CONVO = urlParams.get('conversation') || '';

if (DETACHED_MODE) {
  document.body.classList.add('detached-mode');
}
```

**Step 2: Auto-select target conversation in detached mode**

Extend the Task 2 initial-load hook:

```js
await loadConversations();
if (DETACHED_MODE && DETACHED_CONVO) {
  const match = conversationByID(DETACHED_CONVO);
  if (match) revealAndSelectConversation(match);
} else if (!pendingDeepLinkConversationID && !activeConvoId) {
  restoreLastConversation();
}
```

**Step 3: Confirm sidebar/chat selectors**

```bash
grep -nE "id=\"sidebar|class=\"sidebar|id=\"chatArea|class=\"chat-area" web/src/legacy.js web/index.html | head -10
```
Update the CSS in Step 4 to actual selectors.

**Step 4: Detached-mode CSS**

Append to `web/src/legacy.css`:

```css
.detached-mode #sidebar,
.detached-mode .sidebar,
.detached-mode #folderTabs {
  display: none !important;
}
.detached-mode #chatArea,
.detached-mode .chat-area {
  width: 100% !important;
  max-width: 100%;
}
```

**Step 5: Live verification**

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle/desktop
./scripts/build-sidecar && ./scripts/dev --launch
```
Right-click → Open in new window. Expected: sidebar hidden, conversation fills the window.

**Step 6: Commit**

```bash
git add web/src/legacy.js web/src/legacy.css
git commit -m "detached mode: hide sidebar + auto-select from URL (P2 #22)"
```

---

### Task 9: Cmd+K palette entry

**Files:**
- Modify: `web/src/legacy.js` — `cmdkActions` (around line 5284)

**Step 1: Add per-conversation palette entry**

Match the pattern of pin/mute/archive per-conversation actions:

```js
{
  id: `detach:${c.ConversationID}`,
  title: `Open "${conversationDisplayName(c)}" in new window`,
  icon: 'window',
  run: () => {
    closeCommandPalette();
    if (!window.__TAURI__) return;
    window.__TAURI__.core.invoke('open_conversation_window', {
      conversationId: c.ConversationID,
    }).catch(err => console.warn('textbridge: open_conversation_window failed', err));
  },
},
```

**Step 2: Live verification**

Dev app → ⌘K → type contact name → "Open … in new window" → Enter. Expected: detached window opens.

**Step 3: Commit**

```bash
git add web/src/legacy.js
git commit -m "cmd+K: open conversation in new window entry (P2 #22)"
```

---

## Phase 4 — Release v0.7.0

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 10. Ship feature branch | manual | — | Phases 1-3 |
| 11. Cut v0.7.0 | manual | — | Task 10 |
| 12. Close roadmap items | manual | — | Task 11 |

### Task 10: Ship

```bash
cd ~/code/personal/textbridge/.worktrees/window-lifecycle
/f:ship
```

### Task 11: Cut v0.7.0

```bash
cd ~/code/personal/textbridge/desktop
./scripts/release minor
```
Wait ~5-10 min for notarisation. Script auto-bumps, PRs version commit, merges, tags, publishes `latest.json`.

**Verification:**

```bash
spctl -a -vv /Applications/Textbridge.app
# Expected: accepted, source=Notarized Developer ID
```

### Task 12: Roadmap hygiene

Comment on issue #39 with the three shipped items + `v0.7.0` tag. Don't close — remaining P2/P3/follow-ups still tracked there. Close #7 and #18 (stale trackers).

---

## Summary

- **12 tasks** across 4 phases.
- **Execution strategy:** sequential within the feature branch. `web/src/legacy.js` is the shared-touch file across almost every task — don't parallelise (issue #39's merge-conflict warning).
- **Complexity:** Medium. Rust plugin registration + detached-window spawn are the unfamiliar pieces.
- **Estimated effort:** 2-3 h coding + 10 min release cut.
- **Risk areas:**
  - `tauri-plugin-global-shortcut` API drift between Tauri 2 minor versions — verify `Shortcut::matches` signature against `cargo doc` if Step 4/Task 4 fails to compile.
  - Detached-mode CSS selectors depend on actual monolith IDs — grep (Task 8 Step 3) before trusting the defaults.
  - Compose input selector in Task 5 is a guess — confirm (Task 5 Step 1) before shipping.

---

*Plan authored 2026-04-18. References: roadmap issue #39, existing deep-link implementation in `desktop/src-tauri/src/lib.rs` + `web/src/legacy.js::handleDeepLink`, issue #39's suggested-next-steps ordering.*
