---
title: "Propose Duel: should the pairing set piece show in every step, or only the setup?"
checked: "Round 6 'Inside a step' ruling (private decisions file) read: it names Propose Duel's pairing as a set piece and says nothing about later steps. Style guide 'Play steps' says a set piece stays rare. DESIGN_PRINCIPLES and RULINGS' Propose Duel section don't touch it."
---

The Round 6 piece draws the duel's pairing ("alice's Ser Ott FACES bob's Dame Orr") once, in the setup step,
above the difficulty meter. The bouts and the final roll name the players and peers they mean in their own lines
but draw no pairing, because a staged moment repeated in every step stops being one. The old "alice vs bob" lead
line used to show in every step, so a watcher of the bouts no longer has the duelling *peers* named at the top of
the step.

- **Setup only** (what is built). Rare, and each later step already names its players.
- **Every step.** The pairing is always on screen, at the cost of the set piece becoming furniture.
- **Setup, then a one-line reminder** in the later steps (plain text, not the set piece).

This was my call, not ruled on. The code is `ProposeDuelPanel.svelte` (the `SetPiece` block).
