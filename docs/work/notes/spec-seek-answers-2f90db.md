# Seek Answers e2e regression specs

## What changed and why

Extended `frontend/tests/e2e/seek_answers.spec.ts` (phase 4 of the UX audit: one
regression spec per plan) and added `frontend/tests/e2e/helpers/seek_answers.ts`.
The existing ask → veto → re-ask → answer test is kept; its contexts now close in an
`afterEach`. New, all with one browser context per player at 375px:

- **pre-roll + Buffet** — preparer sees the narration form, the others a greyed copy and
  "alice is researching…"; a non-preparer cast is a 403, a second cast a 409; the Buffet is
  collapsed by default and a bystander can open it before any dice, including the mar tab's
  copy (the preparer breaks one of their own resources per point the roll fell short);
  the narration reaches the log.
- **mar flow** — shared Stepper: exact total enforced, 44px targets, "learn the secrets"
  capped at its one live target (reason shown), uncapped options fill the budget, + shuts at
  the budget; bystanders get a greyed copy and a wait line, never a button; after the commit
  the wait line changes and the picks persist; the self-flaw penalty is capped at the
  preparer's one resource; reveal/break/penalty steps run, "Resolved so far" names assets to
  every viewer, the plan resolves with no "Complete plan"; outcomes checked on the assets.
- **hidden information** (same test) — bob writes a secret after every page is open; alice
  learns it by reveal-secret and can read it; as carol (bystander) no response, no log entry,
  no WebSocket frame and no rendered page contains the text, while `secret_count` stays public.
- **zero-pick mar commit** — recorded as a committed choice, plan stays open with only the
  preparer waited on.
- **question and answer** — public by design: a bystander sees the pending question live, only
  the target may answer (403), the answer reaches the log.
- **forfeit hatch** — a committed `reveal_secret` pick with no secret anywhere shows "Skip — no
  valid targets" to the preparer only (a bystander's forfeit is a 403), and the skip resolves the plan.
- **veto** — bob, ranked below alice, is offered no Veto and the server answers 409.
- **four `test.fixme`s**: three new items (below) plus the perform_steps gating bug, pointed at
  `perform-steps-panels-gate-on-preparer.md`.

## How it was verified

Against my own build (`server_seek_answers`, port 8187, private database
`uneasy_claude_spec_seek_answers_2f90db`, via `E2E_BASE_URL`); never `:8090`/`uneasy_test`.

- 3 consecutive runs: 5 passed, 4 skipped (the fixmes) each. `make check-fast` passes.
- The three fixmes were run un-fixme'd and fail for the stated reasons.
- Bites, by breaking the product locally and reverting: Buffet mar copy changed (pre-roll
  spec fails); reveal-secret cap removed (mar spec fails at the Stepper); the reveal log line
  made to include the secret text (mar spec fails the hidden-information check).
- Zero-pick: removing `ChoiceSubmitted` from `ChoicesCommitted()` fails the test, because the
  plan then won't auto-resolve once the penalty is paid. (Row state alone doesn't show the marker.)
- Not run: `make test-integration` (no Go changed).

## Behaviour changes to look at before merging

None (tests only).

## Open questions for the owner

None.

## Noticed, not fixed

New items, each pinned by a `test.fixme`:
- `next/seek-answers-choice-before-cast.md` — `make-choice` before the dice are cast: a make
  commits with no roll, a mar is a 500.
- `next/seek-answers-substeps-not-broadcast.md` — break/reveal/declare steps don't refresh a
  bystander's open panel.
- `next/seek-answers-zero-pick-panel-reverts.md` — after a zero-pick mar commit the panel
  shows the picker again and never offers the penalty.
- Also: the server accepts fewer picks than the dice result (a short `make-choice` returns
  200); only the UI enforces the exact total. Not filed as an item.
- The perform_steps preparer-picker bug for Seek Answers is already pinned in
  `make_demands.spec.ts` (`perform-steps-panels-gate-on-preparer.md`); no duplicate here.
- The auto-applied-choice gap doesn't apply: Seek Answers never auto-applies a choice.

Audit fixes not pinned here: the veto-locks-the-target rule is in the original test; the
`GetSecrets`/log-completeness pass for Seek Answers' other steps (declare truth, break) is
covered only by the outcome checks above, and the Tier-1 read-only mirror of the sub-steps is
asserted for reveal and break, not for declare or ask.
