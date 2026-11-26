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
