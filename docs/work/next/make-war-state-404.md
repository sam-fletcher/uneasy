---
title: "Make War: the war-state fetch 404s, and logs a console error, before a war exists"
order: 82
---

Every browser at the table logs a red `404` in its console whenever a Make War plan's panel is shown
before its war has been declared. Nothing is wrong on screen: it is the answer the app is built to get.

**Cause.** `MakeWarPanel.svelte` (around line 80) calls `getWarState(plan.id)` when it mounts.
`GetWarState` in `handler/wars.go` (around line 111) answers `404 "no war exists yet for this plan"`
until the war row exists, and the panel treats that as "no war yet". The browser still prints a
"Failed to load resource" console error for any 4xx, so a normal state looks like a fault.

**Seen by.** The e2e guard, once, in `public_record.spec.ts` › "Events: a war the server stored as
"make" is not shown as a Make". The server's request log shows the page's two `war-state` fetches
(both 404) finishing *before* the test's own `POST /api/plans/N/resolve` returned: the page hears the
plan's event while the resolve is still in flight and asks for the war, which is not there yet. It is
a timing race, so it comes and goes (the test passed in three full runs and 20 of 20 repeats before it
tripped). `make_war.spec.ts` shows a Make War panel in many tests and is the other spec that could
trip it; it has not yet (its two 404s on this route in the recording run came after the game was
deleted).

**To do.** Either of these removes it:
- **Skip the fetch until a war can exist.** `refreshWar` runs on mount and on every war or plan
  event, and a 404 is its "no war row yet" answer; the panel would ask only once the plan is resolving
  or a war id is on its `resolution_data`. No API change, so nothing an open tab can trip over.
- **Or the server answers `200` with no war** (for example `{ "war": null }`) instead of `404`. That
  changes the response's shape, so it is a one-way door for tabs left open across the deploy: the old
  `refreshWar` assigns whatever a 200 returns to `war`, so it would read `{ "war": null }` as a war.
  Check what it renders before choosing this, update `WarStateResponse` in
  `frontend/src/lib/api/plans.ts`, and note that `mwLoadWar` in `handler/plan_make_war.go` (around line
  308) is a second place that answers 404 for the same condition, on the plan's own routes.

**Then.** Delete the `public_record.spec.ts` entry (the one naming `make-war-state-404`) from
`frontend/tests/e2e/allowedConsoleErrors.ts` in the same PR. Until it goes, that one test's 404 is
allowed; `make check-fast` fails if this file is deleted while the entry is still there, so the two
leave together. If the 404 trips `make_war.spec.ts` before this is fixed, the guard's failure message
will say so; add a matching entry for that test, naming this item.
