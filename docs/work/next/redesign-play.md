---
title: "Redesign: the Play pane and the plan frame"
order: 40
---

**Starts after** the table shell PR merges.

- **The play area is the plan's steps, in order:**
  - finished steps collapse to one line;
  - only the open step is expanded;
  - steps still to come don't show.
- **The open step's heading is the current ask:** "Your move — …" or "Waiting on otto — …".
  - The Waiting On bar is deleted, which ends the shell's stopgap.
- **The dice roll is a step:**
  - a pending roll splits its dice into "For" and "Against";
  - each die's edge is its owner's colour;
  - the die mark replaces the 🎲 emoji.
- **Boxes and fills go:**
  - the resolving plan and the roll panel lose their boxes and the gold wash;
  - notices become a mark and coloured text;
  - status words lose their chips.
- **Choices** (aid or interfere, easier or harder) wear no meaning colour. The one picked is bright gold.
- **`planPanel.css`** is shared by every plan panel, so this restyles all twelve at once. The plan-panel items then migrate each panel's own styles.
- **Files:**
  - `phases/MainEventView.svelte`, `PlanPanel.svelte`, `plans/planPanel.css`, `DiceRollPanel.svelte`;
  - `WaitingOnBar.svelte`, which is deleted;
  - `plans/shared/*`, `plans/ResolvingCard.svelte`, `RowPill.svelte`, `ChoicesApplied.svelte`, `MakeMarPicker.svelte`;
  - `SimultaneousRevealInput.svelte`, `D6Face.svelte`, `CardPicker.svelte`, `FormField.svelte`, `PlayerChips.svelte`;
  - `shared/choicePip.css`.

**Done when:** the PR has drive-plan screenshots at 375px, as the actor and as a bystander, of:
- one plan in preparation;
- a pending roll with aid and interference;
- a finished plan.
