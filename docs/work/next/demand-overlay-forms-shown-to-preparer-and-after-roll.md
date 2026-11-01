---
title: "Make Demands overlay: the retarget and leverage forms show when they can't do anything"
order: 84
---

`TargetPlanDemandOverlay.svelte` shows the keep-or-change-target form to anyone who
won that option, and the leverage form to anyone who won leverage, without
asking two questions:

- **Is the winner the plan's own preparer?** Then there is no hand-off and the
  server never holds the roll for them, but alice still reads "…so the roll waits
  on you" with Re-aim / Keep buttons on her own plan.
- **Is the roll still open?** The leverage form is gated on the plan being
  `resolving`, not on the roll, so after the dice have landed the winner (or the
  preparer, if she won it) still sees "Pick any number of assets" and "Leverage
  none — let the roll fail"; the server then answers 409.

Both forms should be hidden when the server would not wait on them (the
`pendingDemandChooser` conditions: winner is not the preparer, roll open, not yet
finalized — except that the retarget note says the winner may still adjust it
until the plan resolves, so that part wants an owner call on whether the form
stays after the roll).

Not pinned by a spec yet.
