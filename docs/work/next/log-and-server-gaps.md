---
title: "Small log and server gaps left from the audits"
order: 74
---

**Action log**
- Chronicle Histories: the log never says which artifacts were invoked, though they
  set the difficulty. A mar isn't announced, and mar lines have no narration.
- Make Introductions: when another player writes a newcomer, the "created" line
  credits the preparer, and the arrival reads twice.
- Liaise: a redelay that overflows row 13 logs the same line as a deliberate zero,
  with no cause.
- Spread Rumors: the "consented" line posts before the transfer commits.

**Server**
- Password reset: the single-use check and the mark-used write are separate, and
  `MarkPasswordResetTokenUsed` has no `used_at IS NULL` condition, so two simultaneous
  redemptions can both pass.
- `DISCORD_WEBHOOK_URL` unset writes the full feedback body and contact to the server
  log. Fine in dev, not if production ever loses the variable.
- `LoadResolutionData` discards the unmarshal error, so a malformed blob reads as
  empty.
- A festivity's nested duel has no arms-or-wits type or location, and falls back to
  generic notes.
- The push body is generic; the settled copy named the table's join code.

**Client**
- `SimultaneousRevealInput` can call `GET /api/reveals/null` after a Liaise resolves.
  It needs a null guard.
- `HostFestivityPanel` and `ProposeDuelPanel` still subscribe to window events by hand
  instead of `useWindowEvents`.
