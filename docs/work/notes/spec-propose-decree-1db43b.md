# Propose Decree regression spec (Phase 4)

## What changed and why

- `frontend/tests/e2e/propose_decree.spec.ts` — eight live specs and one
  `test.fixme`, pinning what the 12-plan audit fixed in Propose Decree:
  - **Preparation:** the Prepare button waits for text; the server refuses a
    blank decree (400).
  - **Council meeting:** the preparer finalizes the text and opens the debate
    (everyone then sees the proposed law, live); a non-preparer or blank text is
    refused; the signatory's "End the debate" stays disabled and names the
    undecided players until each has joined (leveraging one asset, through the
    picker) or declined; `call-roll` is 409 while anyone is undecided and 403
    for a non-signatory; a decision is made once (every repeat is 409); the
    roster shows who joined and who declined; the log carries open / join /
    decline / call-roll.
  - **join-council gates:** exactly one asset, owned by the joiner, existing —
    each refusal asserted by status (400/403/404) and shown to spend nothing.
  - **Make:** passing the decree is automatic (no "pass" gate); the addendum
    comes before enactment and each out-of-order or wrong-actor call is refused
    (409/403/400); non-actors see waiting lines and no buttons; the enact button
    waits for a resource name and marginalia; the law lands under Laws with its
    addendum and signatory (read in the drawer by a bystander); the resource is
    owned by the preparer; the log carries the assembled final law and the named
    resource, in order.
  - **Mar (preparer ranked last):** everyone is seated, nobody is asked to
    decide, the top-ranked player signs; amendments run lowest power first,
    one at a time, each working from the last text; out-of-turn amend, skip and
    early addendum/enact are refused; the "writing the law (being amended)." copy
    keeps its space; the law is enacted with the amended text and a mar creates
    no resource; the log carries the amendment, the "left unchanged" line and the
    assembled law.
  - **Monarch (never audited):** with the lowest-ranked player crowned and the
    preparer above them, the monarch is seated for free and signs; the
    preparer can't call the roll or place the addendum; the other low-ranked
    player still owes a decision; the monarch cannot also leverage in (409); the
    preparer keeps the resource. Behaviour matches the monarch ruling (ADR-007
    Phase C) — nothing looked wrong, so no `fixme` and no item.
  - **Roll in progress:** the finalized decree text (not the prep draft) is on
    screen while the dice are open — the audit's "text vanishes" finding is
    fixed (the coordinator asked for a fixme; it passed, so it is a live spec).
  - **Joiner's die and the Waiting On bar:** a joiner holds one `decree` banked
    die, gone once the roll settles; during the debate the bar names the
    undecided players, not the signatory.
  - **Phone width:** Join/Decline/Open-the-debate are ≥44px and nothing
    overflows at 375px.
  - **`test.fixme`:** the signatory is handed the addendum live after the dice
    land. Fails today because of the known gap
    `docs/work/next/auto-applied-choice-not-broadcast.md` (no new item); the
    other specs reload after the roll for that reason. Confirmed to fail today.
- `frontend/tests/e2e/helpers/propose_decree.ts` — seeding, driving, log-reading
  and the `crown` helper, for this plan only. Shared `helpers.ts` is untouched.
- Product code is untouched apart from the dev-only crown route.

Not pinned, on purpose: whether the plan resolves itself once the law is enacted
(`docs/work/questions/propose-decree-make-autoresolve.md`) — nothing asserts the
plan's status after `enact-law`; the law row and the resource are the outcome.
Also not pinned: whether the signatory may leave the addendum blank (an open
audit question), so the specs always write one.

## How it was verified

Against this worktree's own build, on a private database and port
(`E2E_BASE_URL=http://localhost:8197`, binary `pd-server-1db43b`, database named
after the branch; never `:8090` or `uneasy_test`):

- The spec: 8 passed + 1 fixme-skipped, three runs in a row (after two earlier
  flaky runs were fixed: a click raced a direct `enact-law` call, and the page
  was reloaded before the server had recorded the auto-applied outcome —
  `forceAndSettle` now waits for it).
- Each spec was shown to bite by breaking the server locally and reverting:
  `call-roll` ignoring undecided players → the council spec fails; amendment
  order reversed → the mar spec fails; addendum dropped from the law → make,
  mar and monarch fail; the final law dropped from the enacted log line → the
  mar spec fails; the law's signatory forced to the preparer → mar and monarch
  fail; the monarch ignored when picking the signatory → the monarch spec
  fails. Not shown to bite (frontend-only): the "(being amended)." copy space,
  the ≥44px tap targets and the preparation button state — I did not rebuild the
  frontend with those broken.
- `make check-fast` passes; `make test-integration` passes on a private database (includes the new `DevCrown` tests). The new banked-dice, Waiting On and Join tap-target assertions were not individually shown to bite.

## Behaviour changes to look at before merging

None for players. One dev-only route (`/api/dev/crown`), gated by `UNEASY_DEV` like the others.

## Open questions for the owner

None new. (The existing auto-resolve question is untouched.)

## Noticed, not fixed

- **New dev route `POST /api/dev/crown`** (small, allowed by the coordinator):
  does what the Prologue title claim does (new peer with a monarch-titled
  marginalia, throne gate tripped), broadcasts the peer and posts to the log.
  Documented in `docs/DEVELOPMENT.md`; covered by
  `handler/dev_crown_integration_test.go`. It replaces an earlier helper that
  wrote SQL through `docker exec`.
- **Propose Decree stages a scene**, so its log lives in the chat's scene group,
  not a `.plan-group` — the Exchange Courtiers log helper can't read it, hence
  `expectLog` here. Worth knowing when the other scene-staging plans get specs.
- **`/api/dev/force-roll` can leave a roll open.** In about 1 run in 15 the
  decree's roll stayed in the leverage stage with one participant not ready: the
  dev route marked everyone ready before the third participant row existed, so
  the roll never resolved. Not a game bug as far as I know (real play has no such
  single sweep), but it makes specs flaky; `forceAndSettle` works around it by
  forcing again until the outcome is recorded. The cause (a participant row added
  after the roll is created) wasn't traced.
