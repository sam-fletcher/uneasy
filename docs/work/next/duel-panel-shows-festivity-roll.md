---
title: "A Propose Duel shows another plan's roll before it has rolled"
order: 95
---

Seen while checking the festivity duel terms (PR #147), at 375px, as the
duellist, before the duel's bouts. The duel panel's roll line reads "Rolled MAR
· 1 distinct face, difficulty 4", and the same panel's difficulty reads 3 ("6
minus rank 3 is 3"). The duel has not rolled, so the first line is not its roll.

How it was reached: a Host Festivity on a three-player game. Ben rolled a
guest's roll and it resolved as a mar. Cy opted out, which gave the host a
make. The host challenged Ben to a duel, and Ben accepted, so the duel spawned
on the same row. Ben's view of the duel then showed the festivity roll.

Probable cause, from reading the code (not reproduced in a test): the table
keeps one `activeRoll` per row (`MainEventView.svelte`). It passes that roll
to every plan panel, and derives `rollOutcome` from it without checking the
roll's `plan_id` against the panel's own plan. So a resolved roll on the row
can appear on another plan's panel.

Screenshot: the duel heading as ben sees it at 375px, in the PR's Evidence
section (`duel-heading-ben-375.png`).

Done when: a duel's panel shows only its own roll (or none before the roll),
with a test that puts a resolved festivity roll on the same row as a duel.
