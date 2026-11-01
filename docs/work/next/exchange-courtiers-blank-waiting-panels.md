---
title: "Exchange Courtiers: seats that aren't acting see no wait line"
order: 81
---

Three seats get an empty panel where they should be told who the plan is
waiting on: a bystander before the target has made the offer, the target while
the preparer picks the make option, and the preparer while the target picks the
mar response. In each, `ExchangeCourtiersPanel.svelte` has no branch for that
seat, so the card shows only the preparation quote. One fix: a third-person wait
line for each ("Waiting for <actor> to …").

Pinned as two `test.fixme`s in `frontend/tests/e2e/exchange_courtiers.spec.ts`
(by shape: a wait line naming the actor); un-fix them when this lands.
