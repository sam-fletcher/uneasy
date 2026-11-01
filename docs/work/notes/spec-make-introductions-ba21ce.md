# spec-make-introductions: e2e regression specs for Make Introductions

## What changed and why
`frontend/tests/e2e/make_introductions.spec.ts` pins what the UX audit fixed:
- **Pre-roll:** the difficulty meter (2 + peers) and the "What can happen?"
  buffet are visible to every player; only the preparer gets the naming form,
  others see who is naming, and the server 403s a non-preparer's `create-peer`.
- **Make:** the arrival form pre-fills the name, stays disabled until a
  marginalia is written, and the last arrival says it resolves the plan.
- **Mar fates:** a used outcome's chip is `aria-disabled` and tapping it shows
  the reason; the server rejects a repeated outcome; the recipient and the
  "not who you thought" author are each asked for their marginalia, the
  preparer is not; the plan resolves when the last is written.
- **Make Demands perform_steps (Pass 4, #29):** a real Make Demands is driven
  to a perform_steps win (skip vote, leverage, `force-roll`, draft picks).
  Pre-roll the winner is a bystander (preparer names; winner 403s); post-roll
  the winner gets the arrival form and the preparer sees "bob is welcoming…".
- Helpers live in the spec file; no shared file touched.
- Review round: mar test now asserts who owns each peer and which marginalia
  landed where; the demand test reads the demand's winners (bob holds
  perform_steps and keep_assets) and the arrived peer's owner; the Waiting On
  bar is pinned (preparer while naming, then the marginalia authors); the
  removed gate buttons (Apply choices, Begin resolving, Complete plan) are
  asserted absent; `finalize-peers` 403s a non-preparer; a broken_journey +
  delayed test; contexts close in an `afterEach`.
- Three `test.fixme`s for live-update gaps: the roll's auto-apply (owned by
  `auto-applied-choice-not-broadcast.md` in the Spread Propaganda PR) and the
  step routes (`make-introductions-step-routes-not-broadcast.md`).

## How it was verified
Against this worktree's own build on port 8161 with a private DB (dropped
afterwards): spec passed 3 runs in a row after the review round (5 passed, 3 fixme-skipped each; fixmes un-skipped once to confirm they fail for the stated reason);
`make check-fast` passes. Bite checks (each reverted): removing the meter,
the `aria-disabled` on used chips, and gating arrivals on the preparer rather
than the actor failed the pre-roll, mar and perform_steps tests; making
`requireMIPreparer` a no-op and disabling `MarOutcomeTaken` failed the
403/duplicate assertions; enabling the arrival button with no marginalia and
removing the Buffet failed the make and pre-roll tests. Not run: integration
suite (no Go changes), mobile screenshots (viewport is 375px in the specs).

## Behaviour changes to look at before merging
None — test-only, plus a tracker item.

## Open questions for the owner
None.

## Noticed, not fixed
- **Make/mar from the dice isn't live** (`auto-applied-choice-not-broadcast.md`, PR #30): the auto-applied
  outcome saves `resolution_data` but broadcasts no `plan.choice_applied`, so
  the preparer's arrival/fate form needs a reload; likewise a player named to
  write a mar marginalia doesn't see their form (the bar says it's their turn).
  Specs reload to get past it; the fixmes pin the live behaviour. Likely
  affects other auto-applying plans too.
- Playwright treats `aria-disabled` chips as non-actionable, so the spec uses
  `click({ force: true })` on them.
- Sibling sessions seem to kill `./server` by name: run a private copy under
  a unique binary name.
