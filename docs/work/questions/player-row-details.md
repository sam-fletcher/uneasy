---
title: "What does the player row keep: ranks, the at-risk mark, and the 'You' label?"
---

The signed-off mockups draw each player as one 44px chip: their shape, their name in their colour,
and the clock when the game is waiting on them. The table shell is built that way
(`components/table/PlayerRow.svelte`). Four things in it go past what the mockups show, so they are
yours to confirm or change:

- **The Power / Knowledge / Esteem rank line is gone from the chip.** The old chip carried "P2 K1 E4"
  under the name, with the best rank on each track in gold, so relative standing was always on screen.
  A second line doesn't fit the 44px chip in the mockups, and ranks are numbers on the chip. They
  are still at the top of every retinue (the rank strip). If you want standing visible at all times
  again, the choices are a second line (a taller row), a mark per track, or leaving it in the retinue.
  The `rankChip.css` that drew the old line is deleted, and Help's Rankings tab no longer shows it.
- **The at-risk mark is the warning triangle in the at-risk red, with no number.** The rulings say a
  chip "shows a warning mark without a number" but don't draw it. I reused the triangle from the
  mockups (the one that marks a leveraged asset, in orange there) and painted it red, since red is
  at-risk and orange is leveraged. Is that the right mark and colour? The count of at-risk assets now
  lives only in the retinue, where it can be acted on; the retinue piece owns where.
- **Your own chip reads "You", not your name.** That is the app's existing convention (the lobby
  roster does the same) and it costs less width, but the mockups label the viewer's chip with a name.
  Keep "You"?
- **From two columns up the chips centre under the centred columns.** The mockups left-align them.
  On a phone they start at the left edge and scroll, as drawn. If five names don't fit they scroll
  from the left. Left-align at every width instead?
