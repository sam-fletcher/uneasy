---
title: "Redesign: the war band and the war panel"
order: 50
---

**Starts after** the Play pane PR merges.

- **The war band:** each live war gets a one-line band at the top of Play. It has:
  - war's red border and the barbute;
  - the war's name, "otto's war on Wren", adding "and others" when the declarer named more;
  - no red fill.
  - It replaces the shell's stopgap and the old drawer.
- **The war's panel** opens from the band, as a sheet over Play:
  - the sides are named after their leaders: "Join otto", "Join Wren", "Stay out";
  - joining takes two steps. You pick, the cost appears under your pick, and a main button names what you're committing to ("Join otto's side");
  - the entry cost is plain secondary text;
  - Join and Stay out wear no meaning colour.
- **When a cost of battle is due,** the war takes over the play area, as it does today.
- **Files:**
  - `plans/war/*`, `plans/MakeWarPanel.svelte`, `lib/warDrawer.ts`;
  - the band's mount in `phases/MainEventView.svelte`.
