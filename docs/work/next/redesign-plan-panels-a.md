---
title: "Redesign: plan panels: Spread Rumors, Seek Answers, Spread Propaganda, Chronicle Histories"
order: 50
---

**Starts after** the Play pane PR merges. That PR restyles `planPanel.css`, which these panels share.

- **Each panel's own styles** move onto the tokens and the rulings:
  - groups have no boxes;
  - status words are not chips;
  - notices are a mark and coloured text;
  - choices wear no meaning colour;
  - selected is the bright gold border.
- **Check each panel's steps** read like the Play pane's step list.
- **Screenshots in the PR:** drive-plan at 375px for each plan, in preparation and in resolution, as the actor and as a bystander.
- **Afterwards:** once this merges, `plan-legibility-pass` and `plan-polish-leftovers` can start for these plans.

**Files:** `plans/SpreadRumorsPanel.svelte`, `SeekAnswersPanel.svelte`, `SpreadPropagandaPanel.svelte`, `ChronicleHistoriesPanel.svelte`.
