---
title: "Exchange Courtiers: Riposte is described to the target in the preparer's voice"
order: 82
---

The mar picker is shown to the target, but Riposte reads "You take the requested
peer — you may break it first" (`MAR_OPTIONS` in `plans/shared.ts`). Breaking the
peer first is the preparer's move, so "you may break it" is wrong for the
person choosing. Reword for the chooser, e.g. "They may break it first, then you
take it".

Pinned as a `test.fixme` in `frontend/tests/e2e/exchange_courtiers.spec.ts`;
un-fix it when this lands.
