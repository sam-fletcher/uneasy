---
title: "Plans: one group gap and one set of step buttons in the plan frame"
order: 64
---

**Starts after** `plan-step-labels`, which merged, so the panels are not edited twice.

With the boxes gone, the groups inside a plan's open step have to read as groups by the space between them,
which has to be visibly bigger than the lines inside a group. The plan frame (`plans/planPanel.css`) puts
12px between every child, so `redesign-plan-panels-c` fixed it locally in its three panels. Move that into the
frame once, and delete the copies:

- **The group gap.** A column of groups with 24px (`--space-5`) between them, written as `.fest-body`
  (Host Festivity), `.setup-groups` (the duel's setup) and `.demand-banner` (the demand overlay). Give the
  frame one rule for it, and check the other plans' panels for the same crowding.
- **The step's button row.** A main button with the other beside it, written as `.step-actions` in four
  files (the duel's `SetupPhase` and `RollPhase`, the festivity's `SocializingTurn` and `ChallengeBanner`: main
  `2 1 10rem`, other `1 1 6rem`) and as `.demand-actions` in the demand overlay (both `12rem`, so long labels
  stack full-width on a phone). `DiceRollPanel` has its own `.step-actions` too. One shared row, with a
  modifier for the stacking one.
- **The count after a lead line** ("· 0 of 1 taken", "· 2 mars to settle"), written as `.taken-count` in
  `HostChoosing` and `HostPendingMar`.

**The class-name moves `plan-step-labels` left for this piece.** Wave 5 moved the notices and most chips onto
`<Notice>` and `<Chip>`; what is left in `plans/planPanel.css` is one component and a few leftovers, and it is
the same file this piece edits:

- **`plans/PlayerChips.svelte`** still draws `.player-chip`. Convert it to `<Chip>` once: many panels and
  `war/BattleCostForm` use it, `shared/steps.test.ts` pins its look to the house chip, and about five e2e specs
  select by the class.
- **`.choice-card`** is used in `MakeDemandsPanel`, `MakeMarPicker`, `ReAimPanel`, `duel/BoutsPhase` and
  `festivity/HostPendingMar`, and needs a shared option-card component.
- **`.chip-btn.face-chip`** is in `SimultaneousRevealInput`, and `D6Face`'s header comment names it.
- **The `.res-warning` rule** in `planPanel.css` has no users left: delete it.
- **Then delete the paragraph in `docs/STYLE_GUIDE.md`, under "Play steps",** that says `planPanel.css` still
  carries the older class names (`.chip-btn`, `.player-chip`, `.choice-card`, `.res-warning`), once none does.

Add whatever is shared to `/dev/styleguide` and `docs/STYLE_GUIDE.md` in the same change.

**Files:** `plans/planPanel.css`, and the files above.
