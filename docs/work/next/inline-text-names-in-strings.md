---
title: "Inline text: colour the names that still sit inside strings"
order: 68
---

The inline-text piece (`PlayerName`, `AssetName`, `Quote`; the exceptions table is "Inline text" in
`docs/STYLE_GUIDE.md`) moved every name that is written in markup. A component can't go inside a string,
so names built into strings are still plain. Each needs its markup reshaped (a snippet or the parts passed
separately) or its ids passed down, not a one-line swap. The places found:

- **Props that take a string:** `ownerLabel`, `title` and `label` props (Chronicle Histories, Re-Aim,
  Festivity, Seek's pickers, Make Demands' option and plan cards), and `aria-label`s.
- **`EndsNote` text** and `shared/DelayThresholdNote` (it takes `preparerName` as a string).
- **Make Demands' option lines** (`demandOptionLine`, which builds a sentence from `playerName`).
- **Prologue and Shake-Up labels:** "Owned by …", "held by …", the prologue turn card's
  `activeName` and its "took X from <owner>" clause (the claim carries names, not ids).
- **`SimultaneousRevealInput`:** its participants have only `display_name`, so the reveal list and the
  "waiting on" names are plain. It needs the roster, or seats on the participants.
- **`warName` in `WarBand`, `WarName` and `WarSheet`:** a war's name is built from player names.

The step heading's who-line is done: the Round 6 piece colours its names, as `PlayerName` components. A step
*label* (the heading under it) is a plain phrase and names no one.

Keep to the table: names in their colour (no shape inside a sentence), assets italic in the colour around
them, "you" and "your" plain, and the control's colour inside a button or chip.
