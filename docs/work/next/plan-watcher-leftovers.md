---
title: "Plans: the pickers a watcher still sees as a line, and the difficulty left as a button label"
order: 79
---

The legibility pass gave every rolling plan a difficulty meter and a "What can happen?", and showed a
watcher the actor's picks as a greyed mirror in Spread Propaganda, Exchange Courtiers and Make
Introductions (see `notes/plan-legibility-78ff06.md`, which has the per-plan inventory). Three things it
did not do, each small:

- **Pickers a watcher still sees as a line.** Make Demands' draft (the next player to pick sees four
  option cards; everyone else sees finished-pick lines and the heading); Propose Decree's join-the-council
  picker (leverage one asset), its amendment and its addendum; Exchange Courtiers' accept-or-decline
  (two buttons, not a pick). The mirror recipe is `.resolve-mirror-wrap` in `plans/planPanel.css`, and a
  card picker's `readOnly` now covers marginalia rows too. Text steps are not on this list: a watcher is shown
  no live draft of a resolution step, so a mirror of a text box could only ever be empty.
- **Host Festivity's difficulty** is only the number on each guest's Roll button (the host's esteem status).
  `plans/shared/PlanDifficulty.svelte` already works it out; the open question is where a meter would sit
  in an event with no single roll.
- **The meter after the dice.** Until they land only; whether it should stay for the plans whose mar owes
  "difficulty minus result" is [plan-meter-after-the-dice](../questions/plan-meter-after-the-dice.md).

**Files:** `plans/MakeDemandsPanel.svelte`, `plans/ProposeDecreePanel.svelte`,
`plans/ExchangeCourtiersPanel.svelte`, `plans/HostFestivityPanel.svelte`, `plans/festivity/SocializingTurn.svelte`.
