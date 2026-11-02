# plan-live-updates — one save-and-announce path for plan state

## Design

**Problem.** Each route hand-picked which WebSocket events to send, and the
client refetched a plan only for an explicit list of event names. A route that
saved `resolution_data` and forgot an event left every other open panel (and
the Waiting On bar) stale until reload — about eight instances found by the
e2e specs.

**What gets announced.** Every save of a plan's stored state. Two things go
out, after the request has finished:

1. `plan.updated {plan_id}` — new generic event, id only. No plan data, so
   nothing hidden can ride on it.
2. The row state (`row_state.changed`), unless the handler already broadcast
   it after its last save — the stored plan state decides who the table waits on.

**From where.** `saveResolutionData` (the only function allowed to write
`resolution_data`) notes "plan N changed" on the request context. The new
`AnnouncePlanChanges` middleware (`handler/plan_announce.go`, mounted on the
API group in `cmd/server/main.go`) reads the notes when the handler returns and
announces:

- **After the save, after any transaction commit** — the flush runs once the
  whole handler (and its `InTx`) is done, so a refetch never races a save.
- **Once per plan per request** — a route that saves five times sends one
  event (no storms). Row state uses a sequence number: a broadcast made after
  the last save counts, so the common route that already ends with
  `broadcastRowState` does not send a second one.
- Every other plan-writing query goes through a wrapper in `plan_announce.go`
  that notes the change too: `setPlanStatus`, `setPlanResult`,
  `setPlanPreparationNotes`, `setPlanRowAndOrder`, `setDemandOptionWinners`,
  `setPlanTargets`, `setPlanTargetedPlan`, `setPlanFinaleBonus`.
- Changes that live outside the plan row but alter its screen or the acting
  set call `notePlanChanged(ctx, planID)` by hand: Make War join and surrender
  (war participants are their own table). Checked and already covered: the
  duel stake, bout and claim routes each save `resolution_data`; the other war
  payment/peace routes already broadcast war events and the row state.
- **Status:** the flush announces whatever the response status. A refetch is
  idempotent and reads the true state, so announcing a rolled-back change costs
  one fetch, while skipping a non-transactional save followed by an error would
  leave the table stale. The flush is deferred, so a panic after a save still
  announces.
- **Latency:** the actor's HTTP response is written after the flush (the
  compress layer outside holds it), so it waits for one plan lookup per changed
  plan, and other clients can hear `plan.updated` before the actor's own
  response arrives.

**What the client does.** `plan.updated` → `refreshPlan(planID)`, i.e.
`getPlan`, the viewer-aware endpoint, so per-viewer masking still applies. The
existing latest-wins sequence guard in `refreshPlan` makes an overlap with a
route's own specific event harmless. Only `ctx.plans` is touched, replaced by
a new array (no in-place mutation), with no optimistic append, so none of the
`reference_ws_client_traps` apply.

**Guards.** `handler/plan_announce_test.go`: (a) no Go file may call a raw
plan-writing query (`planWriteQueries`) except the wrappers and
`saveResolutionData`, with a per-file, per-query allowlist giving the reason
(`ShiftRowOrderAtOrAfter` only re-ranks neighbours); (b) the middleware sends
exactly one `plan.updated` per plan and one row state per game, skips a
repeat row state, and still announces on an error status or a panic.
`cmd/server/plan_announcer_mount_test.go` walks the real router and requires
the middleware on every `/api/plans/*` route.

## What changed

- `handler/plan_announce.go` (new), `plan_contract.go` (`saveResolutionData`
  notes the change), `row_state.go` (`broadcastRowState` marks itself),
  `model/ws.go` + `frontend/src/lib/ws.ts` (`plan.updated`),
  `ws-handlers.ts` (+ a unit test).
- **Duel ordering** (`plan_propose_duel_bouts.go`): `duel.bout_resolved` now
  broadcasts after the swapped initiative is saved.
- **Make War join / surrender** call `notePlanChanged`, so the Waiting On bar
  gets the joiner (row state rides the same flush).
- Six tracker items deleted; every e2e `test.fixme` pointing at them is a
  live `test` now (Make Introductions ×3, Make War join, Spread Propaganda,
  Propose Decree make, Propose Duel bout 2, Seek Answers bystander,
  Chronicle mar). Every reload workaround these bugs forced was removed from
  the specs (Make Introductions, Spread Propaganda, Propose Decree, Seek
  Answers, Chronicle) and their comments fixed, so the specs prove updates
  arrive live.

## How it was verified

check-fast, integration (private DB) and three consecutive full e2e runs against a private server (133 passed, 16 skipped each) passed. Disabling the note in saveResolutionData made 5 of 7 live specs fail (they bite). Make War spec runs at 375px; I did not separately eyeball the bar by hand.

## Behaviour changes to look at before merging

- The flush loops until nothing new is noted, because sending the row state
  can itself save plans (it kicks off a pending plan's resolution, or advances
  the finale). Routes that never broadcast row state before can now trigger
  that kickoff / finale advance at the end of the request.

- Each plan-state request now costs one extra `GetPlanByID` per changed plan
  at the end of the request, plus a row-state recompute when the handler had
  not already sent one.
- Clients that already refetch on a route's own event (e.g.
  `plan.choice_applied`) now get a second `plan.updated` and fetch twice.
  Latest-wins keeps it correct; the doubles can be pruned route by route later.

## Open questions for the owner

None.

## Noticed, not fixed

- The Duel reorder (`bout.resolved` after the save) is belt-and-braces: the
  generic `plan.updated` at the end of the request would mask a revert, so no
  test pins the order itself.
- A Make War join after the reveal has closed (pending participant) changes no
  acting set until the war's row arrives, so the bar correctly stays put and
  there is no live state to pin; the log line is the live signal. I tried an
  e2e for it and removed it.
- `destroyed-asset-name-live-vs-reload.md` (new item): the Seek Answers
  "Resolved so far" line names a destroyed asset after a reload but not live.

- Redundant explicit plan broadcasts in routes (see above) could be removed
  now that the generic event covers them; left alone to keep this change safe.
- `liaise-keep-secret-not-broadcast` (the Clandestinely Liaise fixme) is a
  secret-visibility refetch, not a plan save; belongs with the secrets work.
