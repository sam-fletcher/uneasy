# demand-overlay — hand-over note

## What changed and why
`TargetPlanDemandOverlay.svelte` now shows each form only where its server
route would act:
- **Retarget form:** only while the plan is pending/resolving and before its
  roll has landed (`/demand-retarget` answers 409 then). A preparer who won
  the option still gets it (the route accepts them and it is their only way to
  re-aim), but without the "roll waits on you" copy, since nothing holds the
  roll for them.
- **Leverage form:** not to a preparer-winner (their own leverage uses the
  ordinary button), only while resolving, and only with an open roll for this
  plan (`/demand-leverage` 409s with none). Plans with no roll of their own
  keep the "dud" note.
- **Copy:** "until the plan resolves" is now "until the dice land" on plans that
  roll, "until the plan resolves" for Liaise and "until its roll resolves" for
  Duel and Festivity. The banner's "the roll waits until they say" disappears
  once the winner has settled or the dice have landed.
- **Banner:** the keep_assets line is skipped when `demandOptionInertReason`
  says the target awards no assets (Seek Answers, Chronicle Histories).

The overlay learns about the roll from the page's active roll, published by
`PlanPanel` through a small Svelte context (`demand/activeRollContext.ts`),
chosen over a new prop so the eleven panels stay untouched. `plans/shared.ts`
is unchanged. An active roll that belongs to another plan is ignored.

## How it was verified
- `make check-fast` and the Make Demands e2e spec (3 runs) against my own
  server and private database; see the PR for the final numbers.
- Render tests in `TargetPlanDemandOverlay.test.ts` bite: with the overlay
  change stashed, most fail.
- Not run: `make test-integration` (no Go changes).

## Behaviour changes to look at before merging
- A preparer who won keep_or_change_target sees the retarget form without the
  "roll waits" line (see the open question).

## Open questions for the owner
What should the option do when the target plan's own preparer wins it? See
`docs/work/questions/make-demands-preparer-wins-retarget.md`.

## Noticed, not fixed
Nothing.
