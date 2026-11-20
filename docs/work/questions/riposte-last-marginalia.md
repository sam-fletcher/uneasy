---
title: "On a riposte, may the preparer tear the requested peer's last marginalia?"
---

Exchange Courtiers' riposte lets the target take the peer they named, which the preparer "may first break".
Tearing a peer's last intact marginalia destroys it, so such a break would leave the target with nothing to
take.

The server refuses that break (the peer must survive to be surrendered), so a peer with one note can only be
surrendered intact. That rule was already in the code; this change made the picker and the log honest about
it (the picker used to offer the note and then fail). Was refusing it the intent, or may the preparer destroy
the peer instead?

**Recommendation: keep the refusal.** The preparer loses the peer either way, so this is not about dodging
the mar. The reason is the option's own promise: the target takes the preparer's peer, and a destroyed peer
would make that untrue. The cost is that a fragile peer can't be spited on its way out.

If the owner would rather allow it, the break destroys the peer and nothing passes to the target: the server
route, its integration test, the picker's note and the log line all change.
