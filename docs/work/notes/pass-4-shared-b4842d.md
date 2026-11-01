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
5. **Make Introductions demanded winner.** Every MI resolution route accepts
   the Make Demands `perform_steps` winner and locks the preparer out, but the
   panel gated its forms on `isPreparer`. New `resolutionActorID(plan, plans)`
   in `plans/shared.ts` mirrors the server's `actsForPreparer` and is derived
   from the plans list (not the overlay's effect-fed bindable, which an SSR
   render never sees). Naming, both arrival forms and the mar-fate form follow
   it, and wait lines name the actor.

## How it was verified

Against this worktree's own build and a private database:

- `make check-fast` — passes (incl. svelte-check, Vitest 569, Go unit).
- `make test-integration` on a private DB — passes, 0 FAIL.
- Playwright e2e against my own server via `E2E_BASE_URL` — 25/25.
- New tests: `Stepper.test.ts`, `MakeIntroductionsPanel.test.ts` (winner gets
  the arrival form, preparer sees a wait on the winner), `resolutionActorID` in
  `shared.test.ts`, `plan_resolved_announce_test.go`. The winner tests were
  shown to fail with `resolutionActorID` returning the preparer (3 failures).
  The announce guard test wasn't separately reverted.
- By eye at 375px (dev game, players rhea/sol/tam): Make Introductions
  pre-roll as preparer (meter + Buffet) and as bystander; the full demanded-
  winner flow driven through the API — winner sees the naming form and then
  the arrival form, preparer and bystander see "sol is naming/welcoming…";
  Seek Answers post-roll picks as actor and bystander (Stepper). Buffet present
  in Spread Rumors and Spread Propaganda pre-roll as actor and bystander
  (text check only, no screenshot read).
- **Not checked by eye:** Stepper in Spread Rumors and Spread Propaganda (same
  component; e2e and unit tests only); the Make Introductions mar branch under a
  winner (only the make branch was driven); the Chronicle/Exchange Courtiers
  panels weren't touched.

## Behaviour changes to look at before merging

- **Spread Rumors, declined Take Asset:** the row used to render in the italic
  faint `.muted` style; it now fades (opacity .55) like the other two steppers.
- **Stepper buttons** gain `aria-label`s ("Fewer" / "More") and `type="button"`.
- **New Buffet** in SA, SR, SP and MI, collapsed by default, hidden once the
  picks are made. In SR the Mar tab reads from the preparer's main character.
- **Make Introductions** now shows a difficulty meter before the roll.
- **MI wait lines** ("… is naming the new peers…", "… is welcoming…") name the
  acting player, which is the demanded winner when there is one (they used to
  name the preparer).
- **Plan-resolution tail:** `resolvePlan` reloads the plan after storing the
  result; Clandestinely Liaise and Host Festivity previously didn't. No change
  to what is posted or broadcast.

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
