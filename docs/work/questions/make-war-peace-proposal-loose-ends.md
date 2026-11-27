---
title: "What should happen to a peace proposal that nobody answers, or that its proposer pays past?"
---

Two linked gaps in Make War:

- A payer with their own proposal open can still pay a battle cost through the API.
  The UI hides the form and the server doesn't refuse. Should it 409, or does paying
  withdraw the proposal?
- A proposal can't be withdrawn and never times out, so an unanswered one blocks any
  new proposal.

(A third gap, accepting votes going unlogged, moved to
`next/war-peace-accept-votes-logged.md` on 2026-11-26: it is a bug, not a ruling.)

Two more for the rules round, from the peace-terms ruling of 2026-11-26. Agreed terms are now kept
(`next/war-peace-treaties.md`), but they stay words:

- The book lets terms be anything the active participants agree, including exchanging assets,
  patching up a broken one, or resolving a plan nobody prepared. Should the app carry any of that
  out, or leave it all to the table by hand? Handing over an asset can't be done by hand at all
  today.
- The book reminds non-participants who disagree with the terms that they may pay the cost of battle
  at once to join the negotiations. Can a player do that while a proposal is open, and does joining
  reopen the vote?
