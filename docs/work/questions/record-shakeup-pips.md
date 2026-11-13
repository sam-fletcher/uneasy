---
title: "Does the Shake-Up row keep its three E / K / P pips?"
---

The Shake-Up row after row 13 used to carry three small circles, E, K and P, filling in as the
Esteem, Knowledge and Power categories finish. The signed-off mockup of the Events list draws that
row with only the burst and its name, and shows no pips (it draws the row before the Shake-Up has
started, which is when the old pips were all empty and faded).

I kept the pips, because they are the only place the record shows how far the Shake-Up has got, but
they now appear **only once the Shake-Up is under way**, filled for finished categories and outlined
in deep gold for the one in play. Before it starts there is nothing to fill in, so the row matches
the mockup.

- **Keep them as they are (what is built).**
- **Drop them.** The title bar already says which category the Shake-Up is on ("Shake-Up · Esteem"),
  so the pips repeat it. It is deleting `showPips` and the `.shakeup-pips` block in
  `PublicRecord.svelte`.
- **Draw them as the three-letter track codes** (`POW`, `KNO`, `EST`) the Prologue uses, instead of
  single letters, so a track reads the same everywhere. They are wider, so they may not fit beside the
  label at 375px.
