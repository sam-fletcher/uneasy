---
title: "Propose Duel: the declarer's page keeps the old initiative after the responder answers"
order: 85
---

After a bout resolves, the server hands initiative to the other duellist
(`plan_propose_duel_bouts.go`, "Swap initiative"). The duellist who did **not**
click last (the declarer, who is waiting on the responder's answer) sometimes
keeps the old plan data: their panel still reads "<me> to declare" and offers a
Declare button, which the server then refuses with a 403. The responder's own
page is right, and so is the bystander's header in most runs.

It is intermittent (about one run in four with two stakes each). The duel-state
refetch on `duel.bout_resolved` brings the bout list up to date ("Bout 2 of 2"
renders), but the initiative lives in the plan's `resolution_data`, which the
declarer's page only refreshes from a later push, and the stale value persisted
for more than five seconds when it happened. The responder is fine because their
own click calls `onPlansChanged`. The bout-resolved broadcast also goes out before
`saveResolutionData` writes the swapped initiative, which may be the race.

Fix options: save the swapped initiative before broadcasting, and/or refetch the
plan on `duel.bout_resolved` in `ProposeDuelPanel.svelte`.

Pinned by `test.fixme('bout 2: both duellists agree whose turn it is')` in
`frontend/tests/e2e/propose_duel.spec.ts`; un-fixme it with the fix.
