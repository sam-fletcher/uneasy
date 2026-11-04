# plan-choice-rulings — hand-over note

## What changed and why

The owner's 2026-11-03 rulings on optional and capped plan choices.

**Propose Duel, mar.** The opponent may spare the loser instead of taking
stakes, all or nothing. `make-choice` on a mar now takes either the picks or the
single value `["spare"]`; a mar with anything claimable never applies itself, so
`AutoApplyChoiceOnRoll` no-ops until they answer (`pduelResolveClaim`,
`pduelMarMayDecline`). Sparing logs "… spared X, taking none of the staked
assets; all staked assets are leveraged." The make's forced claim is untouched,
and a mar with nothing claimable offers no decision. `RollPhase.svelte` shows
"Take the stake / Take all N stakes" + "Spare X" (or the picker + "Spare X
instead" when the difficulty is below the pile).

*Leverage on a spare — settled by the book, not chosen by me.* p.60 puts "Both
players then leverage all staked assets" in its own sentence after the take, so
sparing skips the take and the leverage still happens. No owner question needed.

**Host Festivity.** `EventEndable` no longer requires earned makes to be taken;
a guest's mar IOU and an insisted host mar still block. Ending with makes left
posts "X ended the event, leaving N earned make(s) unclaimed." before the plan
resolves (naming whoever pressed End; "the host's" when a stand-in did). The End
button carries one line: "Ending now forfeits your N unclaimed Make(s)." (or "the
host's" for a stand-in). "earns a make" is now Default severity. Disagreement
peer and turn order unchanged.
Owner ruling 2026-11-04 ("Winner ends it"): when a perform_steps Make Demands
winner holds the host's steps, that winner presses End and the host is refused;
no winner leaves the host unchanged. `hfEndEventHandler` uses
`requireResolutionActor`, the panel shows End to `amStepActor`, and
`ResolvingWaitees` names the stand-in for the end and no longer treats unclaimed
makes as owed (an insisted mar still is). A guest's IOU blocks whoever presses End.

**Spread Propaganda, counter-propaganda.** The recursive plan is now created at
the ordinary pre-roll step (no roll, `PreRollDone` false). The interferer states
their message with the existing `sp-cast-roll` (so the message is posted before
the roll opens) or declines with the new `sp-decline-counter`, which logs, sets
`counter_declined` in `resolution_data` and closes the plan with the existing
"came to nothing" ending (`fizzledOutcome`, NULL result). New row-state kind
`await_counter_propaganda` names the interferer plus whoever owes a parent step, via
`spHandler.ResolvingWaitees` — on the child itself and on the parent while it is
still resolving (a give_peer/break_self owed), wired into `waitingOn.ts` and the
frontend `RowStateKind`. The one-open-roll guard is unaffected: the child has no
roll until it is cast, and the parent's roll is already settled.

**Spread Propaganda, caps.** `ApplyChoice` rejects a repeated `lay_low` (422)
before anything applies, so a rejected request leaves no lockout, no choices and
no post. A repeated counter_prop is refused the same way (422; it used to be a
silent no-op), and a second mar submission is refused once a mar is committed
(409). The picker's "already chosen" reason is intact.

## How it was verified

All against this worktree's code (own server on :8192, private database):

- `make check-fast` passes (after `make build`; without a built frontend
  `cmd/server`'s warm-up test fails on an empty embed, unrelated to this PR).
- `make test-integration` on the private database: handler suite passes; the
  only failure in my first full run was that same missing-embed test, fixed by
  `make build`.
- New tests shown failing with their fix undone: duel mar-asks-first and spare
  (`pduelMarMayDecline` forced false), festivity `EventEndable` (game + handler),
  head-down cap, counter-propaganda row state, decline route.
- e2e against :8192: `propose_duel` (all 11 — the forced-claim mar specs were
  rewritten, one spare spec added), `host_festivity`, `spread_propaganda`.
- 375px: Spread Propaganda counter step as the interferer, a bystander and the
  parent's preparer (screenshots); Host Festivity end-event as host and as a
  guest (guest sees no control). The duel claim panel was driven at 375px by the
  e2e spec as actor, other duellist and bystander, asserted by text and buttons,
  not screenshotted.

Not covered: `ApplyChoice`'s own "spare travels alone" check
(`len(choices) != 1`) has no separate test — at difficulty 1 the pick budget
refuses a mixed request first. No e2e covers counter-propaganda end to end (the
integration tests do).

## Behaviour changes to look at before merging

- A mar duel now always waits on the opponent, including the 1v1 case that used
  to settle itself; a drawn duel (always a mar) too.
- Hosts can end Festivity with makes unspent. With a perform_steps winner, End
  moves to the winner and the host is refused (403). The bar no longer waits on
  the host merely because makes are unclaimed.
- Counter-propaganda now stalls the row until the interferer acts (the owner's
  ruling: the gate earns its stall). There is still no timeout, as everywhere.
- A declined counter-propaganda plan appears resolved with no result ("came to
  nothing"), the same shape as a fizzled re-aim.

## Open questions for the owner

- Declined counter-propaganda: is the preparer's mar pick still spent? See
  `docs/work/questions/declined-counter-propaganda-pick.md`.
- (Host Festivity's end-event with a Make Demands winner is ruled and built; see
  the third review round below.)

## Review round (coordinator's findings)

- A mar is committed once: `CheckChoice` refuses a second submission (409), so
  the head-down cap holds across requests. Counter-propaganda is capped at one
  pick the same way (422 on a repeat).
- A mar duel's take is exactly the difficulty, capped by what is claimable:
  wrong count, an unparseable or repeated id, or an unstaked asset is a 422;
  spare is the only way to take none. The make is unchanged except that both
  sides now ignore a destroyed stake (below).
- Destroyed stakes (reachable: `PUT /api/assets/{id}` has no phase gate and a
  main-character swap can destroy the old one; stakes have no MC restriction).
  The server drops a destroyed stake from the claimable pile (it used to fail in
  `takeAssetEffect` after every stake had been leveraged), and the picker counts
  only live stakes. **This is a behaviour change to the make's claim too.**
  The freeze it can cause is fixed in the review round below.
- The counter-propaganda wait now names the interferer plus whoever must move a
  parent step still owed (the preparer, or a Make Demands perform_steps winner),
  computed identically from either plan of the pair. The interferer is shown the
  counter plan, others the parent (`activeResolvingPlan` takes the viewer).
- Pinned: "earns a make" at Default severity; two-request resubmission;
  counter-propaganda with a step owed; exact take; destroyed stake.
- e2e added: counter-propaganda by click (cast with the parent still owing a
  step; decline). 375px shots in the PR: duel claim as actor, other duellist
  and bystander; the interferer's view with the parent open.
- `_Spare_RejectedWhereNotOffered` is replaced by `_TakeIsExactlyTheDifficulty`,
  which covers spare+pick, junk, unstaked and over-budget requests.

## Noticed, not fixed

- For piece 3: a declined counter-propaganda plan closes through `EmitPlanResolved`
  with "Spread Propaganda came to nothing."; the wording, and a line under the
  Decline button, belong to piece 3 and are not built here. I only updated a
  stale comment inside that function.

## Review round 3: "Winner ends it" (2026-11-04)

- `hfEndEventHandler` goes through `requireResolutionActor`, the existing
  stand-in helper on `main` (nothing from #57). Winner ends, host 403; no winner
  is unchanged. A guest's IOU still 409s whoever presses End.
- `ResolvingWaitees` and its comment name whoever can end once nothing is owed;
  makes are not owed. The lapse line names who pressed End and, for a stand-in,
  says "the host's" makes. The forfeit line reads "the host's" for a non-host.
- Test: `TestHostFestivity_PerformStepsWinnerEndsTheEvent` (IOU blocks the
  winner, bar names the winner alone, host 403 with no post, winner ends, log
  line). Shown failing with the guard and waitees reverted.
- 375px against a game seeded with a made Make Demands plan: the winner sees End
  and "the host's 3 unclaimed Makes"; the host sees no End and "Waiting On: bob";
  a guest sees no End. No e2e drives this (the Make Demands setup is piece 1's
  ground); the integration test and screenshots cover it.

## Review round 4: the duel that froze when a stake was destroyed

- Reproduced first: a mar whose loser's only stake is destroyed while the claim
  waits, and a make (result 2, pile of 3) with all three, or one, destroyed. In
  each the UI offers nothing, yet nothing applied the claim, because the
  auto-apply only runs when the roll lands; the duel sat in 'roll'.
- Chose the server settling it itself: `settleDuelClaimsAfterDestroy`, called
  from `emitAssetDestruction` (every break) and the main-character swap. It
  re-runs the roll's auto-apply for each resolving duel staking the asset;
  `ApplyChoice` no-ops while a real decision remains, so a destruction that
  leaves one changes nothing. Reason: a client-fired settle needs someone's tab
  open, and the whole point is that nobody has a button. Cost: one new sqlc
  query (`ListResolvingDuelPlansStakingAsset`) and two call sites; best effort,
  errors logged.
- Tests (shown failing before the hook): `_Mar_AllStakesDestroyedSettlesItself`,
  `_Make_AllStakesDestroyedSettlesItself`, `_Make_OneStakeDestroyedLeavesNoChoice`.
  They destroy through `breakAsset`; the main-character-swap call site has no
  test of its own.
- A mar settled this way posts "took 0 staked assets" rather than a spare, since
  nobody chose mercy. Wording is piece 3's if it should differ.
