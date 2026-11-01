# spec-spread-rumors — Playwright regression specs for Spread Rumors

## What changed and why

One new file, `frontend/tests/e2e/spread_rumors.spec.ts` (no product code, no
shared helper touched; the helpers live in the spec). Five tests pin what the
UX audit and its follow-ups fixed:

- **Make + take-consent.** The pre-roll narration is its own step (no roll
  exists until the preparer states the rumor, and the stated rumor then reads
  as public). A take asks the victim by name for exactly that asset; a second
  request while one is open is refused (409), only the victim may answer (403),
  nothing moves until they agree, agreeing transfers the asset and resolves the
  plan, and a late request is refused too.
- **Refused take.** Refusing locks "take" off (greyed, "(declined)", and
  `make-choice` carrying it is a 400); the quota is refilled with other picks.
- **Mar.** The target-asset owner (bob), not the preparer, is the actor: the
  Waiting On set names bob, bob writes the counter-rumor first (the preparer
  gets a 403, options are gated behind it), the table reads it, and when bob
  takes from alice the consent screen alice sees names her own main character in
  the "also hits" line (#25), never bob's target asset. The Rumors sheet then
  holds the counter-rumor and not the failed rumor.
- **Zero-pick commit** (#27): the actor commits no picks, the choice marker is
  set, the plan resolves, the wait is no longer a `plan_resolving` one and a late
  counter-rumor is refused.
- **Stepper** (#29) at 375px: every option has its own labelled −/+ pair, all ten
  buttons are at least 44px square, nothing overflows the viewport, and the "+"
  buttons all disable at the dice quota.

All tests run in three browser contexts (alice preparer, bob target-asset
owner, cara bystander) at 375px. Setup (seed, prepare, advance, force the dice)
is over the API; everything a player does in the plan is driven through the
panel. Difficulty 3 for the mar tests comes from seeding the esteem track with
alice last: a forced mar rolls its highest losing result, so difficulty 2 gives
one pick and difficulty 3 gives two.

## How it was verified

Against my own server (`make build`, private database, port 8173/8175 via
`E2E_BASE_URL`), never `:8090` or `uneasy_test`:

- 4 consecutive clean runs, 5/5 each.
- Each guard broken locally, spec watched to fail, restored (nothing committed):
  repeat-request guard removed → make test fails; mar waitees naming the
  preparer → both mar tests fail; counter-rumor actor gate removed → mar test
  fails; `AutoCompleteAfterChoice` false → four fail, including the zero-pick
  test; consent screen subject switched to the target asset → mar test fails;
  Stepper 44px rule shrunk → stepper test fails; Stepper `aria-label` removed →
  tests fail.
- `make check-fast` passes. I looked at the Stepper at 375px (screenshot in my
  scratchpad): 44px square buttons, labels wrap, no sideways scroll. Disabled
  "−" buttons render as dark, low-contrast squares, as designed.
- Not run: `make test-integration` (no Go change).

**What does not bite.** The "already chosen" precondition of
`request-take-consent` is not pinned here: after a resolved plan the status
check refuses first, so the e2e "late request" assertion is true but passes with
that guard removed (the integration test covers the guard). Likewise the
zero-pick *marker* itself has no steady state to observe: the reachable
zero-pick commit auto-completes the plan in the same request, so the spec can
only pin the outcome (resolved, wait moved on, counter-rumor closed). It fails
if auto-completion is lost, not if only the marker read in `ResolvingWaitees`
regresses (a Go test covers that).

## Behaviour changes to look at before merging

None; tests only.

## Open questions for the owner

None.

## Noticed, not fixed

- One run of the pre-roll test failed once while a deliberately broken
  frontend was freshly built and the server cold; it did not recur in 4 clean
  runs plus the other bite runs. If it ever flakes, the first `toBeVisible` after
  `page.goto` in that test (10s) is the place to look.
- Seed players' main characters are named "<name>'s main character", so the mar
  copy reads "alice's main character, alice's main character" in dev; it is a
  seed-name artefact, not a product bug.
- The known `spread-rumors-take-consent-revalidate` gap is untouched, as asked.
