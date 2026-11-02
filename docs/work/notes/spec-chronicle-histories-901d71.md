# Chronicle Histories regression spec and first UI pass

## What changed and why

- `frontend/tests/e2e/chronicle_histories.spec.ts` and
  `frontend/tests/e2e/helpers/chronicle_histories.ts`: Playwright specs for
  Chronicle Histories, the last Phase 4 plan. 11 live tests and 2 `test.fixme`s (mar `invoke_another` is covered; the 375px
  bystander is a player whose artifact isn't in the scene).
  Covered: preparation (notes required, 400 on empty); the pre-roll (live
  difficulty meter for everyone, preparer-only picker and submit, difficulty =
  max(rank, artifacts invoked) with duplicates counted once, 403 for a
  non-preparer, 400 for non-artifacts, 409 on a second cast); the outcome being
  on record the moment the dice land (make budget; mar gate of 3); a make's two
  options one at a time with a 409 on the third, the wrap-up gate, a tear that
  leaves an artifact standing, the narrative option recording no target; a
  mar's one-choice-per-player (409 on a second), the wrap-up gate until all
  three have chosen, and the preparer choosing too; depletion (a last
  marginalia destroys the artifact, a torn line can't be torn again, an
  un-invoked artifact can't be broken, a destroyed relic leaves the next
  chooser's picker); no asset gained, lost or moved by either outcome; the log
  lines; and, at 375px, what the preparer, another player and a bystander each
  see (no sideways scroll, 44px option chips, controls inert for non-actors).
- Deletes `docs/work/next/phase-4-regression-specs.md` (this was the last
  batch) and drops Chronicle Histories from the README's "Never audited" line.
- Two new tracker items from the UI pass (below).

## How it was verified

All against this worktree's own build (`make build`, a private database, port
8197), not the :8090 container. The file ran 3 times in a row: 11 passed, 3
skipped, each time. Bite checks: I broke one guard at a time in
`handler/plan_chronicle_histories.go`, rebuilt, and ran the file on a mutant
server; each went red in the test meant to catch it — over-pick guard, both
`CanComplete` gates, the one-choice-per-player check, `AutoApplyChoiceOnRoll`,
the already-torn check, the invoked-artifact check, the preparer-only cast, the
artifact-type check, and `chStepTarget` (narrative options recording a target).
Each fixme was run un-skipped and fails for the stated reason. Auto-apply: Chronicle's own path is fine, so it is not part of
`auto-applied-choice-not-broadcast.md`: the spec's "auto-applied outcome reaches
open panels live" test passes with no reload (the panel falls back to the
roll's result for the make budget, and the mar gate and Waiting On bar update),
and the budget/gate being on record from the roll is pinned separately (the make and mar tests fail when `AutoApplyChoiceOnRoll` is turned off).
Not run:
`make test-integration` (no Go change). `make check-fast`: see PR.

## Behaviour changes to look at before merging

None — tests and docs only.

## Open questions for the owner

`docs/work/questions/chronicle-repeatable-make-options.md`: the handler lets a
make pick the same option repeatedly; the book doesn't say. The "dictating a character's actions needs consent" clause stays as
ruled (won't build, for now); the Buffet and panel still word it as "requires
that player's consent", whereas the book says the affected player has final say
and the chooser may unselect — tracked as `chronicle-consent-wording`.

## Noticed, not fixed

UI-pass findings, as tracker items: `chronicle-choices-not-pushed-to-open-panels`
(other players' make steps and mar choices, and the Waiting On bar, are stale
until reload), `chronicle-buffet-mar-says-other-players` (reference tab
contradicts the rules and the panel).
`chronicle-first-timer-confusions` collects the smaller UI-pass confusions. The `perform_steps` winner case is covered
by the existing `perform-steps-panels-gate-on-preparer` item, and I left no
fixme for it, since an empty one would pass if un-skipped — add a real one to
this spec when that fix lands, driven as `make_demands.spec.ts` does for Seek
Answers. At 375px nothing overflowed and the "Waiting On" label no longer
collides with the step label (an earlier audit finding). A destroyed artifact
stays in the "Invoked artifacts (n)" list; I took that as history, not a bug.
