---
title: "demand.prepared makes every client fetch /api/plans/undefined"
order: 79
---

When a Make Demands is prepared, every client at the table sends `GET /api/plans/undefined`, gets a
400, and recovers by reloading the whole plan list. Nothing is visible to a player, which is why it
went unseen; the browser logs a red 400 per player per demand.

**Cause.** `frontend/src/routes/table/[id]/ws-handlers.ts` handles `demand.prepared` in the long case
group that reads `{ plan_id }` from the payload and calls `refreshPlan(ctx, plan_id)`. The server's
`demand.prepared` (`mdHandler.OnPrepare` in `handler/plan_make_demands.go`) sends
`model.PlanPayload{Plan: *plan}`, which is `{ "plan": { "id": … } }`, so `plan_id` is `undefined` and
`getPlan(undefined)` requests `/api/plans/undefined`. The `.catch` in `refreshPlan` calls
`refreshAllPlans`, which masks the mistake.

**Seen by.** `make_demands.spec.ts` › "attaching: bob picks alices pending plan, gives a motivation, and
the demand lands right before it": three 400s, one per open page, all from `demand.prepared`. A frame
probe on that test showed the three `demand.prepared` frames arriving with the plan nested under
`payload.plan`, followed by the three requests.

**To do.** Either read `payload.plan.id` for this one event (it is the only one in the group whose
payload is a plan), or take it out of the group and let the plan's own prepared event do the work.
Check that the page still shows the demand the instant it is prepared.
The e2e suite exercises only this event from that group; the other 28 sent `plan_id` in everything
the suite drove, but were not audited one by one against their Go payloads.

**Then.** Delete the `make_demands.spec.ts` entry (the one naming `demand-prepared-plan-id`) from
`frontend/tests/e2e/allowedConsoleErrors.ts` in the same PR. Until it goes, that spec's console error
is allowed; `make check-fast` fails if this file is deleted while the entry is still there, so the
two leave together.
