---
title: "Plans: the facts about a plan that sit among its open step's lines"
order: 80
---

Two lines state a fact about the plan rather than ask for anything, so they read as part of the open step when
they belong with the card's finished lines (`plans/shared/FinishedStep.svelte`, drawn by `ResolvingCard`):

- **Make Demands' "Targeting: Seek Answers by alice (row 5)"** (`MakeDemandsPanel.svelte`, the first line of its
  body). It is true for the whole of the plan, and the open step has moved on from it by the time the draft is
  on screen.
- **The demand overlay's lead line** (`plans/demand/TargetPlanDemandOverlay.svelte`, `.demand-banner-header`),
  which names the demand laid against a plan and its winners, above whatever step that plan is on.

Whether each becomes a finished line, a line of the card's first group, or stays is a design call (it
interacts with `plan-set-piece-line`, which asks what a line the game stages looks like), so take it to the
owner with the other two.
