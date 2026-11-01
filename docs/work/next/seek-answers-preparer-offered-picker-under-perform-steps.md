---
title: "Seek Answers: the preparer is still offered the make/mar picker when a perform_steps winner acts"
order: 83
---

With a Make Demands `perform_steps` winner on a Seek Answers plan, the preparer's
panel still shows a live option picker with an "Apply choices" button
(`amChoiceActor = isPreparer || winner` in `SeekAnswersPanel.svelte`), while the
server refuses her submission with a 403 and the Waiting On bar names the winner.
Everyone else sees "<winner> is choosing make/mar options…"; she should see that
wait line too, not a button that cannot work. Make Introductions already does
this right.

Check the other plans' panels for the same `isPreparer ||` shape.

Pinned as a `test.fixme` in `frontend/tests/e2e/make_demands.spec.ts`; un-fix it
when this lands.
