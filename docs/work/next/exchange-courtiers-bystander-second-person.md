---
title: "Exchange Courtiers: a bystander is told \"You asked for …\""
order: 80
---

After the target proposes a fair trade, the panel's wait line for everyone who
is not the preparer reads "You asked for <peer> in exchange. Waiting for
<preparer>'s decision…". That is right for the target and wrong for a bystander,
who sees it addressed to them (the audit's Tier 6 "second-person wording shown to
non-actors"). The branch needs an `isTarget` check, with a third-person line
("<target> asked for …") for everyone else.

Pinned as a `test.fixme` in `frontend/tests/e2e/exchange_courtiers.spec.ts`;
un-fix it when this lands.
