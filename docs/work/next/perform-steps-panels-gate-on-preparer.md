---
title: "Plan panels offer the preparer a picker when a perform_steps winner acts instead"
order: 83
---

When a Make Demands `perform_steps` winner drives a plan, the server refuses the
preparer's make/mar submissions and the Waiting On bar names the winner — but six
panels still compute the actor as `amChoiceActor = isPreparer || winner`, so the
preparer is shown a live picker and an "Apply choices" button that cannot work:
Propose Decree, Exchange Courtiers, Seek Answers, Spread Rumors, Spread
Propaganda and Chronicle Histories. Make Introductions already uses the shared
`resolutionActorID` (`plans/shared.ts`), which derives the actor from the plans
list so it holds on first render; the others should too.

Fix: use `resolutionActorID` in each panel, checking that each panel's pre-roll
parts (stating the rumor, casting the roll, naming peers) stay with the preparer,
since perform_steps is post-roll only.

Pinned for Seek Answers as a `test.fixme` in
`frontend/tests/e2e/make_demands.spec.ts` (the preparer should see
"bob is choosing make/mar options…", not Apply choices); un-fix it when this
lands, and add the same check to the other panels' specs.
