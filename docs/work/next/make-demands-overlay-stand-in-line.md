---
title: "Make Demands: the target plan's banner says a stand-in makes its \"make/mar choices\", which is wrong on a duel"
order: 80
---

When a Make Demands winner takes "perform the steps", the banner over the target plan
(`demand/TargetPlanDemandOverlay.svelte`, around line 285) says the winner "will submit this plan's
make/mar choices in alice's place", for every target plan. On a Propose Duel that's wrong twice:
the preparer still plays the bouts, and on a mar the opponent chooses, not the stand-in. Since #136
the server matches the ruling. Only this line doesn't.

The option picker already has the right sentence for each plan: `perform_steps` in
`plans/buffets.ts`, which names the duel, liaison and festivity cases. Found while reviewing #136.

**Fix:** the banner's perform-steps line says what that sentence says, in the second person for the
winner and the third for everyone else, so the two can't drift. For a duel: the winner chooses which
staked assets a make takes, once the duel's roll has landed. Keep `PlayerName` for each name.

**Done when:** a server-render test reads the duel line for the winner and for a watcher, and the
generic line for one other plan type; a 375px screenshot of the banner on a duel with a perform-steps
demand in place.
