---
title: "Where does a locked option card say why: under its sentence, or at the edge of the row?"
---

The shared option card (`shared/OptionCard.svelte`) says why a card is locked in words, in a line under the
card's sentence. Make Demands' draft already did that. The make/mar picker (`MakeMarPicker`, e.g. "needs −2") and
the festivity's mar picker ("You have no peers") put the reason at the right-hand edge of the row, in 12px faint
text, so a long reason was squeezed into a narrow column beside the sentence. The card now has one placement, and
it is under the sentence, since the other squeezed a long reason into a narrow column beside it.

Is under the sentence right for a short reason like "needs −2" too, or should a short reason sit at the edge of the
row and a long one under the sentence? Both are the same prop, so the card would pick by length.
