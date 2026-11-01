---
title: "Make Introductions: step routes don't tell the table what changed"
order: 73
---

`introductionsArrivalHandler`, `introductionsMarHandler` and
`introductionsMarginaliaHandler` (`handler/plan_make_introductions.go`)
broadcast only the row state, never `plan.choice_applied`. A non-actor's
remaining-count, recap of fates ("X — Another retinue (awaiting marginalia)")
and wait line therefore go stale mid-flow until they reload. Broadcast
`plan.choice_applied` from each, then un-`fixme` the last test in
`frontend/tests/e2e/make_introductions.spec.ts`. The roll's own auto-apply gap
is tracked in `auto-applied-choice-not-broadcast.md`.
