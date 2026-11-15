---
title: "Spread Rumors: a watcher sees the preparer's draft of a secret rumor"
order: 62
---

Ticking "Keep it secret for now" tells the preparer "Others won't see the text until you spread it", and the
textarea's placeholder says "only you will see it until you spread it". The submitted rumor does keep to that.
The preparation draft does not: `SpreadRumorsPanel` sends `notes: prepNotes` in `emitPrepDraft` whether or not
`keep_secret` is ticked, so a watcher's read-only mirror fills in the preparer's typed text as they type it.
Found by taking a bystander's screenshot while the preparer had the box ticked; it behaves the same on `main`.

The fix is probably to leave `notes` out of the draft while `keep_secret` is true (the flag and the chosen
asset can stay: that a rumor is secret is public, its text is not), and to show the watcher a "kept secret"
line in place of the text. Check the other plans' prep mirrors for the same shape (Clandestinely Liaise keeps
a secret asset, Seek Answers and Chronicle Histories write text), and whether the draft is sent to the server
or only broadcast.
