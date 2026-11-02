---
title: "Seek Answers' break, reveal and declare steps aren't pushed to the other players' panels"
order: 86
---

`break-resource`, `reveal-secret` and `declare-truth` (`plan_seek_answers_routes.go`)
save their progress in `resolution_data` and post to the log, but broadcast nothing
that makes an open panel refetch the plan (`reveal-secret` sends only the secret-grant
event). Ask, veto, answer and the forfeit hatch do send `plan.choice_applied`. So a
bystander keeps reading "Break a resource (1 remaining)" with an empty "Resolved so far"
until they reload. (The log line is live; these routes send no row-state broadcast either, so the Waiting On bar is only as fresh as the next event.)

Fix: broadcast `plan.choice_applied` from those three routes (as the question routes do).

Pinned as a `test.fixme` in `frontend/tests/e2e/seek_answers.spec.ts`; un-fix it with the fix.
