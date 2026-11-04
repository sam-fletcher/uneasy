---
title: "Make Demands: build the stand-in and keep-or-change rulings"
order: 20
---

The owner ruled on these on 2026-11-03. "Make Demands winner" means whoever won the
option named.

**Stand-in.** The Make Demands winner of the perform option takes every choice and
step the preparer would make after the dice. Outcomes still land on the preparer,
facts about the plan stay with the preparer, and third parties keep their own roles.

- Exchange Courtiers riposte: that winner decides the break. The route is
  preparer-only today.
- Spread Propaganda "break yourself": they pick the preparer's asset and learn its
  secrets. Already built, but not pinned by a test; add one.
- Spread Rumors hide-source: the secret goes on one of the preparer's assets, and
  they pick which. Today it lands on their own asset.
- Chronicle Histories "Wrap the scene": that winner presses it. Today it is
  preparer-only.
- Seek Answers veto: no change, it still ranks against the preparer.

**Keep or change target.**

- Live only on Exchange Courtiers, Spread Rumors, Propose Duel and Clandestinely
  Liaise. On every other plan it is a plain dud: no roll-hold, and no retarget form.
  `pendingDemandRetargetChooser` follows the wrong list today, so a roll can wait on
  a click that decides nothing. The fix is to share one list with `RETARGETABLE`.
- Propose Duel and Clandestinely Liaise wait for that winner to settle the aim before
  their first step, then the form and the route close. Likely bug to prove first:
  `/demand-retarget` only closes when the roll lands, and a duel has no roll until
  the bouts end, so the opponent can apparently be changed mid-duel. Found by reading
  the code, not reproduced.
- A preparer who wins it re-aims freely with no hold. Doing nothing means keep. This
  is the behaviour today; pin it.

**Smaller items.**

- The draft picker stays as built. Fix the comment on `demandOptionInertReason`,
  which calls it a hint although the card is blocked while a live option remains.
- Raise the Make Demands draft-pick log line from Minor to Default.
