---
title: "Duel and war preparation mirrors may show the watcher's players, not the preparer's"
order: 81
---

The Propose Duel and Make War preparation forms build their read-only mirror from the viewer, as Exchange
Courtiers and Clandestinely Liaise did: `otherPlayers = playersExcept(players, currentPlayerID)` in
`plans/duel/PrepForm.svelte` and `plans/war/PrepForm.svelte`. A watcher may see the wrong player chips, with the
preparer's pick missing. `PlanContext` now carries `focusPlayerID` (added with the Exchange Courtiers and Liaise fix), so the fix is to
build the mirror from it, as the Exchange Courtiers and Liaise panels now do. Not reproduced in a browser yet:
check each watcher's view at 375px before changing anything.
