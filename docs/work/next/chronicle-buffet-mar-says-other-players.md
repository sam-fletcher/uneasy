---
title: "Chronicle Histories' Buffet says only the other players choose on a mar"
order: 42
---

The rules have *all* players choose one option on a mar, and the resolving
panel's own note says so ("Each player (including the preparer)…", and the
preparer does get a picker and counts towards "(n/3)"). But the "What can
happen?" reference's Mar tab says "Every OTHER player chooses one of these
instead:" (`ChronicleHistoriesPanel.svelte`, `buffetTabs`). A player reading the
reference before the roll — which is when they decide whether to help or
interfere — is told the preparer is exempt.

Fix: reword the tab ("Every player, the preparer included, chooses one…").
Pinned by a `test.fixme` in `frontend/tests/e2e/chronicle_histories.spec.ts`.
