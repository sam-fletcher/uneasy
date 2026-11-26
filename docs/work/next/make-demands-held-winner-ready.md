---
title: "Make Demands: a held winner's ready resolves the roll before they decide"
order: 81
---

A Make Demands winner who has taken control_leverage or keep_or_change_target holds the target's roll
until they settle it. The server seeds them unready and never auto-readies them, but it accepts an
explicit ready from them, and in an e2e run with the winner's decision held back their ready alone
resolved the roll: the decision never reached the server. The roll panel (`DiceRollPanel.svelte`)
doesn't gate the Ready button on that decision either.

**The reading (moved here from a question, 2026-11-26).** This breaks the hold's own promise. A window
that holds a roll is built from three parts (a persisted decided flag, a row kind naming the chooser,
an unready seed) so that the roll can't resolve until the chooser has chosen. A ready that skips the
decision is a bug, not a rules question.

**Fix:** `SetReady` (`handler/rolls_leverage.go`) refuses a held winner's ready with a 409 until their
decision is stored, and the panel shows the decision first and Ready only after it. Check the sibling
paths that can resolve a roll (the auto-ready sweep in `handler/rolls_stage.go`) for the same gap.

**Done when:** an integration test shows the held winner's ready refused before the decision and
accepted after; the "form is gone" specs in `frontend/tests/e2e/make_demands.spec.ts` assert the
decision landed before the readies.
