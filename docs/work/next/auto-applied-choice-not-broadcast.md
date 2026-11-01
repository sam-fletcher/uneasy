---
title: "An auto-applied make/mar isn't pushed to open panels until they reload"
order: 40
---

When the dice land and a plan applies its outcome by itself
(`applyAutoChoiceOnRoll`, called from `finalizeRoll` in `rolls_dice.go`), the
server saves `resolution_data` but broadcasts only `roll.resolved` and the
row state. The client's `RollResolved` handler (`ws-handlers.ts`) only updates
`activeRoll`, and only the `MakeChoice` route sends `plan.choice_applied`. So
the preparer stays on the pre-choice view (Spread Propaganda: the Buffet, not
the artifact step) and everyone else on "resolving" until they reload.

Likely affects every plan that auto-applies — the `ForcedChoicer` plans
(Spread Propaganda's make, Make Introductions) and the `AutoApplyChoiceOnRoll`
ones (Chronicle Histories, Make Demands). Worth checking each.

Two ways to fix it:
- broadcast `plan.choice_applied` from the auto-apply path, or
- refetch the roll's plan on `roll.resolved`.

Pinned by three `test.fixme`s, to un-fixme with the fix: the live artifact step
in `frontend/tests/e2e/spread_propaganda.spec.ts`, and Make Introductions' make
arrival form and mar marginalia-author form in `make_introductions.spec.ts`.
