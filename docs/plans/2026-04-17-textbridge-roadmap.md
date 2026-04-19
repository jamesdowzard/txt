# Textbridge Roadmap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to execute Phase 0 task-by-task. For P1+ items, each scope card is a self-contained session — start with `/f:new tb-<slug>`, read the card, verify the "groundwork" claim against current code, then plan bite-sized tasks within that session.

**Goal:** Ship the 37-item roadmap from issue #7 in dependency order — Phase 0 modularisation first (foundation), then P1 → P2 → P3 items as individual PRs.

**Architecture:** Textbridge is a Tauri 2 macOS shell (Rust) hosting a WKWebView that loads the Go backend's embedded web UI at `http://127.0.0.1:7007`. Current frontend is a single ~8.6k-line `internal/web/static/index.html`. Backend is Go + SQLite + libgm. Google-only mode. MCP at `/mcp/sse`.

**Tech stack:** Tauri 2, Rust, Go 1.22+, SQLite, libgm (`go.mau.fi/mautrix-googlechat`), `go:embed`, Vite + TypeScript (post Phase 0), UnoCSS or vanilla CSS tokens.

**Scope discipline — read this first:**
- This document is Phase 0 in full TDD detail **plus** one scope card per P1/P2/P3 item.
- Each P1+ scope card is a session prompt, not a bite-sized plan. Fresh session + `/f:new` per item. Expand the card into TDD steps at the start of that session.
- Do not execute multiple P1 items in one session. One item, one PR.
- **Before any item: re-read the referenced files.** The groundwork claims in issue #7 were a snapshot. Verify each claim against current code before assuming anything is "already done".

---

## Dependency graph

```
Phase 0 (WebView modularisation) ──┬──► P1 items 1-11 (UI-heavy, benefit most from modularisation)
                                   └──► P1 #12 auto-update (Tauri-side, independent of Phase 0)

P1 #1 archive ──► P2 #13 mark unread, #14 delete, #16 snooze (share folder/state plumbing)
P1 #2 pin     ──► (independent)
P1 #3 mute    ──► P2 #13, P3 #33 VIP (share notification suppression)
P1 #4 reactions ──► (independent, render only)
P1 #5 reply threading ──► P2 #27 MCP summarise (needs thread traversal)
P1 #7 scheduled send ──► (new table, independent)
P1 #8 command palette ──► touches many UI pieces; do after 2-3 P1s land
P1 #9 notification reply ──► P1 #3 mute must exist (bypass rule)
P1 #10 Shortcuts.app ──► P1 #11 URL scheme (both use App Intents / deep-link plumbing)

P2 #24 unify SMS+iMessage ──► (display-only, independent; gate on P1 #2 pin for UI)
P2 #25 DB backups ──► (independent, no UI)
P2 #27 MCP tools ──► no Phase 0 dependency
```

**Recommended order:** Phase 0 → P1 #1 (archive) → P1 #4 (reactions) → P1 #5 (reply) → P1 #12 (auto-update, can run in parallel with any P1 since it's Tauri-only) → P1 #2 (pin) → P1 #3 (mute) → rest of P1 → P2 → P3.

---

## Pre-flight checklist (every session)

Before starting *any* item in this plan:

1. `cd ~/code/personal/textbridge && git pull origin main`
2. `/f:new tb-<slug>` — slug matches the item title, e.g. `tb-archive-unarchive`.
3. Read `CLAUDE.md`, `desktop/CLAUDE.md`, and any file this card references — at current `HEAD`, not the snapshot in issue #7.
4. Re-verify any "groundwork" claim before building on it. Grep for the symbol; read the function.
5. For UI items (all of P1 except #10/#11/#12), **stand up dev first**: `cd desktop && ./scripts/dev --launch`. Keep the app running and reload (⌘R in Web Inspector) as you change the embedded HTML.
6. Ship via `/f:ship`. If user-visible, tag a release: `cd desktop && ./scripts/release patch`.

---

# Phase 0 — WebView modularisation

> **Status (2026-04-17):** **Landed on `feature/tb-phase0-modularise` via lift-and-shift.** Branch has 5 commits — plan, Task 0.1 (Vite scaffold), 0.2 (Go embed → dist), 0.3 (lift-and-shift), 0.6 (delete monolith + docs). Tasks 0.4 (component extraction) and 0.5 (state migration) are **deferred** and reshaped into *"incremental extraction from `web/src/legacy.js`"*, interleaved with P1 UI work. Ship this branch (`/f:ship`) before starting any P1 item. Release cut (`./scripts/release patch` → v0.2.0) deferred to user (notarisation is hands-on).

**Why first:** Every P1 UI item edits `internal/web/static/index.html` (8,615 lines). Without a module system, each P1 item is a painful diff and review slog. Splitting once makes 10+ downstream PRs meaningfully cheaper.

**Outcome:** `internal/web/static/` becomes a Vite build output. Source lives in a new `web/` directory at repo root, built to `internal/web/static/dist/`, embedded via `go:embed`. The runtime contract (same HTTP routes, same `/` entry, same image-probe readiness check) is unchanged — only the source layout changes.

**Non-goals for Phase 0:**
- No new features. Pure refactor.
- No framework choice beyond what's needed. Use **Preact + HTM + UnoCSS** (lightweight, no JSX compile for simple components, tiny runtime). If that feels wrong in practice, pivot to **Lit** — don't get into frameworks debate.
- Do not rewrite CSS tokens (`:root` block at index.html:9-40). Copy verbatim into a `tokens.css`.

**Risk:** The WebView's `window.location.replace('http://127.0.0.1:7007')` navigation means the Tauri shell's probe must still succeed. Verify by running `./scripts/dev --launch` at every major step.

---

### Task 0.1: Set up Vite scaffold + package.json

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`, `web/.gitignore`
- Create: `web/src/main.ts`, `web/src/index.html`
- Create: `web/src/styles/tokens.css`

**Step 1: Create `web/package.json`**

```json
{
  "name": "textbridge-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "preact": "^10.22.0",
    "htm": "^3.1.1"
  },
  "devDependencies": {
    "@unocss/preset-mini": "^0.59.0",
    "typescript": "^5.4.0",
    "unocss": "^0.59.0",
    "vite": "^5.2.0"
  }
}
```

**Step 2: Create `web/vite.config.ts`**

```ts
import { defineConfig } from 'vite';
import UnoCSS from 'unocss/vite';
import { presetMini } from '@unocss/preset-mini';
import { resolve } from 'path';

export default defineConfig({
  plugins: [UnoCSS({ presets: [presetMini()] })],
  build: {
    outDir: resolve(__dirname, '../internal/web/static/dist'),
    emptyOutDir: true,
    rollupOptions: {
      input: resolve(__dirname, 'src/index.html'),
    },
  },
  server: {
    proxy: {
      // Dev-time proxy — Vite dev server fronts the Go backend
      '^/(api|messages|conversations|contacts|mcp|events|favicon\\.svg)': {
        target: 'http://127.0.0.1:7007',
        changeOrigin: true,
      },
    },
  },
});
```

**Step 3: Copy CSS tokens**

Read lines 9–40 from `internal/web/static/index.html` (the `:root` block plus `* { margin: 0; ... }` reset + `html, body` base). Paste verbatim into `web/src/styles/tokens.css`. Do not rename tokens.

**Step 4: Create stub entry**

`web/src/index.html`:
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Textbridge</title>
  <link rel="icon" type="image/svg+xml" href="/favicon.svg">
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.ts"></script>
</body>
</html>
```

`web/src/main.ts`:
```ts
import 'uno.css';
import './styles/tokens.css';
import { render } from 'preact';
import { html } from 'htm/preact';

render(
  html`<div class="p-4 text-[var(--text-primary)]">Textbridge — Vite scaffold OK</div>`,
  document.getElementById('app')!,
);
```

**Step 5: Verify build outputs land in `internal/web/static/dist/`**

Run: `cd web && bun install && bun run build`
Expected: `internal/web/static/dist/index.html`, `assets/*.js`, `assets/*.css` created. No errors.

**Step 6: Commit**

```bash
git add web/ internal/web/static/dist/.gitkeep
git commit -m "phase0: add Vite + Preact scaffold under web/"
```

---

### Task 0.2: Switch Go embed to serve the Vite dist

**Files:**
- Modify: `internal/web/static.go` (or wherever `//go:embed static` lives — grep to confirm path)
- Modify: `.gitignore` — ensure `internal/web/static/dist/` is committed (it's the build output that ships)
- Modify: `cmd/serve.go` if it references `static/index.html` directly

**Step 1: Write failing test**

Create `internal/web/static_test.go`:
```go
package web

import (
    "strings"
    "testing"
)

func TestEmbedServesDistIndex(t *testing.T) {
    f, err := StaticFS.Open("dist/index.html")
    if err != nil { t.Fatalf("open dist/index.html: %v", err) }
    defer f.Close()
    buf := make([]byte, 512)
    n, _ := f.Read(buf)
    if !strings.Contains(string(buf[:n]), `<div id="app">`) {
        t.Fatalf("dist/index.html missing #app root: %s", buf[:n])
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/web/ -run TestEmbedServesDistIndex -v`
Expected: FAIL — either `StaticFS` doesn't exist or path wrong.

**Step 3: Update embed directive + HTTP handler**

Change the embed in the package-level file:
```go
//go:embed all:static
var StaticFS embed.FS
```

Update the HTTP handler that serves `/` to rewrite requests to `dist/`:
```go
// in the routing setup (grep for "static/index.html" or "StaticFS"):
distFS, _ := fs.Sub(StaticFS, "static/dist")
http.Handle("/", http.FileServer(http.FS(distFS)))
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/web/ -run TestEmbedServesDistIndex -v`
Expected: PASS.

**Step 5: Verify end-to-end via dev shell**

```bash
cd web && bun run build
cd .. && go build -o textbridge .
./scripts/dev --launch   # from desktop/
```

Expected: Textbridge-dev opens, shows the "Vite scaffold OK" placeholder. No console errors in Web Inspector.

**Step 6: Commit**

```bash
git add internal/web/ web/
git commit -m "phase0: serve Vite dist via go:embed"
```

---

### Task 0.3: Migrate shell layout (sidebar + main pane) to Preact components

**Files:**
- Create: `web/src/components/App.ts`, `Sidebar.ts`, `MainPane.ts`, `ConversationList.ts`
- Modify: `web/src/main.ts` to render `<App />`
- Source: copy-paste HTML structure from `internal/web/static/index.html` lines ~300-1000 (the `.app > .sidebar + .main` shell)

**Step 1: Identify the shell.** Open `internal/web/static/index.html`, find the `<body>` root — the outermost `<div class="app">` is the shell. Take that subtree, its children, and their immediate style references.

**Step 2: Write component test** (Preact + `@testing-library/preact`, add to devDeps if not present):

`web/src/components/App.test.ts`:
```ts
import { render, screen } from '@testing-library/preact';
import { html } from 'htm/preact';
import { App } from './App';

test('App renders sidebar and main pane', () => {
  render(html`<${App} />`);
  expect(screen.getByTestId('sidebar')).toBeInTheDocument();
  expect(screen.getByTestId('main-pane')).toBeInTheDocument();
});
```

**Step 3: Run (expect FAIL)** — component doesn't exist.

**Step 4: Implement `App`, `Sidebar`, `MainPane`** as functional components. Keep markup identical to the source; just split files. Do not refactor styles yet.

**Step 5: Run tests (expect PASS) + visual parity check via `./scripts/dev --launch`.**

**Step 6: Commit**

```bash
git commit -m "phase0: extract shell layout into Preact components"
```

---

### Task 0.4: Migrate per-feature regions

Split the remaining monolith into these components (one sub-commit each, same TDD pattern as Task 0.3):

| Component | Source lines (approx, verify) | Notes |
|-----------|-------------------------------|-------|
| `ConversationListItem.ts` | row markup inside `.sidebar` list | takes `Conversation` prop |
| `MessageView.ts`          | `.main` message scroll container | renders list of `Message` |
| `MessageBubble.ts`        | individual message row | reads `is_from_me`, `status` |
| `Composer.ts`             | bottom compose box | textarea + send button |
| `Header.ts`               | top bar (conversation name, actions) | |
| `StatusFooter.ts`         | connection / backfill progress indicator | subscribes to `/events` SSE |
| `Modals.ts`               | any existing modals (new message, pair) | |

**Per-component checklist** (apply to each):
1. Grep current `index.html` for the component's root class or id.
2. Create `web/src/components/<Name>.ts` + `.test.ts`.
3. Move HTML verbatim, swap into Preact `html\`\`` template.
4. Move component-scoped CSS into `web/src/styles/<name>.css`, import from the component. Shared styles stay in `tokens.css`.
5. Wire into the parent component.
6. Visual parity check.
7. Commit: `phase0: extract <Name> component`.

---

### Task 0.5: Migrate state + data fetching

**Files:**
- Create: `web/src/state/store.ts` — minimal Preact signals (`@preact/signals`) or a custom `useReducer`-backed store. No Redux.
- Create: `web/src/api/client.ts` — typed wrappers for REST routes under `/messages`, `/conversations`, `/contacts`.
- Create: `web/src/api/events.ts` — SSE subscription to `/events` (typed event union).

**Step 1: Grep existing `index.html` for `fetch(` and `new EventSource(` — enumerate endpoints.**

**Step 2: Define `ApiClient` with methods mirroring each endpoint. Strict types from `internal/db` Go structs (hand-translated JSON shapes).**

**Step 3: Replace inline fetch calls in components with `apiClient.*` methods.**

**Step 4: Tests — mock `fetch` via MSW or a simple stub, assert components render API data.**

**Step 5: End-to-end dev-shell check: list + open conversation, send message still works.**

**Step 6: Commit per migration (one per endpoint).**

---

### Task 0.6: Delete the monolith + update docs

**Files:**
- Delete: `internal/web/static/index.html` (the monolith)
- Keep: `internal/web/static/favicon.svg`, `internal/web/static/dist/` (build output)
- Modify: `CLAUDE.md` — update "Key files" to point to `web/src/` + `dist/`
- Modify: `desktop/CLAUDE.md` — note Vite dev proxy option for fast iteration
- Modify: `README.md` — dev loop now includes `cd web && bun run build` before `./scripts/build-sidecar` (or add it to `build-sidecar`)

**Step 1: Grep repo for `static/index.html` references — expect 0 hits after this commit.**

**Step 2: Update `scripts/build-sidecar` to call `cd ../web && bun run build` before `go build`** (the released binary must embed the dist).

**Step 3: Verify release path**:
```bash
cd desktop && ./scripts/release patch
spctl -a -vv /Applications/Textbridge.app   # expect Notarized Developer ID
open /Applications/Textbridge.app            # smoke: list shows, messages render
```

**Step 4: Commit**

```bash
git commit -m "phase0: delete monolithic index.html, update docs + build"
```

**Step 5: Merge via `/f:ship`. Tag a release (`./scripts/release patch` → v0.2.0 because architecture change).**

---

**Phase 0 done-when:**
- `web/src/` is the source of truth; every component has a test.
- `internal/web/static/` contains only `dist/` + `favicon.svg`.
- Dev loop: `cd desktop && ./scripts/dev --launch` works; Vite dev proxy works at `http://localhost:5173`.
- Release: `./scripts/release patch` produces a signed, notarised `.app` with identical behaviour to v0.1.1.

---

# P1 — daily essentials (12 items)

Each card below is a **session prompt**. Run one per `/f:new` session. Expand into bite-sized TDD steps inside that session before coding.

---

## P1 #1 — Archive / unarchive conversations

**`/f:new tb-archive-unarchive`**

**Verify first:** `rg 'is_archived|folder.*column' internal/db/` — at writing time there was **no** folder column on `conversations`. The issue's "schema already has is_archived capacity" hint is wrong. Confirm before proceeding.

**Files:**
- Modify: `internal/db/db.go:175-240` — add `ALTER TABLE conversations ADD COLUMN folder TEXT NOT NULL DEFAULT 'inbox'` to the migration block.
- Modify: `internal/db/conversations.go` — add `SetFolder(convID, folder string) error`, update list queries to filter by folder, extend `Conversation` struct.
- Modify: `internal/app/backfill.go:95-113` — pass the folder enum down into `paginateFolder` and persist it on each conversation via `storeConversation`.
- Modify: `internal/client/*.go` — if `ArchiveConversation`/`UnarchiveConversation` wrappers don't exist around `gmproto.UpdateConversationRequest_ARCHIVE`/`UNARCHIVE`, add them.
- Modify: `internal/tools/list_conversations.go` — accept optional `folder` param.
- Add: `internal/tools/archive_conversation.go` — MCP tool `archive_conversation`.
- UI: new `web/src/components/FolderTabs.ts` (Inbox / Archive / Spam), wire archive button into `Header.ts`.

**Tasks:**
1. DB migration + struct changes (TDD: `conversations_test.go`).
2. libgm client wrappers + backend handler (`POST /conversations/:id/archive` and `unarchive`).
3. Backfill fix: record `folder` when iterating `INBOX` / `ARCHIVE` / `SPAM_BLOCKED`.
4. MCP tool + registration in `internal/tools/tools.go`.
5. UI: folder tabs in sidebar + archive action in header.
6. Live-sync: incoming archive events from Google (if libgm emits them) flip the row.

**Verification:**
- `go test ./internal/db/ ./internal/app/ -v`
- Manual: archive a conversation in the app → row moves to Archive tab; unarchive → returns to Inbox. Check the remote Google Messages web UI shows it archived too.

**Out of scope:** Bulk archive. Smart auto-archive rules.

---

## P1 #2 — Pin conversations to top

**`/f:new tb-pin-conversations`**

**Files:**
- Modify: `internal/db/db.go` — `ALTER TABLE conversations ADD COLUMN pinned_at INTEGER NOT NULL DEFAULT 0`.
- Modify: `internal/db/conversations.go` — order by `pinned_at DESC, last_message_ts DESC`. Add `SetPinned(convID string, pinned bool)`.
- Modify: `internal/tools/list_conversations.go` — include `pinned_at` in response, honour `include_pinned` sort.
- Add: backend route `POST /conversations/:id/pin`, `DELETE /conversations/:id/pin`.
- UI: pin/unpin action in row context menu; pinned rows render with a subtle marker + sort to top.

**Verification:** `go test ./internal/db/ -v` + manual: pin 3 conversations, confirm they stick to the top regardless of `last_message_ts`.

**Out of scope:** Remote sync — pin state is local-only by design (libgm has no pin concept).

---

## P1 #3 — Per-conversation mute (with optional duration)

**`/f:new tb-mute-conversation`**

**Files:**
- Existing: `conversations.notification_mode` column already exists (`db.go:185`). Leverage it.
- Modify: `internal/db/conversations.go` — add `SetNotificationMode(convID string, mode string, until *int64)` and add `muted_until INTEGER DEFAULT 0` column.
- Modify: `internal/notify/macos.go` — before delivering a notification, look up the conversation's mode; skip if `muted` and `muted_until == 0 OR muted_until > now()`.
- UI: menu in header with options (1h / 8h / until tomorrow / forever / unmute).

**Verification:** Mute → send self a message → no notification. `notify-test` helper if one exists.

**Out of scope:** Remote sync. Per-keyword mute.

---

## P1 #4 — Reactions UI (render + send)

**`/f:new tb-reactions-ui`**

**Verify first:** `rg 'ExtractReactions' internal/client/` + open `internal/db/messages.go` for `Reactions` column. Issue #7 claims it's populated — confirm non-empty on a recent reacted message: `sqlite3 messages.db "SELECT reactions FROM messages WHERE reactions != '' LIMIT 5"`.

**Files:**
- Read-only: `internal/client/reactions.go` (or wherever `ExtractReactions` lives) to understand the JSON shape.
- Add: backend route `POST /messages/:id/react` with body `{"emoji":"👍"}` — calls libgm `SendReaction`.
- Modify: `internal/tools/send_message.go` or add new `internal/tools/react_to_message.go`.
- UI: `web/src/components/ReactionBar.ts` — long-press / hover to surface emoji picker; badge row under each message bubble.

**Verification:** Send 👍 from Textbridge → phone shows reaction. Receive reaction from phone → UI badge updates live via SSE.

---

## P1 #5 — Reply-to threading in UI

**`/f:new tb-reply-threading`**

**Verify first:** `rg 'ReplyToID|reply_to_id' internal/` — confirm `ReplyToID` is populated on at least some messages. If empty on all recent messages, the groundwork claim is stale and this needs backend work first.

**Files:**
- Add: backend route `POST /messages/:id/reply` — accepts `{"body":"…","reply_to":"<parent_id>"}`.
- Modify: libgm send path to include the `reply_to` protobuf field.
- UI: `MessageBubble.ts` — render quoted parent preview above the bubble; click-to-scroll to parent. Composer shows a "Replying to …" chip with ✕ to cancel.

**Verification:** Reply to a message; phone shows the thread; reloading Textbridge preserves the thread.

---

## P1 #6 — Drag-and-drop + clipboard image paste

**`/f:new tb-media-paste`**

**Files:**
- Modify: `web/src/components/Composer.ts` — handle `paste` event, `dragover`/`drop`, `<input type="file" accept="image/*,video/*">` fallback.
- Add: backend route `POST /media/upload` returning a media ID that `send_message` accepts.
- Modify: libgm send path to attach media.

**Verification:** ⌘V an image → preview → send → phone receives it.

**Out of scope:** File type restrictions beyond image/video (no arbitrary files for now).

---

## P1 #7 — Scheduled send + outbox

**`/f:new tb-scheduled-send`**

**Files:**
- Add: `internal/db/outbox.go` — new table `outbox(id, conversation_id, body, media_id, send_at, status, attempts, error)`.
- Add: `internal/app/outbox.go` — ticker that scans `WHERE status='pending' AND send_at <= now()` every 10s and dispatches via existing send path.
- Add: backend route `POST /outbox`, `GET /outbox`, `DELETE /outbox/:id`.
- Add MCP tool: `schedule_message`.
- UI: Composer gets a "Schedule…" dropdown (now / in 1h / tomorrow 9am / custom). Outbox view in sidebar footer.

**Verification:** Schedule a message 2 min out, leave app running, confirm it fires. Kill the backend mid-window, restart, confirm it still fires (persistence works).

---

## P1 #8 — Cmd+K command palette

**`/f:new tb-command-palette`**

**Files:**
- Add: `web/src/components/CommandPalette.ts` with fuzzy matcher (`fuzzysort` or hand-rolled trigram).
- Registry: commands = `{ switch_to_conversation, archive_current, mute_current, compose_new, … }`.
- Keybind: global ⌘K listener in `main.ts`.

**Verification:** ⌘K → type part of a name → Enter switches. ⌘K → "arch" → runs archive on current conversation.

**Defer until:** At least 2 P1 items beyond archive/pin are shipped so the palette has commands worth indexing.

---

## P1 #9 — Reply from native macOS notification

**`/f:new tb-notification-reply`**

**Files:**
- Modify: `desktop/src-tauri/src/lib.rs` — register `UNNotificationCategory` with a `UNTextInputNotificationAction`.
- Add: Rust handler that decodes the reply and posts to `http://127.0.0.1:7007/messages/:conv_id/send`.
- Verify `NSUserNotification` entitlements aren't blocked by the unsandboxed entitlements plist.

**Verification:** Receive a message → banner appears → click "Reply" → type → phone receives.

**Depends on:** P1 #3 (mute) exists so muted conversations don't surface the reply UI.

---

## P1 #10 — macOS Shortcuts.app App Intents

**`/f:new tb-shortcuts-intents`**

**Files:**
- Add: `desktop/src-tauri/src/intents.rs` (or a small Swift shim via `swift-bridge`) exposing `SendMessage`, `SearchMessages`, `OpenConversation`.
- Register intents in `Info.plist` via Tauri's bundler config or a post-build xcode step.

**Verification:** Shortcuts.app → New Shortcut → "Send Message via Textbridge" visible.

---

## P1 #11 — URL scheme `textbridge://compose?to=…&body=…`

**`/f:new tb-url-scheme`**

**Files:**
- Modify: `desktop/src-tauri/tauri.conf.json` — register `deepLink` plugin + scheme `textbridge`.
- Add: Rust handler parsing query, emits event to WebView.
- Modify: `web/src/main.ts` — listener navigates to compose view pre-filled.

**Verification:** `open 'textbridge://compose?to=+61…&body=hi'` → Textbridge opens with compose pre-filled.

**Depends on:** P1 #10 logic overlaps (both are entry points); do them back-to-back.

---

## P1 #12 — Auto-update via Tauri updater plugin

**`/f:new tb-auto-update`**

**Does not depend on Phase 0** — this is a Rust-side + release-pipeline change.

**Files:**
- Modify: `desktop/src-tauri/Cargo.toml` — add `tauri-plugin-updater`.
- Modify: `desktop/src-tauri/tauri.conf.json` — updater config, public key for signature.
- Modify: `desktop/scripts/release` — generate + sign `latest.json`, upload to release endpoint (GitHub Releases or a Cloudflare R2 bucket; pick one).
- Keys: store updater private key in macOS Keychain under account `textbridge-updater`.

**Verification:** Cut a fake `0.1.2` release → install `0.1.1` → wait for auto-update prompt → accepts and restarts on the new version.

---

# P2 — high-leverage (15 items)

Scope cards are deliberately lighter — flesh out at session time.

**13. Mark unread** — `conversations.unread_count`; toggle + push to Google via libgm `UpdateConversation`. UI: right-click → Mark as unread.

**14. Delete conversation (local + remote)** — libgm `DeleteConversation`; DB cascade delete messages; UI confirm modal. Danger zone: confirm copy must match `/f:new` rule "destructive actions need user confirmation".

**15. Local nicknames** — new `nicknames(conversation_id, display_name)` table; overrides `conversations.name` in the read path; editable from Header.

**16. Snooze** — reuses P1 #1 archive + `muted_until` plumbing. Job: scan for `snoozed_until <= now()` and unarchive.

**17. Search filters** — extend `internal/tools/search_messages.go` + backend `/search` route with `date_from`, `date_to`, `sender`, `has_media`, `has_link`.

**18. Jump to date** — calendar popover in Header; scrolls message view to first message on/after selected date.

**19. Per-conversation media gallery** — `/conversations/:id/media` route; grid of thumbnails; click → QuickLook (`open -a Preview`).

**20. Voice memo recording** — AVFoundation via Rust; records .m4a; uploads via `/media/upload` (from P1 #6).

**21. Audio playback waveform** — Web Audio API in `MessageBubble.ts` when `mime_type` is audio; playback speed 1×/1.5×/2×.

**22. Detached conversation windows** — Tauri `WebviewWindow::new`; share IPC with main window for state sync.

**23. Menu-bar quick-reply popover** — `tauri-plugin-positioner` + status item; small composer; no conversation switcher.

**24. Unify SMS + iMessage threads per contact** — read path only. `internal/db/conversations.go` gets a `listUnified()` that groups by normalised phone number + display-only merge. Needs phone-number normalisation helper (E.164). Do not mutate rows.

**25. Automatic timestamped DB backups + restore wizard** — cron in `internal/app/backup.go`; writes `messages.db.bak.YYYYMMDD` to data dir; keeps last 7. Restore: pick a backup → swaps db file after confirm modal.

**26. Export conversation** — `/conversations/:id/export?format=json|html|txt|mbox` route; UI: Export… menu → file save dialog via Tauri's `dialog` API.

**27. New MCP tools:**
  - `summarize_conversation` — calls Anthropic API with user-supplied key (prompt-cache enabled per global rule); returns summary + action items.
  - `find_unanswered` — SQL: threads where `last_message.is_from_me = 0 AND now() - last_message_ts > ?`.
  - `suggest_reply` — Anthropic API; input = last N messages; output = 3 draft replies; opt-in BYO key.
  - `extract_action_items` — Anthropic API; structured JSON output (`[{item, due, assignee?}]`).

**P2 verification pattern:** Per item, add at least one Go test for backend changes and one Preact component test for UI changes. All MCP tools must be listed in `internal/tools/tools.go` registry and round-trip through the `TEXTBRIDGE_MCP_TOOLS=all` env var.

---

# P3 — polish & extras (10 items)

One-paragraph scope each.

**28. Spam folder UI** — reuses P1 #1 folder tabs; add third tab; same archive/unarchive semantics against SPAM_BLOCKED folder.

**29. Group chat management** — libgm `UpdateConversation` (rename), `LeaveConversation`; UI: Header menu for groups.

**30. Link multiple phone numbers to one identity** — manual merge UI → writes `unified_contacts(unified_id, member_phone_numbers)`. P2 #24 unification reads from this table.

**31. Saved searches in sidebar** — new `saved_searches(name, query_json)` table; sidebar section renders them; click → runs search.

**32. Configurable global focus hotkey** — `tauri-plugin-global-shortcut`; default ⌃⌥⌘T; settings UI in a new `/settings` route.

**33. VIP contacts that bypass DND** — per-conversation `is_vip` flag; mute/DND honours `is_vip=1` as escape valve.

**34. Hide preview in notifications** — global + per-conversation setting; notifier sends "New message" instead of body when enabled.

**35. Reopen last conversation on launch** — persist `last_open_conversation` in a tiny settings JSON at data dir; restore on boot.

**36. Connection diagnostics screen** — `/debug` route showing libgm state, last reconnect time, RPC error counts (need to instrument `internal/client/*`).

**37. Insights dashboard** — mount `internal/story/` + `internal/viz/` outputs at `/insights`; render charts with a small viz lib (uPlot or vanilla SVG). Don't pull in d3.

---

# Explicitly out of scope (enforce across all sessions)

- Re-enabling WhatsApp / Signal live bridges (`TEXTBRIDGE_GOOGLE_ONLY` is deliberate).
- Sending via iMessage from Textbridge (needs private Apple APIs).
- Cross-device sync.
- Linux / Windows builds.
- At-rest encryption of `messages.db`.
- Plugin system / third-party extensions.
- Touching `site/` (upstream marketing).

If an item seems to need something in this list, stop and flag it — don't quietly widen scope.

---

# Execution contract

- **One item per `/f:new` worktree.** Never stack multiple items into one PR — the roadmap assumes independent merges.
- **Verify groundwork claims against `HEAD` before trusting them.** Issue #7 is a snapshot; six months from now most claims rot.
- **Tests: TDD for backend (Go) and component tests for UI (Preact).** Integration parity test (dev-shell smoke) for every P1+.
- **Commits: frequent, small.** Conventional commits (`feat:`, `fix:`, `phase0:`, `refactor:`). No Claude attribution.
- **Auto-push on commit** fires via the auto-push hook — nothing stays local-only.
- **Releases: tag a patch/minor after each user-visible change** via `desktop/scripts/release patch`. P1+ items ship as their own release.
- **When a card in this plan turns out wrong, update the card in-place** and commit the update as part of that item's PR. Keep this document honest.

---

# Done-when for the whole roadmap

- Phase 0 merged; `web/src/` is the source of truth.
- All 12 P1 items shipped, each as its own release tag.
- All 15 P2 items shipped.
- All 10 P3 items shipped.
- `CLAUDE.md` + `desktop/CLAUDE.md` reflect final architecture.
- Issue #7 closed with a comment linking each item's PR.
