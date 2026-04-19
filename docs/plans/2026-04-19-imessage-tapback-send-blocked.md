# iMessage tapback send — blocked

**Date:** 2026-04-19
**Branch:** `feature/imessage-tapback-send`
**Issue:** #15 item 3 (outbound tapback reaction send)
**Status:** Blocked — no viable implementation path on macOS 26.4.1

## TL;DR

Outbound iMessage tapbacks **cannot be sent** via any public API on modern
macOS. The Messages.app AppleScript dictionary has no `message` class, no
`reaction` property, and no tapback-related verb. GUI scripting (System
Events) works in principle but would require screen-coordinate clicking and
Messages.app to be foregrounded on the correct chat — incompatible with a
background app that never steals focus. Recommendation: ship the other four
items in #15 and skip outbound tapback until Apple exposes an API (unlikely)
or a private-framework route is taken (requires entitlements we don't have).

## macOS / Messages.app version

```
sw_vers -productVersion  → 26.4.1   (build 25E253)
Messages.app version     → 26.0
```

## Scripting dictionary audit

`sdef /System/Applications/Messages.app` — the complete suite exposes:

**Commands:** `send`, `login`, `logout`
**Classes:** `application`, `participant`, `account`, `chat`, `file transfer`
**Enumerations:** `service type`, `direction`, `transfer status`, `connection status`

There is **no** `message` class, **no** `reaction` / `tapback` / `emphasize` /
`heart` / `thumb` term anywhere in the dictionary, and **no** verb that
reaches a single message by GUID. The `send` command only takes `text` or
`file` direct parameters routed to a `participant` or `chat`.

Grep confirms:

```bash
$ sdef /System/Applications/Messages.app | grep -iE 'react|tap|heart|thumb|emphas'
# (no output)
```

## AppleScript forms attempted

### Form 1 — documented "set reaction of message id"

```applescript
tell application "Messages"
  tell chat id "any;-;+61437590462" to ¬
    set reaction of message id "AF063AD5-E61E-4F91-9B71-9F4944338535" to heart
end tell
```

**Result:** `syntax error: A property can't go after this identifier. (-2740)`
`message` is a reserved AppleScript property (the "last message of chat"
accessor), not a class you can address by `id`. Cannot be parsed.

### Form 2 — reply-addressable variant

```applescript
tell application "Messages"
  set targetChat to chat id "any;-;+61437590462"
  set targetMsg to message id "AF063AD5-E61E-4F91-9B71-9F4944338535" of targetChat
  set reaction of targetMsg to heart
end tell
```

**Result:** `syntax error: Expected end of line, etc. but found property. (-2741)`
Same cause — `message` isn't a class term. Parser error before any runtime.

### Form 2b — does `message` as a class work at all?

```applescript
tell application "Messages"
  set targetChat to chat id "any;-;+61437590462"
  return message id "AF063AD5-E61E-4F91-9B71-9F4944338535" of targetChat
end tell
```

**Result:** `syntax error: Expected end of line, etc. but found property. (-2741)`

### Form 2c — `messages of chat` (plural)

```applescript
tell application "Messages"
  set targetChat to chat id "any;-;+61437590462"
  return count of messages of targetChat
end tell
```

**Result:** `Messages got an error: Can't make messages of chat id
"any;-;+61437590462" into type specifier. (-1700)`
The token parses (since there's an `AXGroup`-adjacent AEO coercion) but
there is no iterable messages element on `chat`.

### Form 3 — direct `set reaction of <guid>` (long shot)

```applescript
tell application "Messages"
  set reaction of "AF063AD5-E61E-4F91-9B71-9F4944338535" to heart
end tell
```

**Result:** `execution error: The variable heart is not defined. (-2753)`
`heart` isn't a defined AppleScript constant anywhere in the suite — and
even if it parsed, `reaction of <text>` is meaningless.

### Form 4 — `tapback` keyword

```applescript
tell application "Messages"
  tapback "AF063AD5-E61E-4F91-9B71-9F4944338535" with heart
end tell
```

**Result:** `syntax error: Expected end of line but found "". (-2741)`
`tapback` isn't a known verb.

### Form 5 — `react to message`

```applescript
tell application "Messages"
  react to "AF063AD5-E61E-4F91-9B71-9F4944338535" with heart
end tell
```

**Result:** `execution error: Messages got an error: Can't continue react. (-1708)`
`react` resolves as a continuation verb but no implementation exists.

### Form 6 — System Events GUI scripting

Attempted to walk the AX tree of Messages.app to locate a specific message
bubble. Findings from the AX dump:

- Message bubbles render as `AXGroup` elements with description
  `"Your iMessage, <text>, <time>"` (outgoing) or
  `"<Sender Name>, <text>, <time>"` (incoming).
- Bubbles carry **no persistent identifier** — no GUID, no ROWID, no stable
  AX identifier. Only a concatenation of (text, time) inside a human-
  readable description string. Long texts are truncated in the
  description, collisions on identical short messages ("OK") are routine.
- No AX action opens the tapback picker. Right-click (`AXShowMenu`) on an
  `AXGroup` has no effect in Messages.app — the tapback picker is
  triggered by a long double-click in the native view layer, not an AX
  event. A `click at {x, y}` via System Events would work but requires:
  (a) moving the mouse, (b) Messages.app foregrounded, (c) the target
  chat currently selected in the sidebar, (d) the target bubble visible
  (not scrolled off). None of these are acceptable for a background app.

### Form 7 — Shortcuts / scripting additions / private frameworks

- `shortcuts list` confirms no tapback-capable Shortcut actions ship with
  macOS. Apple exposes only `Send Message` and `Show in Messages`.
- `/System/Library/ScriptingAdditions/` contains nothing Messages-related.
- Private frameworks (`IMCore.framework`, `IMDPersistenceAgent`) do
  expose `IMChat -sendMessage:`-style reaction APIs, but using them
  requires `com.apple.private.messaging-client` entitlement which we
  cannot sign with a Developer ID certificate. Rejected out of scope.

## Why "inbound still works, outbound doesn't"

Inbound reactions land in `~/Library/Messages/chat.db` with
`associated_message_type` 2000–3005. Reading them is a SQLite scan plus a
GUID join — no Apple API needed. Outbound would have to go the other way
and actually transmit the tapback over the iMessage protocol, which is
gated behind the sealed `imagent`/`IMCore` subsystem.

## Decision

**Document-only commit. Do not merge. Close PR #15 item 3 as won't-do.**
The other four items in #15 ship independently.

If Apple exposes a scriptable reaction verb in a future macOS point
release, revisit by re-running the dictionary grep:

```bash
sdef /System/Applications/Messages.app | grep -iE 'react|tap|heart|thumb|emphas'
```

Until then, the UX workaround is: long-press a message in txt → "Reply
with emoji" → sends a normal iMessage containing the emoji in quoted
context. That's a distinct feature and not blocked by this investigation.

## Time spent

~30 min on the spike (well under the 60-minute timebox).
