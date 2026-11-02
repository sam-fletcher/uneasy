---
title: "Chronicle Histories: other players' make steps and mar choices don't reach open panels"
order: 41
---

`make-step` and `mar-choice` (`handler/plan_chronicle_histories.go`) save
`resolution_data` and post a log line, but never broadcast
`plan.choice_applied` (Propose Decree, Spread Propaganda and Exchange Courtiers
do). The chooser's own panel refreshes itself; nobody else's does. Seen in the
UI pass (alice prepares, bob and carol have the table open):

- Mar: bob submits an option and, 15 seconds later, alice's panel still reads
  "Submitted so far (0)" and "Waiting for all players to choose (0/3)", and
  every Waiting On bar still names bob. A reload fixes it. The waiting bar is
  the thing a play-by-post player glances at, so this is the likely way a mar
  stalls: people wait on a bar that already shows their turn done.
- Make: the preparer's steps ("Resolved so far", the remaining count, and the
  "scene stays open until alice wraps it" line) stay stale for bob and carol.

Fix: broadcast `EventPlanChoiceApplied` from both routes, as the sibling plans
do. (Chronicle's *auto-applied* outcome is fine: the panel falls back to the
roll's result, so the budget and the mar gate show on every open panel without
a reload. That is separate from `auto-applied-choice-not-broadcast.md`.)

Pinned by a `test.fixme` in `frontend/tests/e2e/chronicle_histories.spec.ts`,
which asserts the mar case. Un-fixme it with the fix and add the make case; the
spec's `page.reload()` calls on the other players' pages can then go.
