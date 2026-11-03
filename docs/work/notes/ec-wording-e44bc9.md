# Exchange Courtiers: wait lines and Riposte wording

## What changed and why
- `ExchangeCourtiersPanel.svelte`: every seat that isn't acting now gets a
  third-person wait line naming the actor. Before the offer a bystander sees
  "Waiting for <target> to propose a fair trade…". After the offer only the
  target sees "You asked for …"; everyone else sees "<target> asked for …".
  After the roll, anyone who isn't the chooser sees "Waiting for <actor> …":
  on a mar the actor is the target, on a make it is `resolutionActorID` (so a
  Make Demands perform_steps winner is named, integrating with PR #49's wait
  clause rather than duplicating it). The old branch skipped the target
  during a make and the preparer during a mar, leaving blank cards.
- `MAR_OPTIONS` Riposte now reads for the chooser ("They may break the requested
  peer first, then you take it"); the Buffet's mar tab says "the preparer may
  break it first" instead of "you".
- The four pinned e2e `test.fixme`s are live; new render tests per seat in
  `ExchangeCourtiersPanel.test.ts`; `PerformStepsPanels.test.ts` updated for the
  new wait wording.

## How it was verified
- Vitest (621 pass) and svelte-check (0 errors) against this worktree.
- The new render tests fail on the old panel (7 of 9) and pass on the new one.
- `exchange_courtiers.spec.ts`: 12/12 against a private server (:8193, private DB, since dropped).
- 375px shots: bystander pre-/post-offer, target post-offer, preparer and bystander waiting on a mar, target's mar picker.
- Not run: `make test-integration` (no Go changes).

## Behaviour changes to look at before merging
- Wait copy changed for the perform_steps case: "<winner> is resolving Exchange
  Courtiers…" is now "Waiting for <winner> to say how the trade went…".

## Open questions for the owner
- Who breaks the riposte peer under perform_steps — see
  `docs/work/questions/exchange-courtiers-riposte-under-perform-steps.md`.
  Left alone; the panel still shows the break to the preparer only.

## Noticed, not fixed
- Under perform_steps the winner, if also the target, is shown the mar picker
  by `choiceActor` only on a mar; no change needed, just worth a look in that question.

## Review follow-ups
- The final wait line is guarded on a make outcome, with "Dice roll in progress…" in the window before the roll arrives.
- The pre-offer bystander line names the preparer when they have no peers to trade.
- The Riposte e2e also asserts the positive wording.
