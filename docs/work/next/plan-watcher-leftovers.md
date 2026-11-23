---
title: "Plans: Host Festivity's difficulty, and the meter after the dice"
order: 79
---

The legibility pass gave every rolling plan a difficulty meter and a "What can happen?", and showed a
watcher the actor's picks as a greyed mirror (see `notes/plan-legibility-78ff06.md`, which has the per-plan
inventory; `notes/watcher-leftovers-6b3edb.md` finished the pickers it left as lines). Two things remain,
each small:

- **Host Festivity's difficulty** is only the number on each guest's Roll button (the host's esteem status).
  `plans/shared/PlanDifficulty.svelte` already works it out; the open question is where a meter would sit
  in an event with no single roll.
- **The meter after the dice.** Until they land only; whether it should stay for the plans whose mar owes
  "difficulty minus result" is [plan-meter-after-the-dice](../questions/plan-meter-after-the-dice.md).

Text steps are not on this list and stay lines: a watcher is shown no live draft of a resolution step, so a
mirror of a text box could only ever be empty. That covers Propose Decree's amendment and addendum.

**Files:** `plans/HostFestivityPanel.svelte`, `plans/festivity/SocializingTurn.svelte`.
