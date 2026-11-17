---
title: "Should the marginalia boxes inside an asset card grow from 20px to the house 24px?"
---

Every checkbox in the app is now the asset card's drawn box, at 24px, from one stylesheet
(`shared/checkbox.css`). One exception is left as it was: when a card is open to pick a line of
marginalia (the "Marginalium to tear" picker on Make War's cost of battle, a break in Exchange
Courtiers), each live line has a box of 20px with a 14px tick, set in the card's own CSS.

Those rows are already 44px tall and the whole row is the tap target, so 24px would fit without
moving anything else. Should those boxes be 24px too, so there is exactly one size, or does the
smaller box on a line of text read better inside the card?
