# Issue #15 Roadmap Plan — 5 features + 2 hygiene

**Repo:** `jamesdowzard/txt` @ `main` (v0.8.8)
**Source:** https://github.com/jamesdowzard/txt/issues/15
**Filed:** 2026-04-19

## Goal

Ship the remaining unshipped features from the v0.8.8 sprint, in the right order to minimise merge conflicts in the two hotspot files (`web/src/legacy.js`, `internal/web/api.go`).

## Reality check vs the issue

Before planning, current repo state was reviewed. Three surprises:

| Item | Issue says | Actual state |
|------|-----------|--------------|
| #4 Inline-reply notification | "Today the app just posts a plain notification" | **ALREADY IMPLEMENTED.** `desktop/src-tauri/src/notifications.rs` (`show_with_reply`, `send_reply`) + `sse.rs` (subscribes to `/api/events`, dispatches on `incoming_message`). `mac-notification-sys = "0.6"` in Cargo.toml. Backend emits `OPENMESSAGES_MACOS_NOTIFICATIONS=0` for this process-tree. → Just needs **verification**, not implementation. |
| Hygiene B `.github/` | "verify dir, git rm if empty" | **ALREADY GONE.** `ls .github/` → no such file. → Skip. |
| #1 Scheduled-send backend | "Backend shipped" | Confirmed. `/api/schedule` (POST), `/api/outbox` (GET), `/api/schedule/{id}` (DELETE) all at `internal/web/api.go:1343-1417`. `internal/scheduler/scheduler.go` polls every 30s. |

Net: **5 units of real work** (items 1, 2, 3, 5, hygiene A) + 1 verification (item 4).

## Conflict map (why ordering matters)

```
                        legacy.js  api.go     db.go  desktop/
Item #1 Scheduled UI      X          -          -      -
Item #2 iMessage media    X          X          -      -
Item #3 Tapbacks          X          X          -      -
Item #5a VIP feature      X          X          X      -
Item #5b Menu-bar popover X          -          -      X
Item #4 Verify notif      -          -          -      X
Hygiene A memory file     (zero code — no files)
```

All of #1/#2/#3/#5a touch `legacy.js` + `api.go`. Running them in true parallel worktrees guarantees 3-way merge conflicts. Strategy: **parallel worktrees anyway**, with each feature confined to different regions of the files, and land in small PRs so rebases are cheap.

## Execution strategy

| Phase | Items | Mode | Why |
|-------|-------|------|-----|
| 0 | Verify item #4 + memory refresh | parallel (2 agents) | zero code overlap, cheap |
| 1 | Items #1, #2, #3 | parallel worktrees (3 agents) | different functions within shared files; merge in order of size (smallest first) |
| 2 | Item #5a (VIP star feature) | sequential, single worktree | DB migration + API + sidebar — one cohesive PR |
| 3 | Item #5b (menu-bar popover) | sequential, single worktree | depends on #5a landing so "VIP list" exists to mirror |

Target: phases 0 + 1 in one session, phase 2 next session, phase 3 follow-up (it's ~half-day on its own).

---

## Phase 0 — Verify + hygiene (parallel, haiku)

### Task 0.1: Verify item #4 (inline-reply notification) works end-to-end

**Files:** none — verification only.

**Steps:**
1. Confirm `/Applications/txt.app` is running v0.8.8.
2. `cat /Applications/txt.app/Contents/Info.plist | grep CFBundleShortVersionString` — expect `0.8.8`.
3. Check `mac-notification-sys` feature is compiled-in: `otool -L /Applications/txt.app/Contents/MacOS/textbridge | head` and inspect. Or grep the binary: `strings /Applications/txt.app/Contents/MacOS/textbridge | grep -i 'show_with_reply\|mac-notification' | head`.
4. Send a test message from another device. Observe: does the macOS notification appear? Does the inline-reply field show (`Reply…` placeholder)? Does typing + return POST to `/api/send`?
5. If steps 4 succeed → mark #4 done, close it in the issue tracker.
6. If any step fails → file a follow-up issue with specific symptom (`show_with_reply` not firing vs reply POST failing vs notification style missing).

**Commit:** none. This is a check.

### Task 0.2: Refresh auto-memory file

**Files:** `~/.claude/projects/-Users-jamesdowzard/memory/project_txt_app.md`

**Steps:**
1. Read current state of that file.
2. Update PR list to reflect reality: v0.8.2 through v0.8.8 shipped; tab strip + platform icons; detached from upstream; no CI; `.github/` removed.
3. Add: issue #15 exists as the active roadmap, phase 0 complete.
4. Don't rewrite from scratch — preserve historical context.

**Commit:** none (memory files are gitignored / live outside repo).

---

## Phase 1 — Three parallel feature worktrees (sonnet)

Each feature gets its own harness worktree via `/f:new`. Land in the order listed (smallest first to minimise rebase pain). Rebase the next worktree on main after each PR merges.

### Task 1.1: Scheduled-send UI (item #1)

**Worktree slug:** `scheduled-send-ui`
**Model:** sonnet
**Scope:** frontend-only. Backend already complete.

**Files:**
- Modify: `web/index.html:198-212` — composer row (wire up existing disabled `#schedule-btn`)
- Modify: `web/src/legacy.js:180-210` (element lookups region), `:3497-3550` (sendMessage area for schedule popover integration), `:sidebar-render region` (add Outbox filter row)
- Modify: `web/src/legacy.css` — styles for schedule popover + outbox rows
- Read for reference: `internal/web/api.go:1343-1417` (backend contract)

**Steps:**

1. **Element lookups.** Add below line 195-ish:
   ```js
   const $scheduleBtn = document.getElementById('schedule-btn');
   const $outboxRow = document.getElementById('outbox-row'); // new sidebar entry, added in HTML
   ```

2. **Enable logic.** In the `autoResize`/`updateSendButtonState` area (find the pattern that toggles `$sendBtn.disabled`), mirror for `$scheduleBtn.disabled = !canSendText || !hasText`.

3. **Schedule popover.** On `$scheduleBtn.click` → render popover:
   ```html
   <div class="schedule-popover">
     <label>Send at</label>
     <input type="datetime-local" id="schedule-when">
     <button id="schedule-go">Schedule</button>
     <button id="schedule-cancel">Cancel</button>
     <div class="schedule-err"></div>
   </div>
   ```
   Default value: `new Date(Date.now() + 5*60_000)` (ISO-local, minutes resolution).
   On Schedule click:
   ```js
   const whenMs = new Date($('#schedule-when').value).getTime();
   if (whenMs < Date.now() + 60_000) { showErr('Must be ≥ 1 min in future'); return; }
   await postJSON('/api/schedule', {
     conversation_id: activeConvoId,
     body: $composeInput.value.trim(),
     scheduled_at: whenMs,
     reply_to_id: replyToMsg?.MessageID || ''
   });
   $composeInput.value = ''; autoResize(); closePopover();
   showThreadFeedback('Scheduled.');
   ```

4. **Outbox sidebar entry.** Add a new row below existing folder filters with label "Outbox" + count badge. Count comes from `GET /api/outbox` filtered by `status='pending'`. Refresh count on: initial load, on schedule success, on tab visibility change, and on 30s poll.

5. **Outbox view.** Clicking the row renders a list inside the main pane (replacing the conversation thread view similar to how search-results render). Each row: conversation name, thread preview (body truncated to 80 chars), scheduled datetime (relative: "in 3 hours"), status pill (pending/failed), × button.
   - × button → `DELETE /api/schedule/{id}` → remove row optimistically.

6. **Styling.** Match the existing design-token palette (`web/src/styles/tokens.css`). Popover uses the same card style as the emoji picker; outbox rows match folder-filter rows.

7. **Smoke test.**
   ```bash
   cd web && bun run build
   cd .. && go build -o txt . && ./txt serve &
   # Open http://127.0.0.1:7007, pick an active iMessage conversation
   # Type "hello from scheduler test", click clock, pick time +2 min, click Schedule
   # Expect: thread banner "Scheduled.", compose cleared, outbox row appears with count=1
   # Wait 2 min → expect message to arrive in thread, outbox count drops to 0
   ```

8. **Commit** (feature worktree):
   ```bash
   git add web/ && git commit -m 'feat(ui): scheduled-send composer + outbox sidebar'
   ```

9. **PR** via `/f:ship`. Merge. Rebase #1.2 and #1.3 on the new main.

---

### Task 1.2: iMessage image drag-drop + paste (item #2)

**Worktree slug:** `imessage-media-send`
**Model:** sonnet
**Scope:** extend `/api/send-media` to handle iMessage; add composer paste + drop handlers for iMessage conversations.

**Files:**
- Modify: `internal/web/api.go:986` (`/api/send-media` handler — add iMessage branch) + `:2160-2340` area (new `sendIMessageMedia` alongside `sendIMessageText`)
- Modify: `web/src/legacy.js:1091-1097` (`conversationSupportsMediaOutbound` — add `'imessage'`)
- Modify: `web/src/legacy.js` composer drop/paste handlers (grep `DataTransfer|clipboardData` to locate existing WA/SMS handlers)
- Add: `internal/web/api_imessage_media_test.go` — unit test for AppleScript string assembly

**Steps:**

1. **Backend: assemble new AppleScript form.** Mirror `buildSendAppleScript` shape:
   ```go
   // buildSendMediaAppleScript sends a POSIX file via Messages.app. 1:1 uses
   // buddy-addressing; group uses chat-by-id.
   func buildSendMediaAppleScript(t imessageTarget, absPath string) string {
       if t.chatGUID != "" {
           return fmt.Sprintf(`tell application "Messages"
               send POSIX file %q to chat id %q
           end tell`, absPath, t.chatGUID)
       }
       return fmt.Sprintf(`tell application "Messages"
           set targetService to first service whose service type = %s
           set targetBuddy to buddy %q of targetService
           send POSIX file %q to targetBuddy
       end tell`, t.service, t.buddy, absPath)
   }
   ```

2. **Backend: new send helper.** Next to `sendIMessageText`:
   ```go
   func sendIMessageMedia(store *db.Store, recordOutgoing func(*db.Message, string) error, conversationID, absPath, mimeType string) (*db.Message, error) {
       // same shape as sendIMessageText but runs buildSendMediaAppleScript
       // and records the outgoing message with MediaID=absPath + MimeType.
   }
   ```

3. **Backend: route iMessage in `/api/send-media`.** In the existing handler (line 986), after the platform branch for sms/whatsapp/signal, add:
   ```go
   if strings.HasPrefix(conversationID, "imessage:") {
       // write uploaded multipart to a temp file, call sendIMessageMedia,
       // temp file cleanup on defer.
   }
   ```

4. **Backend: image-only guardrail.** Reject non-image MIME types (`image/jpeg|png|gif|webp|heic`) with 415. Keeps osascript AppleScript attack surface tight.

5. **Unit test.** `api_imessage_media_test.go`:
   ```go
   func TestBuildSendMediaAppleScript_1to1(t *testing.T) { ... }
   func TestBuildSendMediaAppleScript_Group(t *testing.T) { ... }
   ```
   Expect compiled AppleScript contains `POSIX file "/tmp/…"` and correct chat-id / buddy.

6. **Frontend: extend `conversationSupportsMediaOutbound` at legacy.js:1091.**
   ```js
   function conversationSupportsMediaOutbound(convo) {
     return !!convo && (
       sourcePlatformOf(convo) === 'sms'
       || sourcePlatformOf(convo) === 'imessage'
       || (sourcePlatformOf(convo) === 'whatsapp' && whatsAppStatus.connected)
       || (sourcePlatformOf(convo) === 'signal' && signalStatus.connected)
     );
   }
   ```

7. **Frontend: paste handler.** In the composer setup region (search `clipboardData` in legacy.js):
   ```js
   $composeInput.addEventListener('paste', (e) => {
     const items = e.clipboardData?.items || [];
     for (const it of items) {
       if (it.kind === 'file' && it.type.startsWith('image/')) {
         const file = it.getAsFile();
         if (file) { setPendingFile({file, name: file.name || 'pasted.png'}); e.preventDefault(); return; }
       }
     }
   });
   ```
   If a handler already exists for WA/SMS, just make sure it doesn't early-return on iMessage conversations.

8. **Frontend: drop handler.** Same function — ensure the existing drop target doesn't skip iMessage.

9. **Build + smoke test.**
   ```bash
   cd web && bun run build
   cd .. && go test ./internal/web/... && go build -o txt .
   # Run, pick iMessage conversation, paste screenshot into composer
   # Expect: thumbnail preview, Send fires, image arrives in Messages.app
   ```

10. **Commit + PR** via `/f:ship`.

---

### Task 1.3: Tapback (reaction) send (item #3)

**Worktree slug:** `imessage-tapback-send`
**Model:** opus (AppleScript experimentation)
**Scope:** risky. Author flagged tapback AppleScript form as "brittle — expect to experiment". Timebox: if no working AppleScript after **60 min**, document the blocker in the PR and close without merging.

**Files:**
- Modify: `internal/web/api.go` — new `POST /api/reactions` handler alongside `/api/send`
- Add: `internal/web/api.go` — new `sendIMessageReaction` + `buildReactionAppleScript`
- Modify: `web/src/legacy.js` — message-bubble long-press/right-click → tapback picker popover
- Modify: `web/src/legacy.css` — picker styling

**Steps:**

1. **Spike the AppleScript.** Drop to a terminal first:
   ```bash
   osascript -e 'tell application "Messages"
     tell chat id "iMessage;+;chat123456" to set reaction of message id "ABC-DEF-GUID" to heart
   end tell'
   ```
   If that fails, try alternate forms documented in osascript/Messages.sdef. If all fail, shift to UI-scripting via System Events (open Messages, right-click message, click tapback). If even that fails, **stop** — document "tapback AppleScript not available on macOS 26 (26.x.x)" in PR body and close. The other 4 items still ship.

2. **Backend handler.**
   ```go
   mux.HandleFunc("/api/reactions", func(w http.ResponseWriter, r *http.Request) {
     // POST only; body: {message_id, reaction_type}
     // reaction_type ∈ {heart,thumbs_up,thumbs_down,haha,emphasize,question}
     // Look up the message's chat GUID from store, then:
     //   runAppleScriptReaction(chatGUID, messageGUID, reactionType)
     // On success: optimistically stamp the local message's Reactions JSON
     //   (same shape imessage.go importer produces).
   })
   ```

3. **AppleScript builder.** Pattern after `buildReplyAppleScript`:
   ```go
   func buildReactionAppleScript(chatGUID, messageGUID, reactionType string) string {
       return fmt.Sprintf(`tell application "Messages"
           tell chat id %q to set reaction of message id %q to %s
       end tell`, chatGUID, messageGUID, reactionType)
   }
   ```
   Note: `reactionType` is an AppleScript identifier (unquoted). Validate against whitelist before interpolating.

4. **Frontend: long-press handler.** On `.msg` bubble pointerdown → 500ms timer → show picker. Picker shows 6 tapback glyphs inline with the bubble:
   ```
   ♥  👍  👎  😂  ‼️  ❓
   ```
   Click → `POST /api/reactions` → optimistic UI render (append to existing `.msg-reactions` container like incoming reactions do).

5. **Right-click handler.** Same picker on `contextmenu` — reuses existing `#context-menu` element if available.

6. **Test live.** Cannot be unit-tested cleanly — AppleScript is the system. Send 6 tapbacks on a test conversation, verify they appear on the recipient device and in local chat.db (`sqlite3 ~/Library/Messages/chat.db "select * from message order by rowid desc limit 5"` → expect associated_message_type 2000-3005 rows).

7. **Commit + PR** via `/f:ship`. If AppleScript route fails, PR title: `chore: tapback send blocked — document AppleScript failure` with findings in body.

---

## Phase 2 — VIP feature (item #5a, sequential, sonnet)

**Worktree slug:** `vip-contacts`
**Depends on:** Phase 1 merged (avoid legacy.js merge hell).
**Scope:** one cohesive PR: DB migration + API + sidebar star UI. Menu-bar popover is a separate PR.

**Files:**
- Modify: `internal/db/db.go` — add migration for `conversations.is_vip INTEGER NOT NULL DEFAULT 0`
- Modify: `internal/db/conversations.go` — add `IsVIP bool` to struct, `SetVIP(id, vip bool)` method
- Modify: `internal/web/api.go` — `POST /api/conversations/{id}/vip` + `/unvip`
- Modify: `web/src/legacy.js` — VIP section at top of sidebar, context-menu Star/Unstar item

**Steps:**

1. **DB migration.** In `internal/db/db.go` migration block:
   ```go
   {Version: <next>, Statement: `ALTER TABLE conversations ADD COLUMN is_vip INTEGER NOT NULL DEFAULT 0`},
   ```

2. **DB model update.** `conversations.go`:
   - Add `IsVIP bool` field, map in `scanConversation`.
   - Method:
     ```go
     func (s *Store) SetVIP(id string, vip bool) error { ... }
     ```

3. **Test.**
   ```go
   func TestSetVIP(t *testing.T) { ... }
   ```
   Create conv, mark VIP, re-read, assert `IsVIP=true`.

4. **API endpoints.**
   ```go
   mux.HandleFunc("/api/conversations/", func(w, r) {
     // Parse path: /api/conversations/{id}/(vip|unvip)
     // POST only; set is_vip accordingly; publishConversations()
   })
   ```

5. **Publish event.** When VIP toggled, include `is_vip` in the conversation SSE event so sidebar updates without refetch.

6. **Frontend: VIP section.** New container above folder filters:
   ```html
   <div class="sidebar-section" id="vip-section" hidden>
     <div class="section-label">VIPs</div>
     <div id="vip-list"></div>
   </div>
   ```
   Render all conversations where `c.IsVIP === true`, clicking opens the thread. Hide section if empty.

7. **Frontend: context-menu items.** In the existing right-click menu code (PR #3), add:
   ```js
   { label: 'Star as VIP',    when: !conv.IsVIP, click: () => postJSON(`/api/conversations/${conv.ConversationID}/vip`) },
   { label: 'Remove VIP',     when: conv.IsVIP,  click: () => postJSON(`/api/conversations/${conv.ConversationID}/unvip`) },
   ```

8. **Smoke test.**
   ```bash
   cd web && bun run build && cd .. && go test ./internal/db/... && go build -o txt .
   # Run, right-click a conversation → Star as VIP
   # Expect: VIP section appears at top of sidebar with that conversation
   # Unstar → section hides
   ```

9. **Commit + PR** via `/f:ship`.

---

## Phase 3 — Menu-bar popover (item #5b, follow-up session)

**Defer to a separate session.** This is a ~half-day task involving `tauri-plugin-positioner`, NSStatusItem, a new small Tauri window, and native tray code. Out of scope for today.

Flag in the PR description for #5a: "Menu-bar popover tracked separately; this PR ships the VIP star feature as the foundation."

---

## Summary

| Phase | Tasks | Parallel? | Model | Est. time |
|-------|-------|-----------|-------|-----------|
| 0 | #4 verify, memory refresh | yes (2 agents) | haiku | 10 min |
| 1 | #1 sched-UI, #2 iMessage media, #3 tapbacks | yes (3 worktrees) | sonnet + opus | ~3 hrs (merges serial) |
| 2 | #5a VIP feature | no | sonnet | ~2 hrs |
| 3 | #5b menu-bar popover | deferred | sonnet | next session |

**6 PRs total** (4 features + 1 verification report + 1 memory update). **Low-medium complexity.** No destructive actions. No main-branch edits — every code change goes through `/f:new`.

## Ambiguities / decisions needed from user

1. **Item #3 risk tolerance.** If tapback AppleScript doesn't work on macOS 26, is a UI-scripting fallback (System Events keystroking the tapback menu) acceptable, or should we just document and move on? — Plan defaults to "document and move on" after a 60-min timebox.

2. **Phase 1 parallelism.** Three worktrees in parallel maximises throughput but means rebasing twice (#1.2 on #1.1, #1.3 on both). Alternative: land them strictly sequentially in ONE worktree, simpler but slower. — Plan defaults to parallel.

3. **Item #4 verification mode.** If verification shows the feature isn't firing, do we plan a fix this session or hand off to a follow-up issue? — Plan defaults to "file follow-up, don't fix this session" since scope was already large.

4. **Scheduled-send UI UX.** Issue says "small popover" — is datetime-local input enough, or do you want quick buttons (in 1 hr / tomorrow 9am / pick custom)? — Plan defaults to datetime-local only; quick buttons are a follow-up.

5. **Menu-bar popover scope for #5b.** Just VIPs list + unread badge + quick compose, or also show recent conversations? — Deferred task; answer when we pick it up.
