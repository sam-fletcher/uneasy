---
title: "Preparation mirrors show the watcher's players and peers, not the preparer's"
order: 78
---

While the focus player prepares an Exchange Courtiers, everyone else sees the form mirrored from the draft. The
target-player chips are built from the **watcher's** other players (`otherPlayers = playersExcept(players,
currentPlayerID)` in `plans/ExchangeCourtiersPanel.svelte`), so when the watcher is the target (bob, watching alice
pick bob) the chips are alice and carol with nothing selected. A watcher should see what the preparer sees: the
chips for everyone but the preparer, with the pick marked. `PlanContext` carries only `isFocusPlayer`, not whose
turn it is, so the fix starts by giving it the focus player's id. Clandestinely Liaise's preparation has the same
shape (`otherPlayers`, and `myPeers` for "Your meeting peer", which shows the watcher's own peers under the
preparer's label), so one change to the context and the two panels should do it. Seen in the before and after
sheets of the plan-panels-B PR (preparation, the watcher's column); not caused by it.
