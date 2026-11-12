---
title: "Redesign: the Public Record pane"
order: 40
---

**Starts after** the table shell PR merges.

- **Events:**
  - each plan or scene line is a 44px tappable row with a caret;
  - on a phone the row opens Chat at that point; in columns it scrolls the chat column;
  - row numbers and empty rows can't be tapped.
- **The current row:**
  - a deep gold number and a 2px deep gold bar down its left edge, with no fill;
  - every other number is faint.
- **A plan line** shows the preparer's shape, the plan's name, and at most one status word:
  - "Resolving", in secondary text, on the current plan;
  - Make in green or Mar in red, on a resolved plan;
  - "No effect", in secondary text with no colour, on a plan that closed with no effect;
  - no word on Make War or Clandestinely Liaise, and none on future plans.
  - Careful: the server stores "make" when a war or liaison ends, so the record must not show Make for either.
- **The Shake-Up row** uses the house burst mark, not the ✶ character.
- **Laws and Rumors:**
  - plain entries separated by lines, not tinted cards;
  - bylines carry each name's shape;
  - a hidden source reads "Unknown" in faint italics;
  - an entry you haven't seen starts with the pale "new" dot, from the server's last-seen record;
  - your own entries get a bordered 44px Edit button.
- **In the Prologue,** Events shows the 13 blank rows and says when play starts.
- **Files:** `PublicRecord.svelte`, `LawsRumors.svelte`.
