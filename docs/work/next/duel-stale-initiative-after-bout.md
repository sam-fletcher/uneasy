---
title: "Propose Duel: save the swapped initiative before broadcasting bout.resolved"
order: 85
---

After a bout resolves the server hands initiative to the other duellist, but the
duellist who did **not** click last (the declarer, waiting on the responder)
sometimes keeps the old initiative: their panel still reads "<me> to declare" and
offers a Declare button the server refuses with a 403. About one run in four with
two stakes each; the stale value persisted for more than five seconds.

Cause, confirmed in `handler/plan_propose_duel_bouts.go`: the handler sets
`state.InitiativePlayerID` on the in-memory copy, then broadcasts
`duel.bout_resolved` (about line 252) **before** `saveResolutionData` persists the
swapped initiative (about line 294, or inside `pduelCreateFinalRoll` when the bouts
are over). The client already refetches the plan on that event
(`ws-handlers.ts` runs `refreshPlan` for `DuelBoutResolved`), so a refetch that
lands between the broadcast and the save reads the old initiative. The responder
is immune because their own click calls `onPlansChanged` after the response.

Fix: persist the resolution data before broadcasting. No client change needed.

Related but separate: the missing-broadcast items
(`auto-applied-choice-not-broadcast.md`, `make-introductions-step-routes-not-broadcast.md`).
Those are routes that never broadcast; this one broadcasts too early.

Pinned by `test.fixme('bout 2: both duellists agree whose turn it is')` in
`frontend/tests/e2e/propose_duel.spec.ts` (checks both duellists and the
bystander); un-fixme it with the fix.
