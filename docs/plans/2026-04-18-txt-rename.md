# txt Rename + Hide Empty Conversations Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Fix the phantom-conversation regression by filtering empty threads at query time, rename the app from `Textbridge` → `txt` end-to-end, and add a Hammerspoon `⌥Y` launcher.

**Architecture:** The phantom threads re-appear after v0.7.2's tombstone cleanup because Google Messages re-sends the conversation list on every sync, re-inserting the empty rows we just deleted. We push the filter into `ListConversations` so empty-conversation rows stay in the DB (needed for later message arrival) but never reach the sidebar. The rename is user-facing only — bundle identifier stays `ai.james-is-an.textbridge` so the auto-updater keeps working across the v0.7.2 → v0.8.0 boundary; only the product name, bundle file name, web title, and startup scripts change. Hammerspoon launch hotkey is a one-liner added to the existing `AppLauncher` module on the Mac.

**Tech Stack:** Go (SQLite query filter), Vite/Preact/HTM (web UI strings), Tauri 2 config, Bash release script, Hammerspoon Lua.

**Decisions (say now if you want to change):**
- New name: **`txt`** (per user suggestion). Sidebar `<h1>` and window title change to match.
- Bundle ID stays `ai.james-is-an.textbridge` — auto-updater keeps routing.
- Keychain notarytool profile stays `Textbridge` — it's a local keychain label, not user-visible.
- Version bump = **minor** (v0.7.2 → v0.8.0) since the rename is a visible breaking-ish change.
- `/Applications/Textbridge.app` symlink gets replaced by `/Applications/txt.app` on first post-rename install. The old one is left orphaned by the release script to avoid surprise deletions — user can delete it manually (or we could add a `rm -f $SYMLINK_OLD` line, optional).

**Working branch:** `feature/txt-rename` (already created via `/f:new`).

---

## Phase 1 — Hide empty conversations (Go backend)

Core bug fix. One-line SQL change at four call sites in `internal/db/conversations.go`. Ships standalone (no rename needed).

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 1. Add EXISTS filter + regression test | sonnet | sequential | none |

### Task 1: Filter conversations that have zero messages

**Files:**
- Modify: `internal/db/conversations.go` — the four `ORDER BY pinned_at DESC, last_message_ts DESC` sites (lines 316, 332, 362, 389).
- Test: `internal/db/conversations_test.go`

**Step 1: Write a failing test**

Append to `internal/db/conversations_test.go`:

```go
func TestListConversationsHidesEmptyConversations(t *testing.T) {
	store := newTestStore(t)

	// Conversation with real content — should appear.
	if err := store.UpsertConversation(&Conversation{
		ConversationID: "c-real",
		Name:           "Real",
		LastMessageTS:  9000,
	}); err != nil {
		t.Fatalf("seed real convo: %v", err)
	}
	if err := store.UpsertMessage(&Message{
		MessageID:      "m1",
		ConversationID: "c-real",
		Body:           "hello",
		TimestampMS:    9000,
	}); err != nil {
		t.Fatalf("seed real msg: %v", err)
	}

	// Phantom conversation with no messages — should NOT appear.
	if err := store.UpsertConversation(&Conversation{
		ConversationID: "c-phantom",
		Name:           "Phantom",
		LastMessageTS:  5000,
	}); err != nil {
		t.Fatalf("seed phantom convo: %v", err)
	}

	got, err := store.ListConversations("", 50)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListConversations returned %d rows, want 1 (only the populated one)", len(got))
	}
	if got[0].ConversationID != "c-real" {
		t.Fatalf("ListConversations returned %q, want c-real", got[0].ConversationID)
	}
}
```

Run: `cd ~/code/personal/textbridge/.worktrees/txt-rename && go test ./internal/db/ -run TestListConversationsHidesEmptyConversations -v`

Expected: FAIL with 2 rows returned, want 1.

**Step 2: Add the filter in the base query builder**

Open `internal/db/conversations.go`. Find the four SELECT query strings that end in `ORDER BY pinned_at DESC, last_message_ts DESC`. Before each `ORDER BY`, inject a WHERE/AND clause that filters to conversations with at least one message:

```
AND EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = conversations.conversation_id)
```

If a query already has a `WHERE` clause (e.g., filtering by folder), add as `AND EXISTS (…)`. If it has no `WHERE`, add as `WHERE EXISTS (…)`. Read the four call sites fully first — they're probably near-identical and this change is the same at each.

**Verification:**

```bash
cd ~/code/personal/textbridge/.worktrees/txt-rename
go test ./internal/db/ -run TestListConversationsHidesEmptyConversations -v
```
Expected: PASS.

Also run the full DB test suite to confirm nothing else regressed:
```bash
go test ./internal/db/ -v
```
Expected: all PASS.

**Step 3: Commit**

```bash
git add internal/db/conversations.go internal/db/conversations_test.go
git commit -m "fix: hide conversations with no messages from sidebar (phantom stub regression)"
```

---

## Phase 2 — Rename Textbridge → txt

Six files touching user-visible strings. Pure find-and-replace with careful scope to avoid touching comments / long prose that reads oddly with a bare 3-letter word.

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 2. Rename in Tauri config + release script | sonnet | sequential | Phase 1 merged |
| 3. Rename in web UI | sonnet | sequential | Task 2 |
| 4. Move installed symlink | sonnet | sequential | Task 3 |

### Task 2: Rename in Tauri config + release script

**Files:**
- Modify: `desktop/src-tauri/tauri.conf.json`
- Modify: `desktop/scripts/release`

**Step 1: Tauri config**

In `desktop/src-tauri/tauri.conf.json`:

```diff
-  "productName": "Textbridge",
+  "productName": "txt",
```

And the window title:

```diff
         "label": "main",
-        "title": "Textbridge",
+        "title": "txt",
```

Do NOT change `identifier` — keep `ai.james-is-an.textbridge`.

**Step 2: Release script**

In `desktop/scripts/release`:

```diff
-APP_NAME="Textbridge"
+APP_NAME="txt"
```

Leave `NOTARY_PROFILE="${NOTARY_KEYCHAIN_PROFILE:-Textbridge}"` alone — that's the keychain profile name created by `xcrun notarytool store-credentials Textbridge …` and it's local-only. The comment `xcrun notarytool store-credentials Textbridge …` around line 132 can stay as historical reference.

Similarly update the user-facing release-notes template (around line 222):

```diff
-    "notes": f"Textbridge {version}",
+    "notes": f"txt {version}",
```

**Step 3: Verify**

```bash
cd ~/code/personal/textbridge/.worktrees/txt-rename
grep -nE "productName|APP_NAME|\"title\":\s*\"Textbridge\"" desktop/src-tauri/tauri.conf.json desktop/scripts/release
```
Expected: no remaining "Textbridge" for productName / APP_NAME / window title.

**Step 4: Commit**

```bash
git add desktop/src-tauri/tauri.conf.json desktop/scripts/release
git commit -m "rename: Textbridge -> txt in Tauri config + release script"
```

---

### Task 3: Rename in web UI

**Files:**
- Modify: `web/index.html` (title + sidebar `<h1>`)
- Modify: `web/src/legacy.js` (document.title template + user-facing strings)

**Step 1: Web HTML**

In `web/index.html`:

```diff
-  <title>Textbridge</title>
+  <title>txt</title>
```

Sidebar:

```diff
             <h1>Textbridge</h1>
+            <h1>txt</h1>
```

The longer helper strings (e.g., `<p class="wa-helper">Link a WhatsApp companion device for live WhatsApp sync, typing updates, and text replies inside Textbridge.</p>`) read oddly with a 3-letter lowercase name. **Keep those as "Textbridge"** for now — we can copy-edit later. This task only touches the product identity strings.

**Step 2: legacy.js document title**

Around line 2626:

```diff
-    document.title = total > 0 ? `(${total}) Textbridge` : 'Textbridge';
+    document.title = total > 0 ? `(${total}) txt` : 'txt';
```

Leave the other "Textbridge" strings in legacy.js (1259, 1265, 1285, 1288, 1860, 2061, 4123, 4153, 4162, 4196, 4201) in prose-style sentences alone — they'll be copy-edited separately. Scope here is only the title/identity.

**Step 3: Verify Vite build succeeds**

```bash
cd ~/code/personal/textbridge/.worktrees/txt-rename
cd web && bun run build
```
Expected: `✓ built in <N>ms`.

**Step 4: Commit**

```bash
cd ~/code/personal/textbridge/.worktrees/txt-rename
git add web/index.html web/src/legacy.js
git commit -m "rename: Textbridge -> txt in web title + sidebar header"
```

---

### Task 4: /Applications symlink migration

**Files:**
- Modify: `desktop/scripts/release` — add a one-liner to remove the old `/Applications/Textbridge.app` symlink if present, after installing the new symlink.

**Step 1: Update release script**

After the line in `desktop/scripts/release` that creates `$SYMLINK` (`ln -s "$STABLE_APP" "$SYMLINK"` or similar — search for `ln -s`), add:

```bash
# Post-rename cleanup: remove the old Textbridge.app symlink if it survives.
# Safe because it points at a path that no longer matches what we ship now.
OLD_SYMLINK="/Applications/Textbridge.app"
if [[ -L "$OLD_SYMLINK" ]]; then
    echo "Removing orphaned $OLD_SYMLINK"
    rm -f "$OLD_SYMLINK"
fi
```

Only delete if it's a symlink (`-L`), not a regular file — paranoia against destroying a legitimate older install.

**Step 2: Verify**

```bash
cd ~/code/personal/textbridge/.worktrees/txt-rename
bash -n desktop/scripts/release
```
Expected: no syntax errors.

**Step 3: Commit**

```bash
git add desktop/scripts/release
git commit -m "release: clean up old /Applications/Textbridge.app symlink after rename"
```

---

## Phase 3 — Hammerspoon ⌥Y launcher

Lives outside the repo — in `~/.hammerspoon/modules/app-launcher.lua`. Separate commit in the user's Hammerspoon config (if that's version-controlled elsewhere) or just an in-place edit.

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 5. Add ⌥Y binding | haiku | sequential | (independent of textbridge repo) |

### Task 5: Bind ⌥Y to launch/focus txt.app

**Files:**
- Modify: `~/.hammerspoon/modules/app-launcher.lua`

**Step 1: Add to M.bindings table**

Append an entry in the `M.bindings` table (immediately after the last existing entry, before the closing `}`):

```lua
  { mods = {"alt"}, key = "y", bundleID = "ai.james-is-an.textbridge" },
```

**Step 2: Reload Hammerspoon config**

```bash
hs -c "hs.reload()"
```
Or via menu bar: Hammerspoon → Reload Config.

**Step 3: Manual verification**

1. With txt.app NOT running: press ⌥Y. Expected: app launches, window appears.
2. With txt.app already running and not focused: press ⌥Y. Expected: window comes to front.
3. With txt.app running and focused: press ⌥Y. Expected: window hides (matches other AppLauncher bindings' toggle behaviour).

**Step 4: Commit**

If `~/.hammerspoon/` is version-controlled (check `git -C ~/.hammerspoon status`):

```bash
cd ~/.hammerspoon
git add modules/app-launcher.lua
git commit -m "app-launcher: add alt+y for txt"
```

Otherwise just leave the edit in place.

---

## Phase 4 — Release v0.8.0

| Task | Model | Execution | Dependencies |
|------|-------|-----------|--------------|
| 6. Ship feature branch | manual | — | Phases 1-3 |
| 7. Cut v0.8.0 | manual | — | Task 6 |

### Task 6: Ship

```bash
cd ~/code/personal/textbridge/.worktrees/txt-rename
# Regenerate dist before committing (web changes in Task 3).
cd web && bun run build && cd ..
git add internal/web/static/dist/
git commit -m "chore: regenerate dist for txt rename"
/f:ship
```

### Task 7: Cut v0.8.0

```bash
cd ~/code/personal/textbridge/desktop
./scripts/release minor
```

Expected runtime ~8-12 min (full Go sidecar + arm64/x86_64 Rust builds + notarisation). The script now rebuilds the sidecar first (PR #45).

### Task 8: Post-release verification

1. **Relaunch the installed app.** Auto-updater v0.7.2 → v0.8.0 picks it up on next launch.
2. Sidebar should show ONLY populated conversations — no more "Amy Yang / Amy Swan / Amy Rogers / Amber Baron / Amanda Chen" phantom rows.
3. The app window title and menu bar should read `txt`.
4. `⌥Y` from anywhere should focus txt.app.
5. Gatekeeper check: `spctl -a -vv /Applications/txt.app` → `accepted, source=Notarized Developer ID`.
6. Orphan check: `ls -la /Applications/Textbridge.app 2>&1` → "No such file or directory" (release script removed it).

### Task 9: Create GitHub release

```bash
cd ~/code/personal/textbridge
gh release create v0.8.0 \
  --repo jamesdowzard/textbridge \
  --title "v0.8.0 — txt" \
  --notes "## Rename: Textbridge → txt

Short product name. Bundle ID unchanged so v0.7.2 installs auto-update cleanly.

## Fix: phantom conversations finally gone

v0.7.2 deleted the tombstone-stub messages but Google Messages re-seeded the empty conversation rows on every sync. This release adds an EXISTS filter to ListConversations so conversations without any messages never reach the sidebar, regardless of how many times Google re-sends them.

## Plus

- Hammerspoon ⌥Y launcher recipe in the install notes." \
  desktop/release/stable/txt.app.tar.gz \
  desktop/release/stable/txt.app.tar.gz.sig \
  desktop/release/stable/latest.json
```

Note: file names are `txt.app.tar.gz` (not `Textbridge.app.tar.gz`) because `APP_NAME` changed.

---

## Summary

- **9 tasks** across 4 phases.
- **Phase 1 is the urgent bug fix** — it alone solves the "I still see them" complaint. Phases 2-4 are the rename + hotkey + release.
- **Execution strategy:** sequential (each phase depends on the previous). Phase 3 is technically independent but trivial.
- **Complexity:** Low. Most tasks are a few lines of code / config edits.
- **Estimated effort:** 30-45 min hands-on work + 8-12 min release build.
- **Risk areas:**
  - Rename breaks user muscle memory — no mitigation, just a heads-up.
  - `/Applications/Textbridge.app` orphan cleanup: only deletes if it's a symlink (safety guard). Can always roll back manually.
  - Auto-updater cross-rename: tested once before (bundle ID unchanged = should work). If v0.7.2 installed users report updater failures, patch v0.8.1 can hard-code the fallback.

---

*Plan authored 2026-04-18. References: tombstone stub bug investigation (conversation above), `desktop/scripts/release`, `~/.hammerspoon/modules/app-launcher.lua`.*
