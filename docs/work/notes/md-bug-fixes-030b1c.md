# Make Demands bug fixes (three items)

Branch `claude/md-bug-fixes-030b1c`. Fixes the three open Make Demands items; their
files in `next/` are deleted.

## What changed and why

1. **`demand.prepared` no longer fetches `/api/plans/undefined`.** The event carries a whole
   plan (`{ plan }`), but `ws-handlers.ts` read it as `{ plan_id }` with the group of plan
   events. Both senders (`mdHandler.OnPrepare` and the counter-demand synthesis) also send
   `plan.prepared` with the same plan, which already puts it on screen, so the event is now its
   own case that fetches nothing and only announces itself on `window`. The e2e allow-list entry
   for the 400 is gone. The counter-demand path is driven by the existing
   "a marred demand: … places the counter-demand" spec, which now passes with the entry removed.
2. **The preparer plays a duel's bouts under a perform-steps demand.** The server seated the
   winner for `bout-declare` and `bout-respond`, but the panel gives bout controls only to the
   duellists. `pduelEffectiveSeat` and `pduelSeatHolder` are deleted: the bout routes accept the
   two duellists only (403 otherwise), and the Waiting On acting set is the duellist. The winner's
   choices start with `make-choice` once the roll resolves (that route and its Waiting On
   handoff were already the winner's). The demand's buffet line and the draft card said "play
   the preparer's side of the bouts"; they now say the winner chooses which staked assets a make
   takes after the roll and the preparer still plays the bouts.
3. **A held winner cannot ready before deciding.** `SetReady` answers 409 for a winner in
   `pendingDemandRollBlockers` (control_leverage or keep_or_change_target, decision not stored).
   `DiceRollPanel` takes a `heldDecisions` prop (computed in `MainEventView` from the new
   `heldRollDecisions` helper in `plans/shared.ts`) and shows "The roll waits for your Make Demands
   choice: …" where Ready and Pass would be, and drops the "or pass" brief. The auto-ready sweep
   (`runAutoReadySweep`), the skip-leverage short-circuit and the seed already honoured the hold; a
   new test pins the sweep. The e2e "form is gone" specs now wait for the decision to land before
   they ready everyone (the click returns before the request does, which the new 409 would expose).

The owner's two readings are recorded in the private rulings (Make Demands).

## How it was verified

Against this worktree's code (private database `uneasy_claude_md_bug_fixes_030b1c`, own server on :8163):

- `make check-fast`: passes (Go lint and unit, svelte-check, Vitest 2390, tracker check).
- `make test-integration`: full suite on the private database. One failure, an older Waiting On duel test (`TestWaitingOn_ProposeDuel_…`) that pinned the old seating; I rewrote it to the new reading and re-ran the duel tests (`TestWaitingOn_ProposeDuel|TestProposeDuel`), which pass. The full suite was not run again after that one edit.
- Each new test was shown failing without its fix:
  - `ws-handlers.test.ts` › "demand.prepared, whose payload is a plan, fetches no plan": one
    `getPlan` call with the fix reverted.
  - `TestProposeDuelHTTP_PerformStepsWinnerStartsAfterTheRoll` (replaces the test that pinned the
    old seating): winner got 200 on `bout-declare` and Waiting On named the winner, without the fix.
  - `TestPlanLifecycle_MakeDemands_HeldWinnerCannotReadyBeforeDeciding` (both options): the ready
    returned 200 without the fix.
  - Not a bite check: `…HeldWinnerCommittingTheirLastDieStaysHeld` passes without a code change; it
    pins the sweep's existing behaviour. The `DiceRollPanel` and `heldRollDecisions` unit tests use
    a new prop and helper, so they can't exist without the change.
- `make_demands.spec.ts`: 19 of 19 pass against my own server, including a new spec for the held
  winner (reminder shown to the winner only, the early ready 409s, the reminder goes once decided).
- 375px screenshots (throwaway spec, not committed): the duel's bouts with a perform-steps demand
  in place as the preparer (alice sees "Your move: Declare the bout") and the winner (bob sees
  "Waiting on alice", no controls); the roll panel for a held winner as winner, preparer and
  bystander (text of all three read). Only the with-a-stand-in duel was rendered; the duel without
  one is unchanged code.

Did **not** run: the other e2e specs (only `make_demands.spec.ts`), a desktop-width look, the
reminder notification path beyond reading that it takes its names from the row state.

## Behaviour changes to look at before merging

- A perform-steps winner against a Propose Duel now sees nothing to do until the roll resolves.
  Before, the server let them declare for the preparer (the panel never offered it).
- A held winner who presses Ready (or posts it) gets a 409 until they decide. Their draft of
  leverage picks can't be submitted until then either, since the submit goes through Ready.
- The Make Demands banner on a duel still says "You will submit this plan's make/mar choices in
  alice's place", which is true for the claim after the roll and no longer implies the bouts.

## Open questions for the owner

None.

## Noticed, not fixed

- `handler/dev.go` readies every participant with `SetAllParticipantsReady` for the dev force-roll
  path; it ignores the hold. It is dev-only, so left alone.
- A held winner's intent chips (Aid / Interfere) stay on screen while their Ready is withheld; they
  can draft but not submit until they decide.
