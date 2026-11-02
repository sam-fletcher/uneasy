---
title: "Propose Duel: a claim naming a non-staked asset returns 500"
order: 86
---

`POST /api/plans/:id/make-choice` with an asset the loser owns but never staked
fails in `pduelHandler.ApplyChoice` ("asset N is not one of the losing side's
staked assets") with a plain error, which the route turns into a 500. It is a
client error; the over-budget claim already answers 422 and this should too, with
nothing half-applied (the validation already runs before any change).

Pinned by `test.fixme('a claim naming a non-staked asset is refused with a 4xx…')`
in `frontend/tests/e2e/propose_duel.spec.ts`, which expects 422; un-fixme it with
the fix.
