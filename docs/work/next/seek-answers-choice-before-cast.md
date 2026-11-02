---
title: "Seek Answers accepts make-choice before the dice are cast"
order: 85
---

`POST /api/plans/{id}/make-choice` on a Seek Answers plan that is still on its
pre-roll step (no dice cast) is not refused. A `make` is committed with no roll at
all and its picks go on to play out; a `mar` fails with a 500 ("could not load roll
for mar penalty") instead of a 4xx. Spread Propaganda refuses both with a 409. The
panel never offers this, so it takes a hand-built or stale client.

Fix: refuse `make-choice` with a 409 until the plan's pre-roll is done (a roll
exists), for every plan with a pre-roll step if they share the gap.

Pinned as a `test.fixme` in `frontend/tests/e2e/seek_answers.spec.ts`; un-fix it with the fix.
