---
title: "Make Demands preparation mirror may list the watcher's plans, not the preparer's"
order: 82
---

The Make Demands preparation form builds its target list from the viewer. In
`plans/MakeDemandsPanel.svelte` (`targetablePlans`, around line 70) the filter is
`p.preparer_id !== currentPlayerID`, so it drops the viewer's own plans, not the
preparer's. That list feeds the target `<select>` in the prep form. A watcher's read-only
mirror may therefore list the preparer's plans and leave out the watcher's own. If the
preparer's draft targets a plan the watcher prepared, the select may show its placeholder
instead. Not reproduced yet.

The fix follows #97 and the Duel and War prep forms: when the form is read-only, exclude
`ctx.focusPlayerID` (the preparer) instead of the viewer, and leave the preparer's own
form on `currentPlayerID`. The server-side filter that the file's comment says must stay in
lockstep (`mdHandler.CheckPrepEligibility` / `ValidatePreparation`, `handler/plan_make_demands.go`)
is not part of the mirror and should not change.

Reproduce at 375px as a watcher first, before changing code. The watcher's mirror of the
preparer's Make Demands should list the watcher's own plan as a target, and leave out the
preparer's plans, since the preparer cannot demand their own plan. Check it both with and
without the preparer's draft pointing at the watcher's plan.
