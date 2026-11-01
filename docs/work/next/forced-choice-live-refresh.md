---
title: "A self-applied make doesn't reach open panels until they reload"
order: 40
---

When the dice land and a plan applies its only legal outcome by itself
(`ForcedChoicer`: Spread Propaganda's make, Make Introductions), the server saves
the choice but broadcasts nothing that makes an open client refetch the plan —
`roll.resolved` only updates the dice card. The preparer stays on the pre-roll
Buffet and everyone else on "resolving" until they reload, so the artifact step
(Spread Propaganda) isn't offered live. A make-choice and an auto-completed plan
both broadcast; the forced-choice path in `applyAutoChoiceOnRoll` should too
(`plan.choice_applied`). Pinned by the `test.fixme` in
`frontend/tests/e2e/spread_propaganda.spec.ts`; un-fixme it with the fix.
Check Make Introductions for the same gap.
