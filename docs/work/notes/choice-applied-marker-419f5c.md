# choice-applied-marker — tracker items 4 and 8

## What changed and why

**Item 4 (choice-applied marker).** The marker already existed in part:
`ResolutionData.ChoiceSubmitted` / `ChoiceAutoApplied`, read through
`ChoicesCommitted()`, set by the generic make-choice route. The Waiting On
bar's actor sets (`ComputeRowState`, the generic `plan_resolving` case,
`pendingPerformStepsChooser`, `performStepsStillOwed`) already read it. What
was left:
- Spread Rumors' `ResolvingWaitees` and its counter-rumor guard still read
  `len(MakeMarChoices) > 0`; both now use `ChoicesCommitted()`, so a mar
  committed with no picks hands the wait to the preparer.
- The Propose Duel claim, which writes `MakeMarChoices` directly, now sets
  `ChoiceSubmitted`. (Spread Rumors' take-consent commit needs no write: a take
  always records `take_asset`.)
- Frontend: `choice_submitted` / `choice_auto_applied` added to
  `ResolutionData` in `types.ts`, a `choicesCommitted()` helper in
  `plans/shared.ts`, used by the three panels that tested
  `make_mar_choices.length` for "pre-roll done".
- `waitingOn.ts` needed no change: it renders the server's
  `acting_player_ids` verbatim, so it picks the fix up through the row state.
- Old rows: the markers are new optional booleans, absent = false, and
  `ChoicesCommitted()` still counts a non-empty `make_mar_choices`. A
  zero-pick commit stored before the marker existed cannot be recovered.

**Item 8.** `nextFocusPlayer` now returns the players it passed over (no
peers) and `EmitFocusPassed` names them: "A passes the focus to C, skipping B
(no peers left)." The post still anchors with `logRow(game)`. Focus only
passes in the main event, so the anchor is a real row (1–13), not row 14. It carries `skipped_player_ids` in its data.

## How it was verified

- New tests, each shown failing with its fix reverted:
  `TestSpreadRumors_Mar_ZeroPickCommitHandsTheWaitToThePreparer`,
  `TestSpreadRumors_Mar_CounterRumorClosedByAZeroPickCommit`, the marker assertion
  in `TestProposeDuelHTTP_Make_TakesStakeLeveragesAndLogs`, and
  `TestFocusPassLog_PassFocus_NamesSkippedPlayers` (one and two skipped names).
  The focus-pass tests moved to `focus_pass_log_integration_test.go`.
- `TestChoicesCommitted_StoredShapes` (Go) and the `choicesCommitted` Vitest
  cases pin the stored shapes, including the old one; they guard decoding
  rather than fail on a revert.
- `make check-fast` passes. `make test-integration` on a private database: handler, game, hub, middleware, cmd/server pass; `db` `TestGenerateJoinCode_Uniqueness` failed once and passed 3/3 on rerun (looks like a random join-code collision, unrelated).
- **Make Introductions** (the other zero-pick commit) is a steady state, but it
  doesn't read this marker: its own `miActorStillOwes` names the preparer for the
  arrival forms, behaviour this PR doesn't touch, so I took no screenshot there.
- **Not done:** the 375px look at the Waiting On bar. The reachable zero-pick
  mar auto-completes the plan in the same request, so there is no steady
  state for the bar to show; the wait is only transient. The bar's
  rendering is unchanged and reads the server row state.

## Behaviour changes to look at before merging

- Focus-pass log line is longer when someone is skipped.

## Open questions for the owner

None.

## Noticed, not fixed

- The test for the SR bug stores the commit directly because the real route
  auto-completes; no plan I found leaves a zero-pick commit waiting in the
  UI, so the bar bug was largely latent after the earlier gate-removal pass.
