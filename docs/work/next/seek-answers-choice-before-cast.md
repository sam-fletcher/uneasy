---
title: "Seek Answers and Chronicle Histories accept make-choice before the dice are cast"
order: 87
---

`POST /api/plans/{id}/make-choice` on a Seek Answers plan still on its pre-roll step
(no dice cast) is not refused: a `make` is committed with no roll at all and its picks
play out; a `mar` fails with a 500 ("could not load roll for mar penalty") instead of a
4xx. Chronicle Histories has the same pre-roll step and the same gap. Spread Propaganda
refuses both with a 409. The panels never offer this, so it takes a hand-built or stale
client.

Don't fix it as a blanket "a roll must exist" check in the generic make-choice route:
Make Introductions, Clandestinely Liaise, Host Festivity and Make War legitimately
commit a choice with no roll. Give the plans with a pre-roll step a `CheckChoice` hook
(or a pre-roll-done marker the route reads) and have Seek Answers and Chronicle
Histories refuse with a 409 until the pre-roll is done.

Pinned for Seek Answers as a `test.fixme` in `frontend/tests/e2e/seek_answers.spec.ts`;
add the same check to Chronicle Histories' spec when it lands.
