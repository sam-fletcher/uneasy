# Pass 4: shared components

## What changed and why

Five commits, one per item.

1. **`plans/shared/Stepper.svelte`** replaces the −/count/+ row hand-rolled in
   Seek Answers, Spread Rumors and Spread Propaganda. It owns the markup and
   the 44px tap targets; each panel still passes its own count and its own
   cap/quota rules as disabled flags. The old `.stepper-row` CSS in
   `planPanel.css` is gone.
2. **Buffet adoption.** The audit's complaint was that these plans' pick lists
   can't be previewed before the roll, which is when the others decide whether
   to aid or interfere. Seek Answers, Spread Rumors, Spread Propaganda and Make
   Introductions now show the shared read-only Buffet until the picks are made.
   The text reuses each plan's existing labels (viewer-neutral `summary` where
   one exists). Nothing hand-rolled was *replaced* — none of those plans had a
   preview of their own; Make Demands still has none (see below).
3. **`DifficultyMeter` in Make Introductions**, while peers are named and the
   dice are in the air: value `2 + peers`, floor 2.
4. **`resolvePlan` / `announcePlanResolved`** (`handler/plan_resolution.go`).
   `EmitPlanResolved` was already the single announcement; what five plans
   repeated was its tail (store the result, reload, broadcast `plan.resolved`,
   post). `resolvePlan` owns that sequence and is used by the shared completion
   path, Make War, Clandestinely Liaise, Host Festivity and the fizzled re-aim.
   `announcePlanResolved` is the event+post pair, used directly by the finale
   fall-through and by Exchange Courtiers (which logs its two asset transfers
   first). Per-plan wording stays where it carries information: the
   `ResolvedDescriber` lines for Host Festivity and Clandestinely Liaise. A guard
   test (`plan_resolved_announce_test.go`) fails if a handler calls
   `SetPlanResult` outside `resolvePlan`.
5. **Make Introductions demanded winner.** The post-roll MI routes accept the
   Make Demands `perform_steps` winner and lock the preparer out, but the
   panel gated its forms on `isPreparer`. (Naming the peers and rolling are
   pre-roll, so they stay the preparer's: `create-peer` and `finalize-peers` now
   403 the winner, and `ResolvingWaitees` names the winner only once the make or
   mar branch is pending. This reverses what PR #9 had done.) New `resolutionActorID(plan, plans)`
   in `plans/shared.ts` mirrors the server's `actsForPreparer` and is derived
   from the plans list (not the overlay's effect-fed bindable, which an SSR
   render never sees). Naming, both arrival forms and the mar-fate form follow
   it, and wait lines name the actor.

## How it was verified

Against this worktree's own build and a private database:

- `make check-fast` — passes (svelte-check, Vitest 574, Go unit), run after the review fixes.
- `make test-integration` on a private DB, full suite, run on the final commit: every package `ok` (handler 132s). An earlier full run, before the review fixes, failed once in `TestWaitingOn_MakeIntroductions_PerformStepsWinnerIsNamed`; that is the test described under Behaviour changes, now updated.
- Playwright e2e against my own server via `E2E_BASE_URL` — 25/25 (built from the review-fix code).
- New tests: `Stepper.test.ts`; `MakeIntroductionsPanel.test.ts` (arrival form
  and mar-fate form go to the winner, naming form stays with the preparer, each
  with the other side's wait line); `resolutionActorID` in `shared.test.ts`;
  `plan_resolved_announce_test.go`; integration
  `TestMakeIntroductions_PerformStepsWinnerNamesPeers` (winner naming or
  finalizing pre-roll → 403, bar names the preparer, then the winner owns the
  arrival). Each was shown red: the winner tests with `resolutionActorID`
  returning the preparer, the naming render tests with the naming gate on the
  actor, the integration test with the preparer gate and waitee change reverted,
  and the announce guard with a bare `SetPlanResult` in a stray handler file.
- By eye at 375px (dev game, players rhea/sol/tam): Make Introductions
  pre-roll as preparer (meter + Buffet) and as bystander; the full demanded-
  winner flow driven through the API on the final build — the winner's naming
  attempt is refused (403); the preparer names and rolls; then the winner gets
  the arrival form while the preparer and a bystander see "sol is welcoming the
  new peers…";
  Seek Answers post-roll picks as actor and bystander and Spread Propaganda mar
  picks as actor (Stepper). Buffet present in Spread Rumors and Spread
  Propaganda pre-roll as actor and bystander (text check only).
- **Not checked by eye:** the Stepper in Spread Rumors (same component; tests
  only); the Make Introductions mar branch under a winner (render test only).

## Behaviour changes to look at before merging

- **Spread Rumors, declined Take Asset:** the row used to render in the italic
  faint `.muted` style; it now fades (opacity .55) like the other two steppers.
- **Stepper buttons** gain `aria-label`s ("Fewer <option>" / "More <option>") and `type="button"`.
- **New Buffets** — previews on four plans that had none, not replacements; read
  their copy. In SA, SR, SP and MI, collapsed by default, hidden once the picks
  are made. In SR the Mar tab reads from the preparer's main character.
- **Make Introductions** now shows a difficulty meter before the roll.
- **MI wait lines** ("… is naming the new peers…", "… is welcoming…") name the
  acting player, which is the demanded winner when there is one (they used to
  name the preparer).
- **Exchange Courtiers:** its `plan.resolved` WebSocket event now fires after the
  two asset-taken log posts instead of before them.
- **Make Introductions naming** is the preparer's alone, even with a
  perform_steps winner (server-enforced; see item 5).
- **Plan-resolution tail:** `resolvePlan` reloads the plan after storing the
  result; Clandestinely Liaise and Host Festivity previously didn't. No change
  to what is posted or broadcast.

- **Edited PR #9's test:** `TestWaitingOn_MakeIntroductions_PerformStepsWinnerIsNamed`
  (`waiting_on_pass_integration_test.go`) asserted that the winner names the
  peers and the bar names them. That contradicts the standing ruling that
  `perform_steps` is post-roll only (the preparer still rolls), so naming and
  finalizing now go through the preparer in that test; the winner-owns-the-arrival
  assertions are unchanged.

## Open questions for the owner

None.

## Noticed, not fixed

- The Stepper's "+" still renders as a stark white native button (the audit's
  "missing button variant" finding): `.action-btn` with no variant has no
  background. Shared now, so it is a one-place fix.
- Make Demands has no "What can happen?" preview of its four options (audit
  group D); it isn't a make/mar list, so I left it out of this pass.
- The Make Introductions mar branch still gates the "who writes the
  marginalia" chips on `playersExcept(preparer)`, which is right (the server
  rejects the preparer), but under a winner the *winner* could in principle be
  named; the server permits it. Unchecked.
