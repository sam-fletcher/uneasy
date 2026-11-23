---
title: "Propose Decree: should a watcher see every pending joiner's picker, and where should the signatory's own step sit?"
---

A player ranked below the preparer decides for themself whether to leverage one asset to join the council. A
player who owes no decision (the signatory, a seat already filled, one who has decided) now sees each pending
player's picker, greyed out: that player's own un-leveraged assets, under a line saying whose they are. It is
the same recipe every other picker uses, so the table can see what is being chosen from. The mirrors show
before the debate opens as well as during it, because joining is open from the start of the council.

They stack, one per pending player, so the length grows with players times assets: in a dev-seeded
three-player game (four assets each) the signatory's page is about 400px longer at 375px, and a five-player
table with full retinues would be several screens.

**Who pays for the length.** Not the player who must act. A joiner has the live picker and is shown no one
else's, and "End the debate" is disabled for as long as any mirror is showing (it is disabled while anyone is
undecided, and the mirrors are exactly the undecided players), so the signatory never scrolls past a mirror to
reach a live button. The preparer's own text box, before the debate opens, comes above the mirrors. What the
signatory does lose is that their "Waiting on tobin, edda…" line and the disabled button sit below the mirrors,
and snap up into view when the last joiner decides. Everyone else (a player who has decided, one already
seated) has no step of their own, and the mirrors are what they came to read.

Is a full picker per pending player right, or should a watcher get a line only ("tobin and edda are deciding
whether to join")? The line already exists for the signatory alone, in the "Waiting on" sentence.

**Recommendation:** keep the mirrors; they are the point of the mirror work. If the length bothers anyone, put the
signatory's own step (the waiting line and the button) above the mirrors. That settles the signatory's case: the
step holds its place as the mirrors drop away, and it costs everyone else nothing. It is a layout change and is
not made in this PR. The cheaper alternative, if playtesters still scroll past the mirrors, is to mirror the
first pending player only and say how many more are deciding.
