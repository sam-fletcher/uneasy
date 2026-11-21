---
title: "Host Festivity: End the event refuses a roll in progress with a different message"
order: 79
---

Low priority. End can't get through a guest's roll anyway, so nothing a player can tap goes wrong.

`hfEndEventHandler` (`handler/plan_host_festivity.go`) doesn't call `hfBlockOnActiveRoll`, so during a roll
the reply is the end-of-event refusal, "the festivity is still in full swing", not the roll lock's "a dice roll
is in progress". The refusal itself is right: `EventEndable` needs `AllGuestsResolved` (`game/festivity.go`),
which is false while a roller has no outcome. The client disables End mid-roll too (`festivityEndable` mirrors
the rule), and the e2e spec asserts it.

If it is wanted: route End through `hfBlockOnActiveRoll` like the other host actions, so the message matches,
and add a server test that End gets the lock's 409 during another guest's roll. It is a Go change, so it sits
outside the pending-mar piece.
