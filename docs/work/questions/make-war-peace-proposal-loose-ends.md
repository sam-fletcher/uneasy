---
title: "What should happen to a peace proposal that nobody answers, or that its proposer pays past?"
---

Three linked gaps in Make War:

- A payer with their own proposal open can still pay a battle cost through the API.
  The UI hides the form and the server doesn't refuse. Should it 409, or does paying
  withdraw the proposal?
- A proposal can't be withdrawn and never times out, so an unanswered one blocks any
  new proposal.
- Accepting votes aren't logged, though rejections are.
