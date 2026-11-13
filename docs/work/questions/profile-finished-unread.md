---
title: "Should a finished table's profile row still show unread chat?"
---

The brief says the row shows the pale dot and "N new" when there is unread chat, with no exception
for finished tables. The old card hid the unread chip on finished games, and this piece kept that
rule, so a "Game over" row never shows "N new".

That is a call I carried over, not one the brief made. Finished games can still gather chat (a
post-game debrief, say), and a player who has stopped opening them would never see it was waiting.
The other reading: a finished table isn't asking anything of you, and a dot there competes with the
tables that are.

If you want it shown, it is one line in `TableRow.svelte` (`const unread = …`) plus a flipped test
in `TableRow.test.ts`.
