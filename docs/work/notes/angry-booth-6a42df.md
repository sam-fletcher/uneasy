# A late roll fetch can no longer undo a resolved roll

## What changed and why
The table page's `roll.created` handler fetched `GET /api/rolls/<id>` and wrote
the answer into `activeRoll` whenever it landed, with nothing checking that
anything newer had happened. Every other roll event only patches a roll that is
already on screen, so an event that arrived while that fetch was still out was
dropped, and the older answer then installed the roll as it stood before the
event. For `roll.resolved` that left a resolved roll showing "Dice roll in
progress…" (or a duel's choices never appearing) until a reload.

Plans already had the cure (`planSeq` / `refreshPlan`: a response applies only
if it still holds the latest sequence number). Rolls now have the same, in
`frontend/src/routes/table/[id]/ws-handlers.ts`:

- `fetchRoll` takes a sequence number; its answer applies only while it still
  holds the latest one. The state is per page (a `WeakMap` keyed by the
  `WSContext`), not module-global, so tests and a second page can't see each
  other's fetches. There is one slot because the table shows one roll at a time:
  only one non-shake-up roll can be open (a partial unique index), and the
  shake-up opens the next roller's roll in the same request that resolves the
  previous one, so `roll.resolved(N)` is followed at once by `roll.created(N+1)`,
  which the slot plus the sequence number handle.
- `roll.resolved` installs the roll and its dice from its own payload when the
  roll's fetch is still out (or the roll is already on screen), then supersedes
  the fetch with a fresh one, which fills in the participants and ballot the
  payload doesn't carry.
- Every other event about a roll (`leverage_added`, `stage_changed`,
  `intent_set`, `ready_changed`, `vote_cast`, `vote_resolved`) does the same
  supersede-and-refetch if the roll is still loading. Same bug shape: the event
  had nothing to patch, and the stale answer lacked its effect. Once the fetch
  has landed these are no-ops, so a normal roll costs no extra requests.
- `demand.leverage_set` did a bare `getRoll(...).then(dice)` for the roll on
  screen, so a late answer could overwrite the dice `roll.resolved` had just
  installed. It now goes through `fetchRoll` (a full refetch of the roll,
  dice, ballot and participants, rather than dice only) and supersedes a
  loading fetch like the others.
- `plan.resolved` drops a fetch still out for that plan's roll. It already
  clears the roll; without this a late answer, or the post-resolution refetch,
  would put the roll straight back after the plan finished.
- A failed fetch still falls back to the roll `roll.created` announced, unless a
  newer event got there first.

Judgement calls: the in-flight state holds just the announced roll (its id and
plan id are on it), and the helpers are named for what they do
(`supersedeRollFetch`, `rollFetchInFlight`, `claimLatest`). The eight
`supersedeRollFetch` calls are copied into the event cases rather than hoisted,
which fits that switch; the `it.each` over every roll event in the tests is what
stops a future roll event from silently skipping it.

`docs/work/next/roll-fetch-overwrites-resolved.md` (the item this fixes) is
deleted, and the comment in `tests/e2e/helpers/roll_open.ts` that pointed at it
now says the bug is fixed and why specs should still wait for the panel.
`expectRollOpen` and its calls are untouched.

## How it was verified
Against this worktree's own code, rebased on `main` with PR 65 merged:

- **Vitest** (`ws-handlers.test.ts`, 18 new cases): resolved before the fetch
  lands; a stale answer after `roll.resolved`, including when the refetch also
  fails; a resolved roll replacing an older one still on screen (asserted before
  any fetch lands, with a ballot seeded); the newest fetch winning out of
  order; a failed fetch falling back, and not overwriting a resolution; a
  plan-linked roll not coming back after its plan resolved, with the fetch
  succeeding and failing; `demand.leverage_set` not undoing a resolution; an
  `it.each` over all seven roll events asserting a loading fetch is superseded;
  an event about another roll leaving the fetch alone; and the normal order
  costing exactly one request. 15 of the 18 fail on the old handlers; the other
  three pin behaviour that must not change.
- **Mutation check** (a scratch copy of the handler, each guard broken in turn,
  restored after): removing the votes/participants reset, the latest-check in
  the fetch's `then` or `catch`, the `plan.resolved` drop, the bare-`getRoll`
  revert of `demand.leverage_set`, or the supersede call from any one of the
  seven events turns at least one test red. The "supersede refetches any
  in-flight roll, not just this one" mutation was green until the
  other-roll test was added.
- **Playwright, with latency injected on `GET /api/rolls/<id>`**, on my own
  server and a private database, using a throwaway copy of the suite whose
  browser contexts answer that route late (not committed). The old-build numbers
  below were measured on this branch *before* rebasing onto PR 65, so the specs
  did not wait with `expectRollOpen`:
  - *Delayed 400 ms* (the real response is read at once and handed over late, as
    a slow client sees it): old build **14 failed / 145 passed**; the fixed build
    **159 passed**. The 14 are the force-roll specs, the 14 PR 65's note
    describes.
  - *Stale snapshot* (the first fetch of each roll, the one `roll.created`
    triggers, returns the roll as it stood before it resolved, delivered 400 ms
    late; later fetches are real): old build **21 failed / 138 passed** (14
    force-roll specs and 7 duel specs, including both named Propose Duel specs,
    "mar: the target spares the loser" and "a tie on the final bout"). This
    variant exists because, on an idle machine, the plain delay does not stop
    the 11 duel specs passing on the old build: the server answers the fetch
    after the final bout has resolved, so there is no stale answer to deliver.
    The stale snapshot is what a busy server hands out in that window.
  - The fixed build was re-run on the rebased branch, with the final code and
    corrected fixtures: **159 passed** for each of the delayed and stale-snapshot
    variants (those specs wait with `expectRollOpen`, which is why this is a
    softer test than the old-build rows).
  - Both throwaway fixtures first failed a few specs on `route.fulfill: Fetch
    response has been disposed` (a test ending while a delayed answer was in
    flight); that was the fixture (a missing `await` inside a try/catch), fixed.
- **Full Playwright suite, undelayed**, on the rebased branch and my own server:
  159 passed. **`make check-fast`** passes (695 Vitest tests, svelte-check 0
  errors); two pre-existing reports, neither from this change: `deadcode` lists
  `middleware.AccountContext`, and svelte-check warns about an unused
  `.type-note` selector in `HelpContent.svelte`.

Not run against this code: `make test-integration`. The change is frontend-only
and the Go side is untouched, so there is nothing for it to exercise.

## Behaviour changes to look at before merging
A player who opens the table while a roll is created and resolved inside one
request (the last Propose Duel bout; a Host Festivity guest roll nobody can
leverage) and whose roll fetch is slow now sees the resolved roll, with the make/
mar choices where they apply, instead of "Dice roll in progress…" until reload.
`demand.leverage_set` now refetches the whole roll rather than only its dice.
Nothing changes for a roll that is not resolving at that moment, and no extra
requests are made unless an event arrives while the roll's own fetch is out.

## Open questions for the owner
None.

## Noticed, not fixed
- **If the post-resolution refetch fails**, the resolved roll and dice (from the
  event) stay on screen but participants and ballot stay empty until reload.
  Left alone: they matter little once a roll has resolved.
- **`loadGameState` (a page resync) can also race the roll fetch**, in both
  directions, and cannot see this guard. Filed as
  `docs/work/next/table-resync-races-roll-fetch.md`.
- **The preparer's own cast-roll response** (`onPlanRollCreated` in
  `MainEventView.svelte`, via `bind:activeRoll`) installs the roll returned by
  the POST. It is a freshly created roll that still needs player action, so it
  can't be older than a `roll.resolved`; left alone.
