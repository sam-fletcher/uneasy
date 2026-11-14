---
title: "Step heading wording: 'Your move' naming no one else"
---

The heading of the open step is built from who the game waits on and a phrase for the step
(`lib/waitingOn.ts`). One thing in it is mine, not yours:

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
