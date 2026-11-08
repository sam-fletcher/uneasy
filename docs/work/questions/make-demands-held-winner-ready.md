---
title: "Should a Make Demands winner who still owes a decision be refused a ready?"
---

A Make Demands winner who has taken control_leverage or keep_or_change_target
holds the target's roll until they settle it. The server seeds them unready and
never auto-readies them, but an explicit ready from them is accepted. In an
e2e run with the winner's decision request held back, their ready alone
resolved the roll, and the decision never reached the server. In the roll
panel (`DiceRollPanel.svelte`) the Ready button is not gated on that decision.

Should the server refuse a held winner's ready until they have decided? Or is
ready meant to be a separate act that can come first? The answer decides
whether the roll-hold rule can be bypassed, and whether the e2e "form is gone"
specs in `frontend/tests/e2e/make_demands.spec.ts` should assert the decision
landed before the readies.
