---
title: "Make Introductions' choicesDone still reads picked options, not the committed marker"
order: 91
---

`MakeIntroductionsPanel.svelte` computes `choicesDone` as `existingChoices.length > 0`, unlike
Seek Answers, Spread Propaganda and Spread Rumors, which read `choicesCommitted()`. It is not a
one-line swap: `ForcedChoice` (`handler/plan_make_introductions.go`) auto-applies every mar with an
empty `make_mar_choices`, so `choicesDone` is false for the whole mar and the `marPending` branches
of the template paper over it. Moving to the committed marker changes those branches, so it needs
its own render tests and an e2e through the mar. Exchange Courtiers has the same line and is
covered by another session in the same batch.
