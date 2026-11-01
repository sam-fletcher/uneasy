---
title: "Make Introductions: the roll and the mar fates don't reach the table without a reload"
order: 72
---

Found by the Make Introductions regression spec. When the dice land,
`applyAutoChoiceOnRoll` applies the make or mar and saves `resolution_data`
(the arrival or fate forms depend on it), but it broadcasts nothing; only the
`MakeChoice` handler emits `plan.choice_applied`. So the preparer's panel stays
on "is welcoming the new peers…" / "is resolving the marred introductions…"
until the page is reloaded. Probably affects every plan that auto-applies on
the roll, not only this one. Fix: broadcast `plan.choice_applied` from the
auto-apply path (or have the client refetch the plan on `roll.resolved`), then
un-`fixme` the last test in `frontend/tests/e2e/make_introductions.spec.ts`.

The same gap on the mar side: `introductions-mar` saves each peer's fate and
moves the Waiting On bar to whoever owes a marginalia, but sends no plan event,
so that player's "write the marginalia" form is missing until they reload (the
bar says it's their turn). The second `test.fixme` in the spec pins it.
