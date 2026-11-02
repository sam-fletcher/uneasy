---
title: "Spread Propaganda's panel reads a zero-pick commit as no choice made"
order: 90
---

`SpreadPropagandaPanel.svelte` derives `choicesDone` from the picked options alone
(`existingChoices.length > 0`). The server also records a commit with nothing picked
(`choice_submitted`, read by `choicesCommitted()` in `plans/shared.ts`, which the panel already uses
for its pre-roll fallback), so after a zero-pick mar the picker reappears and the mar's follow-up
steps are never offered. Same bug Seek Answers had; fix by deriving `choicesDone` from
`choicesCommitted()`, with a render test and an e2e like Seek Answers' zero-pick one.
