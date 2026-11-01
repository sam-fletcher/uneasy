# spec-make-demands: e2e regression specs for Make Demands

## What changed and why
`frontend/tests/e2e/make_demands.spec.ts` (+ `helpers/make_demands.ts`) pins what
the 12-plan UX audit and the Make Demands audit fixed. 10 tests, 9 run, 1 `fixme`:
- **Attaching** (UI, bob): only another player's pending plan is offered; Prepare
  stays disabled until a target and a motivation exist; the demand lands on the
  target's row ahead of it.
- **The demand's own roll:** the actor sees Skip vote / Call difficulty vote,
  everyone else "Waiting for bob to decide…"; skipping clears it for all.
- **The draft** (UI, both pickers, a bystander): duds are `aria-disabled` with their
  reason while a live option remains; tapping one selects nothing and doesn't clear a
  live pick; once only duds are left none is blocked and the picker says so (#24),
  and the forced last pick is taken. Bystander and non-picker see no cards.
- **Control leverage:** "Leverage none — let the roll fail" is offered at zero and
  submits; the preparer's own leverage 403s with the "taken over" message; a third
  player can't submit it; the log line posts. Against Propose Duel (no preparer
  roll): greyed in the draft, the forced last pick still works, the banner says the
  option came to nothing, the winner sees an explanation not a picker, the server
  answers 409, and Waiting On doesn't name the winner.
- **Keep or change target** (Spread Rumors): the table is told the roll waits on the
  winner, only the winner gets the form, re-aiming (player chip + asset card) moves
  the plan and shows live to the others, "keep" settles without moving it, both log,
  a non-winner gets 403.
- **Perform steps** (Seek Answers): preparer still casts and rolls; the winner is
  told they act in her place, the others who; the preparer's own make-choice is a 403;
  the winner picks through the page and writes the truths; the plan resolves.
- **A marred demand:** only the target's preparer is asked for the counter-demand;
  placing it (defer) ends the plan with no "Complete plan" click.
- Not pinned, per the brief: whether the server should refuse an inert pick, whether
  Duel/Liaise should wait on the retarget winner, the draft-pick log severity.

## How it was verified
Against this worktree's own build on :8173 with a private DB (dropped afterwards):
3 runs in a row, 9 passed + 1 skipped each; the fixme was un-skipped once and fails
for the stated reason (no wait line for the preparer). Bite checks, each reverted:
leverage button disabled at zero; retarget never "finalized"; draft greying
without the live-remains condition (and again with no greying at all); Duel counted
as having a preparer roll; `leverageBlockedByDemandWinner` a no-op; `actsForPreparer`
ignoring the winner — the draft, leverage, dud, keep-or-change and perform-steps tests
failed each time. Not separately bitten: the 409 body text for leverage against a
roll-less plan (it failed earlier in that test in the same run), and the attach and
skip-vote tests. `make check-fast` — see PR. Not run: Go integration suite (no Go
changes).

## Behaviour changes to look at before merging
None (tests only).

## Open questions for the owner
None new. Still open: `make-demands-server-refuse-inert`,
`make-demands-retarget-wait-duel-liaise`, `minor-severity-logs`. One data point for
the second: the retarget gate (`pendingDemandRetargetChooser`) does hold the roll
for Seek Answers and other plans whose picker greys retarget out as inert, so
"inert" and "gating" already disagree there too.

## Noticed, not fixed
- New: `seek-answers-preparer-offered-picker-under-perform-steps` (pinned by the
  fixme) and `demand-overlay-forms-shown-to-preparer-and-after-roll`.
- Seek Answers' make-choice answers a refused preparer with "only the plan's
  preparer can do this", which reads wrongly when she is the preparer.
- The pre-roll "hold" is weaker than the copy suggests: a winner who hasn't finalized
  can still be readied, and a roll with everyone ready resolves. The gate only stops
  auto-ready and the no-leverageable-assets short-circuit, so the specs pin the
  winner's controls and done-state, not a held roll.
