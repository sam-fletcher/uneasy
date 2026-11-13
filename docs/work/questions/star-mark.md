---
title: "Is the main character's star drawn right?"
---

The main character has always been marked with a star, but it was drawn three ways: a typed `★` on
asset cards (a different font on every platform), an inline SVG on the retinue's tiles, and the same
SVG as an outline for the "make main character" control. This PR draws it once, as
`shared/StarMark.svelte`, so it is a house mark like the others. A new mark needs your sign-off, so
this asks.

The drawing is the star the retinue already used (a five-point star on the 24-unit grid), filled for
the main character and stroked (2 units, round ends) for the control that makes one:

```
M12 17.75l-6.172 3.245l1.179 -6.873l-5 -4.867l6.9 -1l3.086 -6.253l3.086 6.253l6.9 1l-5 4.867l1.179 6.873z
```

You can see it, filled and outlined, on `/dev/styleguide` under Marks, and on a retinue's main
character.

- **Is this the right star?** If you want a different drawing, `StarMark.svelte` is the one file to
  change: every use goes through it.
- **Should the card also say the words "Main character"?** The star is the only sign on an asset card
  (the retinue tile has an accessible name but no visible word). Elsewhere a mark is paired with a word
  so the meaning doesn't rest on the shape; I left this as it was.

The tick on a selected card is not a new drawing: it is the tick that `Notice`'s success tone already
drew. All four of `Notice`'s tone marks now live in `shared/ToneMark.svelte`, and the card takes the
tick from there, so the notice and the card share one drawing.
