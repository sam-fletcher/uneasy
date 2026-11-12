---
title: "Redesign: the retinue and the game pieces"
order: 30
---

**Starts after** the shared pieces PR merges. It doesn't need the shell.

- **Game pieces:** asset and peer cards and marginalia tiles. Each has:
  - a warm border at about 3:1 against the page;
  - corner radius 4;
  - the raised surface.
- **Selected:**
  - a 2px bright gold border and a tick;
  - the gold wash goes.
  - The tick may be clutter on cards that carry other elements. If it is, say so in the PR rather than dropping it.
- **Owner marks** on cards use the player's shape.
- **The at-risk count** lives in the retinue. Chips elsewhere show a warning mark without a number.
- **Leveraged** stays orange, beside its word.

This screen was never mocked up. Apply the rulings as written, put before-and-after screenshots at 375px in the PR, and file a question in `docs/work/questions/` for anything the rulings don't settle, rather than guessing.

**Files:**
- `RetinueView.svelte`, `AssetCardSelectable.svelte`, `AssetCreationForm.svelte`;
- `MainCharacterChoicePanel.svelte`, `SuggestionPicker.svelte`, `shared/marginaliaTile.css`.
