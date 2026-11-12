---
title: "Redesign: shared pieces and /dev/styleguide"
order: 20
---

**Starts after** the tokens and guard test PR merges.

The pieces every later redesign PR builds from, and one page that shows them all.

- **House marks, one component each:**
  - the five player shapes, drawn wherever a colour stands for a player;
  - the clock, which is the waiting glyph;
  - the barbute, which is war's mark;
  - the die, replacing the 🎲 emoji in the roll panel;
  - the pale "new" dot.
- **Shared recipes:**
  - buttons: main (deep gold), secondary, and greyed out;
  - chips, selected or not. Selected is a 2px bright gold border and bright gold text, with no fill;
  - the tappable row: a caret and lines, no box;
  - status words: small uppercase text, not a chip;
  - notices: a mark and coloured text, not a tinted box;
  - sheets.
- **`/dev/styleguide`:**
  - shows every token and every piece above, in each state;
  - the tokens are the sizes, gaps, radii, surfaces, text shades, borders, both golds, the meaning colours, and the player colours with their shapes;
  - it renders the real components, not copies, so it can't drift from the app;
  - it ships to the live site, but nothing links to it.
- **Files migrated here:**
  - `shared/actionButton.css`, `statusText.css`, `cornerBadge.css`, `jumpPulse.css`, `modalShell.css`;
  - `RetinueSheet.svelte`, `shared/ChecklistRow.svelte`, `shared/TableRoster.svelte`, `CrownGlyph.svelte`, `CharCounter.svelte`.

**Done when:**
- every piece exists and the files above use it;
- the styleguide shows it all at 375px and at desktop width;
- those files' guard markers are gone.
