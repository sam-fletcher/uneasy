# fix-wars-list-race

## What changed and why

The 2026-11-08 nightly e2e run failed on one spec, `make_war.spec.ts ›
surrender: the claimant takes an asset of their choice…` (issue #69): a
player's "Row advance blocked" bar never cleared after the claim was taken.

Cause, found by reproducing it (about 1 run in 30 with the CPU saturated, never
on an idle machine) and catching the page's exceptions with a debugger:

1. `GET /api/tables/:id/wars` built its list from two separate queries — active
   wars, then ended wars with an open surrender claim — appended together.
   A war that ended between the two reads was in both, and came back twice
   (`status: active` and `status: ended`). Clients refetch `/wars` the moment
   `war.player_surrendered` is broadcast, which is while the surrendering
   request is still running, so the window is real.
2. `warGatePlans` pushed the war's plan once per list entry, so the play area
   rendered the same plan twice in a keyed `{#each … (p.id)}`.
3. Svelte threw `each_key_duplicate`; the table page's error boundary caught it
   and froze its region, waiting-on bar included, until a reload. The server
   and the WebSocket were fine throughout: every client got the right
   `row_state.changed`.

The fix, in two layers:

- **Server (the cause):** one query, `ListActiveOrClaimedWarsByGame`, replaces
  `ListEndedWarsWithOpenClaimsByGame` for `ListWars`. One statement is one
  snapshot, so a war is listed once. (`ListActiveWarsByGame` stays for the
  other callers.)
- **Client (so a bad list can never freeze the table):** `dedupeWars` in
  `warGate.ts`, applied where `MainEventView` stores the list (later entry
  wins, it was read later); and `warGatePlans` never returns a plan twice.

## How it was verified

Against this worktree's own code:

- `make check-fast`: passes (one svelte-check warning in `HelpContent.svelte`,
  not from this change).
- `make test-integration` on a private database: `handler` and the other
  packages pass. `cmd/server` needs the embedded frontend, so it ran after
  `make build`.
- New tests bite: `TestMakeWarHTTP_ListWars_NeverRepeatsAWarWhileItEnds` fails
  on the old two-query code at poll 2 with the same active + ended pair, and
  passes now; the `warGatePlans` duplicate case fails with its guard removed.
- e2e on my own server (production build, private DB, port 8192): the failing
  spec **150 of 150 passed with 20 CPU-burning processes running**, where it
  failed roughly once per 20–40 runs before; then all of `make_war.spec.ts`
  (18 tests) passed unstressed.

Not run against this code: the rest of the Playwright suite, and the nightly
workflow itself (it runs on `main` once this merges).

## Behaviour changes to look at before merging

- `GET /wars` now returns wars ordered by id as one list. Before, active wars
  came first, then ended-with-claims ones. Nothing in the client depends on the
  order (it looks wars up by `origin_plan_id`).
- Where `dedupeWars` meets a repeated war it keeps the later entry.

## Open questions for the owner

None.

## Noticed, not fixed

- The table page's error boundary has no `onerror`, so it swallows what it
  catches. Filed as `docs/work/next/table-error-boundary-silent.md`.
- `ComputeRowState` (`handler/row_state.go`) reads wars and surrender claims in
  parallel goroutines without a shared snapshot, so in principle a row state can
  mix two moments. Nothing has been seen to go wrong from it; a snapshot (one
  transaction) would be the fix if it ever does.
- `deadcode` reports `middleware.AccountContext` as unreachable; it did before
  this change.
