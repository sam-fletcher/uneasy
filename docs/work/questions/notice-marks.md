---
title: "Which marks should danger and info notices wear?"
---

A notice is a mark and coloured text. `shared/Notice.svelte` gives each tone a default mark so a
caller doesn't have to pick one:

| Tone | Colour | Default mark | Where it came from |
|---|---|---|---|
| warning | orange | triangle with an exclamation point | drawn in the signed-off mockups |
| success | green | tick | drawn in the signed-off mockups (the selected tick) |
| danger | red | octagon with an exclamation point | **mine, not signed off** |
| info | blue | circled "i" | **mine, not signed off** |

The two I drew follow the house line idiom (24-unit grid, 2-unit stroke, round ends), and I picked the
octagon so it wasn't another circle (the clock and the help mark are circles) or a diamond (the first
seat's shape). A new mark needs your sign-off like any other, so this asks:

- **Are those two the right marks?** If not, `Notice.svelte` is the one file to change.
- **Does an error get a mark at all?** `ErrorText` still renders plain red text with `role="alert"`. The
  ruling says a notice is a mark and coloured text, and an error is arguably a notice. If it should
  wear the danger mark, that is a change to `ErrorText.svelte`, which belongs to the pages piece.
- **Should a notice usually borrow the mark of its noun instead?** `Notice` accepts any house mark as
  `mark` (the styleguide shows a torn sheet on a danger notice). The default marks are only for a
  notice with no noun of its own, so it may be that few notices ever use them.
