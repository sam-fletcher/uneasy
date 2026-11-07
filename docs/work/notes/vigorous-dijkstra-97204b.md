# Three stale e2e specs (host festivity, make demands, seek answers)

## What changed and why

Three Playwright specs failed on a healthy stack because they were stale, not
because the product was broken. Test-only change; no product code.

- **host_festivity.spec.ts, closing post.** `festivityLog()` returns the post
  bodies as an array, and the spec matched the closing line with an exact
  element `.includes`. Commit bf29f8c made closing posts end "Next: <who> to
  <what>.", so the body is no longer exact. Now `.some(l => l.includes(...))`,
  as the neighbouring assertion already did.
- **host_festivity.spec.ts, "Set the scene" (found on the full run).** The
  same commit makes the closing outcome read "…drew to a close. Next: alice to
  set the scene.", so the substring `getByText('Set the scene')` also matched
  the outcome text and failed strict mode. Now `{ exact: true }`, which matches
  only the step label.
- **make_demands.spec.ts, banner copy.** Commit 8026241 changed "…came to
  nothing here" to "…had no effect here". The expectation now matches the
  shorter substring "drew leverage control, but Propose Duel has no roll of
  its own", so a copy edit to the tail of the line won't break it again.
- **seek_answers.spec.ts, bystander race.** Bob's secret was written straight
  after `openTables()`, before carol's WebSocket had registered, so
  `secret.created` reached no captured frame and the absence check passed
  vacuously. Added a poll for `presence.snapshot` in carol's frames before bob
  writes.
- **spread_propaganda.spec.ts, same race.** Same guard on bob's frames, plus an
  explicit `expect(bobFrames.length).toBeGreaterThan(0)`, so the absence check
  can't pass against an empty capture.

## How it was verified

Against this worktree's own server (`make build`, then `UNEASY_DEV=1
PORT=8192 ./server` on a private database `uneasy_claude_vigorous_dijkstra_97204b`),
with `E2E_BASE_URL=http://localhost:8192`. Nothing ran against `:8090`.

- Full `npx playwright test`: **159 passed (3.6m)**, exit 0.
- The four touched specs before the full run: host_festivity 9/9 after the
  "Set the scene" fix (the first run had one failure there); make_demands,
  seek_answers and spread_propaganda all passed.
- `make check-fast`: passed (677 Vitest tests, lint, tracker check).
- `make test-integration` on a separate private database
  (`…_it`, dropped afterwards): passed. No Go code changed, so this is a
  regression check only.
- Not run: `make test-e2e` at its default port (it hits `main`'s containers,
  so it is not a check of this branch).
- "Prove the test bites" was not repeated for the guards; the presence-snapshot
  guard was diagnosed and confirmed beforehand (the brief's 3.0s pass), and the
  full run passing shows the spec now waits for carol's socket.

## Behaviour changes to look at before merging

None. Test files only.

## Open questions for the owner

- Should the e2e suite gate PRs, or run on a schedule on `main`? Written up in
  `docs/work/questions/ci-e2e-cadence.md`.

## Noticed, not fixed

- The e2e suite runs in no CI job, so these three specs went stale unnoticed.
  Tracked in `docs/work/next/ci-runs-e2e.md`.
- Assertions matching free-text copy (here "Set the scene", the banner
  wording, "came to nothing") break on every copy edit. A broader pass to
  prefer exact or data-driven matches is worth doing, but it's out of scope.
