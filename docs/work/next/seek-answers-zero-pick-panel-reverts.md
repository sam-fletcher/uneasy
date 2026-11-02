---
title: "Seek Answers' panel shows the picker again after a zero-pick mar commit"
order: 84
---

`SeekAnswersPanel.svelte` decides the make/mar choice is made from the picked
options alone (`choicesDone = existingChoices.length > 0`). The server also records a
commit with nothing picked (`choice_submitted`, read by `choicesCommitted()` in
`plans/shared.ts`, which the panel already uses for the pre-roll fallback). So after
a mar committed with `choices: []`, every viewer is shown the picker again
("alice is choosing make/mar options…") and the preparer is never offered the mar
self-flaw penalty that is still owed. The Waiting On state and the plan data are right;
only the panel is stale.

The picker can't produce this (it needs the exact dice result, at least one pick), so it
takes a hand-built or stale client. Fix: derive `choicesDone` from `choicesCommitted()`.

Pinned as a `test.fixme` in `frontend/tests/e2e/seek_answers.spec.ts`; un-fix it with the fix.
