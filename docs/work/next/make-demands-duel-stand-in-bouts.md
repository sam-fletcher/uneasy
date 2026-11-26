---
title: "Make Demands: a perform-steps winner is seated for a duel's bouts, but the panel gives them nothing to press"
order: 80
---

When a Make Demands winner takes "perform the steps" against a Propose Duel, the server seats them in
the preparer's seat, but the duel panel gives bout controls only to the preparer and the target. In the
browser the winner has nothing to press, and the preparer may be refused.

**The reading (moved here from a question, 2026-11-26).** The bouts come before the roll: the book
resolves the duel "with the normal dice roll procedure as though each player rolled dice they won in
the bouts" (p.60). The existing rulings already say a perform-steps winner is post-roll only and the
preparer still rolls. So the preparer plays the bouts, and the winner takes over only once the duel's
roll has resolved: on a make they choose which staked assets are taken; on a mar the opponent still
chooses, as a third party keeps their own role.

**Fix:** the server stops seating the winner for the bouts; the panel needs no change for them. Check
every route the bouts use (`handler/plan_propose_duel*.go`) and the acting set the row state names, so
Waiting On and the reminder go to the preparer during the bouts.

**Done when:** an integration test refuses the winner's bout move and accepts the preparer's, and the
winner is offered the post-roll step once the roll resolves; the e2e or a screenshot at 375px shows the
preparer's bout controls with a perform-steps demand in place.
