---
title: "Should the profile's table list widen on a desktop, or stay one phone-width column?"
---

The old profile page widened to two phone-widths at 790px and up, so its table cards could sit
two across. The rows that replace the cards are a list read top to bottom under three headings, so
this piece keeps them in the one 440px column at every window width, centred, with the form
sections below at the same width. That matches the column cap in the layout widths ("no other
content column, profile included, is ever wider").

On a desktop that leaves a narrow column in the middle of an empty dark page. The alternatives:

- **Keep one column (what's built).** One reading order, headings stay in line with their rows, and
  it needs no new layout rule.
- **Two columns of rows from 790px up.** Shorter page, but each group's heading would have to span
  both columns or repeat, and "Waiting on you" would no longer read as the top of the list.
- **Tables in a wider single column** (say 600px) for desktop only. Gives long names room, but it
  is a column width the layout system doesn't have.

The mockup shows a phone only, so nothing signed off covers this. See the screenshots in the PR.
