---
title: "Stay out of a war: should the choice stay changeable on screen?"
---

The war's panel shows a sheet's copy for "Stay out": *"Staying out tells the table you won't join. You
can still join later, while the war lasts."* That is true of the game (the server lets anyone join an
active war), but the old panel made it true only after a page reload: once you pressed "Stay out of it",
the three buttons collapsed into a note and there was no way back on screen.

I made the copy true instead of leaving it a half-promise. After you confirm "Stay out" the chips stay,
with "Stay out" still picked, and the cost and main button give way to the line *"You're staying out of
this war. You can still join while it lasts."* Picking a side again shows that side's cost and its main
button, so changing your mind is the same two steps as joining. Nothing changes on the server, and
"Stay out" is still only a note to yourself that lasts for the session.

Is that right? The other reading is that "Stay out" should be final on screen, as before, in which case
the sheet's copy should stop promising a later join. Going back to the old behaviour is deleting `picked`'s
`stayedOut` fallback and the `settledOut` branch in `plans/war/WarStatus.svelte`, and its test and e2e
step ("is two taps, becomes a note, and can still be changed to a join").
