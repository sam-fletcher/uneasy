---
title: "Make War: a join during the delay vote isn't pushed to the Waiting On bar"
order: 71
---

Found by the Make War regression spec (`make_war.spec.ts`, the `test.fixme` at
its end). While a war's delay vote is open, a third player who joins a side
becomes a vote participant: the sides list updates for everyone, and the joiner
is given the Submit control. But the other players' Waiting On bar keeps naming
only the original participants until they reload (live: "You, bob"; after a
reload: "You, bob, carol"), so the table is told it is not waiting on someone it
is.

Likely cause: `join-war` broadcasts a war event (the panel refetches) but not
the row-state change the bar derives from — the same class as
`make-introductions-step-routes-not-broadcast.md`. Fix, then flip the
`test.fixme` to `test`.

A join *after* the reveal closes (the late joiner owing an entry payment, the
pending-participant branch) probably has the same gap and is not pinned yet.
