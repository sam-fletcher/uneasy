# spec-propose-duel — Propose Duel regression specs

## What changed and why

Extended `frontend/tests/e2e/propose_duel.spec.ts` (the original random-outcome
test is kept) with seven specs, plus a helper file
`frontend/tests/e2e/helpers/propose_duel.ts`. Each pins something the 12-plan UX
audit fixed or the duel rulings settled:

- **Setup:** arms/wits heading, champion election (one log line on Confirm, the
  opponent's "Facing…" follows), stake cap and exact refusals (400 over cap, 403
  not your asset / bystander, 409 second commit), blind stakes (the opponent and
  a bystander see none; the stake log waits for both; the bar names only the
  duellist still owing), what a bystander sees and can't do.
- **Bouts, hidden information:** each duellist sees their own dice and `?` for the
  other side's; a bystander sees only `?`; checked through `duel-state`, the
  rendered tracker, and every WebSocket frame the responder and bystander receive
  while a bout is open (none contains a die). The call is public (callout, log
  line); no `rolled` line on the log until the bout resolves; dice go public on
  resolve; the next bout's dice stay sealed.
- **Make** and **mar** with one stake each, through the UI at 375px: the claim
  applies itself, the plan resolves with no click for anyone (no "Complete plan"
  on the losing preparer), the choice-committed marker (#27) is set, the assets
  change hands and everything staked ends leveraged, the log names what was taken.
- **Mar with a real pick** (3 stakes vs 1): only the mar winner is asked; the
  loser and the bystander see "Waiting for bob…"; the bar names the winner; exact
  refusals (403 loser claims, 422 over budget); nothing moves until they pick.
- **Make with a real pick** (bob fourth on the track: difficulty 2, 3 stakes):
  the preparer picks 2 of 3, the unclaimed one is still leveraged.
- **Final-bout tie:** zero dice, a mar, and the log says why.
- Two `test.fixme`s for new bugs (below).
- The compulsory-take assertions (forced mar, tie, exact-take picker) sit together in one `describe`, commented as following the current implementation and changing if the owner rules "may take".

Outcomes are deterministic without touching the database: each duellist can read
their own dice from `duel-state`, so the helper stakes, reads both first dice and
re-seeds until they give the wanted bout 1 (alice wins → make, bob wins → mar,
equal → tie), then the spec plays the bout through the UI.

## How it was verified

All against a server built from this branch (`duel-c861dd-srv`, port 8163,
private database): the file passes 3 runs in a row (8 passed, 1 skipped);
`make check-fast` passes. Each spec shown failing against a mutated build:
bout/stake dice unmasked, a die in the `bout_declared` frame, setup stakes
visible to the opponent, the declaration log removed, the choice marker removed,
`AutoCompleteAfterChoice` off, the forced claim off, the final-tie log line
removed, the mar's Waiting On bar naming the preparer. Not run: the integration
suite (no Go changed), and the stake-log-waits-for-both assertion was not
mutation-checked (the Go gate is awkward to mutate).

## Behaviour changes to look at before merging

None; no product code.

## Open questions for the owner

None new. The compulsory-take question is not decided by these specs: the assertions that depend on it are grouped in one block and flagged. The Make Demands retarget wait on Duel is not pinned.

## Noticed, not fixed

- A claim naming a non-staked asset returns 500 (`claim-non-staked-asset-returns-500.md`, `test.fixme`).
- Blind stake count: pinned over `GET /plans/:id` and the sockets of the opponent and bystander (mutation-checked).

- New bug: the declarer's page keeps the old initiative after the responder
  answers (`docs/work/next/duel-stale-initiative-after-bout.md`, pinned by a
  `test.fixme`): the bout-resolved broadcast fires before the swapped initiative is saved.
- A truly zero-claimable-stakes commit (the #27 marker with nothing to take) can't
  be reached through the UI or API: every duellist stakes at least one asset and a
  mar's budget is at least 1. The marker is pinned end to end on the claims that
  do happen (forced make, forced mar, mar pick).
- The `auto-applied-choice-not-broadcast` and `perform-steps-panels-gate-on-preparer`
  items did not show up in the duel: the forced claim cleared both players' panels
  live without a reload. (The perform_steps seat for Duel is a known gap in memory
  and not pinned here.)
