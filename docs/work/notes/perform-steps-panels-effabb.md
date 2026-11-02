# perform-steps-panels

## What changed and why
- Six plan panels (Propose Decree, Exchange Courtiers, Seek Answers, Spread Rumors, Spread Propaganda, Chronicle Histories) decided the post-roll actor as `isPreparer || winner`, where `winner` came from the demand overlay's bindable (empty on first render). They now use `resolutionActorID` from `plans/shared.ts`, which mirrors the server's `actsForPreparer`, so a perform_steps winner is the only actor and the preparer sees a wait line. Pre-roll steps (narration, scene, casting, fair-trade offer) still use `isPreparer`.
- Gates found that were still on the preparer although the server uses `requireResolutionActor`: Propose Decree's "Enact the law" form and Seek Answers' sub-flow area (`canSubflow`). Both now follow the actor. Wait lines that named the preparer (Propose Decree, Chronicle, Seek Answers, Spread Rumors `actorName`, Exchange Courtiers) now name the actor; Exchange Courtiers shows the preparer a wait line on a winner-driven make instead of nothing.
- Review fixes: Seek Answers' "Complete plan" is preparer-only again (the server's CompletePlan is), with a wait line for the winner; Propose Decree's addendum goes to the winner when the signatory is the preparer (third-party signatories unchanged); the Exchange Courtiers wait clause is limited to a make; remaining wait lines name the actor; render tests assert the actor's name and cover addendum, completion and pre-roll.
- Seek Answers `choicesDone` also counts `hasCommittedChoices` (the server's committed marker), so a zero-pick mar no longer shows the picker again.
- Un-fixme'd the two e2e tests (`make_demands.spec.ts` preparer wait line; `seek_answers.spec.ts` zero-pick and perform_steps). New `PerformStepsPanels.test.ts` renders each panel as winner and preparer.

## How it was verified
- `make check-fast` passes. New render tests fail on all 12 cases with the panel changes reverted.
- `make test-integration` on a private DB passes.
- Full e2e (139 passed, 11 skipped, 0 failed) against my own server on :8193.
- 375px as winner, preparer and bystander for Seek Answers and Spread Rumors (screenshots, text checked).
- Propose Decree addendum checked at 375px as the winner (form shown) and a bystander (waits on the winner).
- Not run in a browser: Exchange Courtiers, Chronicle, Spread Propaganda; render tests only.

## Behaviour changes to look at before merging
- Under perform_steps the preparer no longer sees any form after the roll on these panels, only who is acting.

## Open questions for the owner
- [Who breaks the riposte peer on a mar under perform_steps?](../questions/exchange-courtiers-riposte-under-perform-steps.md)

## Noticed, not fixed
- Spread Propaganda's zero-pick `choicesDone`: filed in `next/`.
- Winner-acting edges left alone: a question target's veto ranks against the preparer; a hide-source secret lands on the winner's own asset; Chronicle's "Wrap the scene" stays with the preparer.
- Exchange Courtiers shows blank panels on a mar to some viewers (existing item `exchange-courtiers-blank-waiting-panels`).
- The Make Demands overlay still shows its leverage and retarget forms after the dice land (another session owns it).
