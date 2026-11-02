---
title: "Make Demands against a Clandestinely Liaise never gets its roll"
order: 84
---

A demand's difficulty is its target's plus a rank gap when the target's
preparer outranks the demander. A liaison has no roll, so its own difficulty is
0, and in the game the e2e spec builds the demander has climbed to the top of
the power track by the time the demand's row arrives (preparing it engrails
them), so the gap is 0 too. The demand then tries to insert a roll with
difficulty 0, which the `dice_rolls` check (1 to 6) rejects: the server logs
"auto-kickoff failed", the demand sits as `resolving` with no roll and no one
can move it. Reproduced three times; the same plan against Seek Answers rolls
fine because that target's difficulty is already 1 or more.

So the audit's perform_steps fix for the Liaise panel is covered server-side
(Go integration tests seed a made demand directly) but cannot be reached from
play. Fix options for the owner: clamp the demand's difficulty to at least 1, or
stop offering plans without a roll as demand targets.

Pinned as a `test.fixme` in `frontend/tests/e2e/clandestinely_liaise.spec.ts`
("perform_steps: the demand winner acts for the preparer"). With difficulty
clamped locally the rest of that test passes; un-fix it when this lands.
