---
title: "Is the plan grid right on the new scales? (it was never mocked up)"
---

The Play mockups cover the roll and the waiting view. The grid you pick a plan from has no mockup, so
it was rebuilt from the rulings as written. Three of its choices go past them; each is one rule in
`plans/planPanel.css`, so changing one is a small edit.

- **It is still three columns, with the text at the smallest size that is allowed.** Nothing goes
  below 12px, and the old grid used 10 to 11px. At 375px the cards are about 108px wide, so each
  description wraps to four or five lines and the grid is taller than before (about 600px against
  about 400px; the page scrolls). The alternatives are one column of 12 rows (taller still, and
  easier to read), or three short sections with a row per plan. Which do you want?
- **A picked card has the bright gold border and a tick, like every other card.** You said the tick
  may become clutter where a card carries other things; this card carries a title, a description, a
  delay and up to five shapes, so it is the case to look at. Without the tick the border still shows
  the pick, but the colour is then the only difference (width apart).
- **A watcher sees the focus player's highlighted card in neutral text colour, not bright gold.**
  Bright gold means "yours", so showing someone else's pick in it would say you had picked it. It is
  a 2px border in the secondary text colour, with a grey tick. If you would rather it were gold for
  everyone, delete the `.mirrored` rules.

The "Finale" tag on a plan that would spend the one Explosive Finale plan is a warning mark and the
word "Finale", in orange, so the colour is not the only signal. It replaces the orange card fill.
