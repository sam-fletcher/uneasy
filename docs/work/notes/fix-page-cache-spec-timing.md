# fix-page-cache-spec-timing

## What changed and why

The nightly e2e run of 2026-11-14 (run #10, `2cbbdc7`) failed on one spec of
204: `page_cache.spec.ts › returning to a page you just left paints from the
snapshot, not the network`, with 1984ms against a 300ms budget (issue #92).

The app was fine. The run's trace shows `page.click()` itself took ~1957ms,
all of it in Playwright's "waiting for element to be visible, enabled and
stable" step, which needs animation frames. The table then painted 26ms after
the click landed. The spec started its clock *before* the click, so the click's
own actionability wait counted as paint time. The same window shows the
profile's revalidate responses taking 1.55s instead of ~10ms and the server's
access log stamping three unrelated requests as finished at the same instant,
which points at a ~1.5–1.9s stall of the CI runner. The cause of that stall is
not proven; the artifact carries no host metrics.

The spec now:

- times only the paint: a trial click does Playwright's waiting untimed, then
  the clock starts and a forced click dispatches. Both timed clicks (Home, then
  the table card) use it.
- waits on the page's own in-flight `/api` requests (`trackApi` / `settled`)
  instead of `waitForLoadState('networkidle')`. In the trace the second
  `networkidle` wait returned immediately while the profile revalidate was
  still in flight, so the next click raced it.

The 300ms budget and the 400ms injected latency are unchanged.

## How it was verified

Spec-only change: the app code on this branch is identical to `main`'s, so
runs against the `server-e2e` container (`:8090`, main's code) exercise the
same app. Run against `:8090`:

- 15 consecutive runs of the new spec: 15 passed.
- Recreated last night's failure by animating the card for 1.5s before the
  click (temporary copies of both versions, deleted): the old spec failed with
  1874ms, the new spec passed.

Run against a private server on this worktree's own build (port 8192, private
database, both removed afterwards):

- With `readTableSnapshot` temporarily made to return `null`, the new spec
  fails at the return-visit assertion (1318ms). The spec still catches a broken
  snapshot. The app change was reverted.

`make check-fast` passes. Not run: the integration suite (no Go or SQL
changed) and the rest of the e2e suite.

## Behaviour changes to look at before merging

None for players. The spec's pass/fail behaviour changes only in that a slow
`click()` or an unsettled load no longer counts as a slow paint. A real stall
*during* the paint would still fail it, deliberately.

## Open questions for the owner

None.

## Noticed, not fixed

- `chat.spec.ts`, `main_event_seed.spec.ts` and `resilience.spec.ts` also use
  `Date.now()`; I did not check whether any of them bracket a `click()` the
  same way.
- The nightly's one stall was not explained. If time-budget specs flake again
  on CI, add a cheap host probe (e.g. log event-loop lag around each spec) so
  the next trace says whether the runner stalled.
