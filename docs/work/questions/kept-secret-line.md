---
title: "Are the watcher's two Spread Rumors lines ('Kept secret…', 'is preparing the rumor') the right words?"
---

A watcher no longer sees the rumor's text as it is typed (ruled: it is the least important part of a
read-only screen and must never cost anything else), so their read-only copy of the form has nothing
to show where the text box was. An empty box would read as a rumor not yet written, so it says, in
plain secondary text where the box would be:

- with "Keep it secret for now" ticked: "Kept secret for now. Only {preparer} can read it."
- otherwise: "{preparer} is preparing the rumor." (It said "is writing the rumor" until an open rumor
  could be left blank at preparation, when "writing" stopped being always true.)

The "Hidden under: …" line below them is unchanged.

The wording and the shape were my call, not ruled on. Two things to confirm or change:

- **The words.** Shorter ("Kept secret for now.", "Preparing the rumor…")? Or should the second say that
  the text will appear when the plan is prepared? The first stays to what a watcher can't read and who
  can; the "Hidden under:" line already names the asset.
- **A plain line, or a `Notice`?** Both are plain secondary text (`.form-hint`), because the info
  `Notice`'s circled "i" isn't signed off yet (`notice-marks.md`). If you would rather they wore a
  house mark (a secret's mark, say), it is one line in `SpreadRumorsPanel.svelte`.
