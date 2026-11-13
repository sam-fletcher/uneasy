---
title: "Step heading wording: 'the next step', and 'Your move' naming no one else"
---

The heading of the open step is built from who the game waits on and a phrase for the step
(`lib/waitingOn.ts`). Two things in it are mine, not yours:

- **A plan that is simply "being resolved" is headed "the next step"** ("Your move — the next step").
  The server knows only that the plan is resolving, not which of its own steps it is on; the plan's
  panel knows ("What have you learned?", "Result: Make"), and those lines sit directly under the
  heading. So the heading is correct but vague for those plans. The fix is to let each panel name its
  own step to the frame (a small hook), which belongs with the plan-panel pieces. Is "the next step" an
  acceptable stopgap until then, or would you rather it say something else (the plan's name, say)?
- **When you are one of several the game waits on, the heading says only "Your move — …" and names
  nobody else.** The old Waiting On bar read "Waiting On: You, otto"; that list is gone, and the only
  place the others still show is the clock on their chips on the player row. This is a design call, not a
  ruling: the mockup for the roll does the same (your move, with two other players' clocks showing), and it
  keeps the heading about what you must do. It does mean the full set of who is owed is now carried by a
  mark and no longer by words, and "Waiting on everyone but you" is not something a player can read. If you
  want "Your move, and otto's" instead, it is one line in `lib/stepHeading.ts` (and the e2e helper
  `expectWaitingOn`).

The Lobby, the Prologue, the Shake-Up and the end still have no step to hold a heading, so for now
the table page draws the same heading above them. Their pieces can move it into their own steps and
delete that line. The Prologue's own labels ("Prologue — 3 of 15 chosen") predate the new wording and
read doubled after the dash ("Your move — Prologue — 3 of 15 chosen"); they should be reworded as
lower-case phrases like the main event's.
