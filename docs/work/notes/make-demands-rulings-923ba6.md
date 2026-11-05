# Make Demands rulings (2026-11-03)

## What changed and why

**The bug, proven first.** `/demand-retarget` only closed when a roll landed, and a
duel has no roll until its bouts end, so a Make Demands winner could swap a duel's
opponent mid-bouts. The new test
(`TestMakeDemandsRulings_DuelOpponentCannotChangeMidBouts`) failed on the old code:
the route answered 200 and the opponent changed. Confirmed, not just suspected.

**Stand-in** (design principle 9): the perform-steps Make Demands winner now takes
- Exchange Courtiers' riposte break (route, wait and log line; the peer still goes to
  the target),
- Spread Rumors' hide-source: the secret goes on one of *the preparer's* assets, the
  Make Demands winner picks which (on a mar the target-asset owner keeps their own role).
  The preparer is granted that one secret, since it is about their own rumor on their
  own asset, so they can read it; nobody else gains access, and the asset's other secrets
  stay hidden from them,
- Chronicle Histories' "Wrap the scene" (the shared `/complete` route lets the Make Demands winner
  through for that plan only; the preparer is locked out).
Seek Answers' veto is untouched. Each has a panel control for the Make Demands winner, a named wait
for everyone else, and its own log line. Waits use the existing generic
`plan_resolving` row state naming the Make Demands winner, so `waitingOn.ts` needed no change.
Spread Propaganda's "break yourself" was already built; it is now pinned by a test.

**Keep-or-change target.**
- One list, `demandRetargetPlans` in `handler/demands.go`: Exchange Courtiers, Spread
  Rumors, Propose Duel, Clandestinely Liaise. `pendingDemandRetargetChooser` and the
  `/demand-retarget` route use it, so on every other plan there is no roll-hold and the
  route answers 409. *Sharing choice:* a Go test (`demands_retargetable_test.go`)
  reads the frontend's `RETARGETABLE` set and fails if they differ. I chose that over
  having the server send the list because the frontend uses it in pure functions
  (draft cards, the overlay) with no game context to carry a server value, and the list
  changes about never.
- Propose Duel and Clandestinely Liaise wait for that Make Demands winner to settle the aim before
  their first step: the Waiting On bar names them (`await_demand_retarget`), every
  first-step route answers 409 until they have (`requireDemandAimSettled`), and both
  panels show a wait. After they settle, the route and the form close.
- A preparer who wins the option is never held, re-aims freely, and doing nothing
  means keep; the route closes for them once the duel's or liaison's first step has
  begun. The no-hold/keep part is today's behaviour and is pinned.
- Re-aiming a duel recomputes who declares first (initiative was computed from the old
  opponent when the plan started resolving).

**Smaller items.** `demandOptionInertReason`'s comment no longer calls the card a
hint; the draft-pick log line is Default severity (test added).

**Review round (PR #57).** Fixed from the coordinator's review, each with a test written
first:
- The aim lock treated "no phase yet" as locked, so a pending duel or liaison refused
  keep/re-aim. An empty phase now means "not begun" (Go and the `shared.ts` mirror).
- A duel under way (or a liaison past its first step) with an unsettled
  Make Demands winner would have waited on a call the route refuses. The hold now ends once the plan
  is locked.
- A stake laid in setup now locks the aim (Go reads the stake rows). The frontend can
  only see the viewer's own stake: the opponent's is hidden until both commit, so the
  server answers 409 for that case rather than the form hiding itself.
- The Chronicle wrap exception moved into `CompletePlan`; starting the plan stays the
  preparer's. The wrap line is anchored with `logRow` and posted after the plan resolves.
- Hide-source's default secret text names the preparer when a stand-in writes it;
  forfeiting a hide-source pick counts the preparer's assets for a stand-in; a riposte
  surrender by the Make Demands winner is logged naming who decided; Chronicle's wrap on a
  mar is covered by a test.

## Second review round (owner rulings, 2026-11-04)

- **Hide-source under a stand-in:** the preparer can read the secret (one grant for that
  secret only), and its default text names the preparer.
- **Liaison re-aim:** whoever re-aims, whether a Make Demands winner or a preparer who won the
  option, also names the new partner's meeting peer. `/demand-retarget` takes
  `partner_peer_id`; `mdApplyRetarget` validates it as the new partner's, live and a peer
  (the same check as preparing), then stores the new partner and peer on the liaison, so every
  later step follows them. The re-aim line in the log names the new partner and their peer.
  That reveals nothing new: preparing a liaison already announces both peers to the table. The
  form swaps the asset picker for a peer picker; a partner with no live peer is greyed with
  the reason beside them, and the preparer is never offered. There is no resolve-time check
  of the new peer, so the route refuses a re-aim until the plan is resolving (a pending liaison
  can only be kept), and the peer is checked once, at the re-aim. The existing steps already
  tolerate a meeting peer destroyed afterwards. If the preparer's own meeting peer is gone, a
  re-aim is refused in terms of the plan ("the preparer's meeting peer is destroyed, so this
  liaison can only be kept"), and the form says so; keeping still works.
- **`advance-liaise`:** the Make Demands winner of the perform option presses it, the preparer
  is refused, the bar names the Make Demands winner in Together at Last, and a log line says who moved the
  meeting on. Two older tests and one e2e step that had the preparer advancing under a perform
  win now have the Make Demands winner do it.
- No other liaison step needed moving: the seat steps (keep-secret, share-choice,
  redelay-reveal) already belong to whoever holds the preparer's seat, and no person presses
  a "complete" on a liaison (`clApplyRedelayOutcome` resolves it outright), so it is not a
  stand-in step.
- The hide-source secret's text is stored as plain text, with no log markup, since the
  preparer reads it as it is.

## How it was verified

- `make check-fast`: passed.
- `make test-integration` on a private database: passed (handler suite included).
- Each new test was shown failing with its fix undone (hide-source, riposte, wrap,
  break-yourself, duel/liaise hold, closing after settle, initiative recompute, dud
  plans, lock for a preparer who wins the option, draft-pick severity, list-drift test). The
  "a preparer who wins the option is never held, and doing nothing keeps" half passes without any fix; it pins
  today's behaviour as asked. Spread Propaganda's break-yourself was already
  correct; its test fails only if that code is broken.
- 375px views as the Make Demands winner, the preparer and a bystander on my own server:
  Propose Duel hold, Clandestinely Liaise hold, Exchange Courtiers riposte, Spread
  Rumors hide-source, Chronicle Histories wrap. A bug found that way: hide-source
  copy named the Make Demands winner as the asset owner; fixed.
- e2e `make_demands`, `propose_duel`, `clandestinely_liaise`, `chronicle_histories`,
  `exchange_courtiers`, `spread_rumors` against my own server: 73 passed after
  changing one spec (below).
- **Not run:** `spread_propaganda`, `host_festivity` and the rest of the e2e suite;
  nothing a Make Demands winner touches changed there.

## Behaviour changes to look at before merging

- On Spread Propaganda, Seek Answers, Chronicle Histories, Make Introductions, Propose
  Decree and Host Festivity a keep-or-change Make Demands winner no longer holds the roll, and
  `/demand-retarget` answers 409. The e2e test "a preparer who won keep-or-change-target
  is offered it" used Seek Answers; it now uses Spread Rumors, with a new test that
  Seek Answers shows nothing. Two integration tests that used Spread Propaganda as the
  retargetable plan moved to Spread Rumors.
- Chronicle's "Wrap the scene" is the Make Demands winner's, and the preparer is refused it, even on a
  mar. The waiting bar names the Make Demands winner from the last choice on.
- `/demand-retarget` now reloads the plan before finalizing (it used to overwrite
  `resolution_data` with the copy loaded at the top of the request, which would undo a
  duel's recomputed initiative).
- Duel champion election: a first "fight for myself" election (`asset_id: null`) was
  treated as unchanged and never set the declared flag. I fixed it
  (`plan_propose_duel_setup.go`) because the aim lock reads that flag. It is the start
  of the duel, so inside this piece, but it touches duel behaviour: the UI's "declared"
  flags now flip for a nil election.

## Open questions for the owner

None open. The liaison re-aim question was ruled on 2026-11-04 (below) and its question
file is deleted.

## Noticed, not fixed

- The Spread Propaganda break-yourself test lives in my new test file and uses
  `spPrepareToMar` from `plan_spread_propaganda_integration_test.go`; if the parallel
  piece renames that helper, rebase needs a one-line fix.
- `make check-fast` prints `middleware/auth.go: unreachable func AccountContext` from
  `deadcode`; it doesn't fail the check and isn't from this change.
