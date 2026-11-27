# Report a render crash to the server

Deletes the item `render-crash-report` (its file `docs/work/next/render-crash-report.md` is removed in this PR).

## What changed and why

- **A table for the crash.** Migration `060_render_crash_reports` adds `render_crash_reports`: the game (nullable),
  the account, the boundary's label, the error's message and stack, the route, and the client's build id. The
  build id is SvelteKit's `version` (`$app/environment`), which changes once per deploy, so the owner can tell which
  deploy a crash came from. FKs are `ON DELETE SET NULL`, the same as `feedback_submissions`: a report is a record,
  so it survives a deleted game or account. `boundary` is an addition to the brief's field list: the route alone
  doesn't say which view threw (`table phase view (main_event)` does).
- **An endpoint shaped like feedback.** `POST /api/render-crashes` (`handler/render_crash.go`) is session-authed
  like `/api/feedback`. The account comes from the session, never the body; logged-out tabs get 401 and nothing
  is stored. Fields go through `textField` with caps, and an empty `boundary` or `message` is a 400.
- **Its own rate limit, keyed on the account.** The limit is a DB count over `render_crash_reports` alone
  (`CountRecentRenderCrashReportsByAccount`), so it never shares a count with `feedback_submissions`. The cap is
  **10 per account per hour**. A component that throws on every render would otherwise file one report per
  re-render. No client-supplied address is involved.
- **Client side.** `lib/renderCrash.ts` (new) builds the report and sends it with `submitRenderCrash`
  (`lib/api/support.ts`). It cuts each field to the server's cap, so the server never refuses a report for length.
  A message that is only whitespace becomes `(no message)`, since the server trims and would refuse it as empty.
  Any failure of the call is swallowed: no console line, no re-report, so a crash in the reporter cannot become
  another crash.
- **A stale or deleted game is not a lost report.** The handler looks up the account's seat in the game it names
  (`GetPlayerByAccountAndGame`). If there is no seat (the game was deleted, or the account was never in it), the
  report is stored with `game_id` NULL. Without this the foreign key would refuse the insert with a 500, and the
  crash would be lost.
- **Wiring.** The table page's `svelte:boundary` onerror now calls `logBoundaryError` (unchanged) and then
  `reportRenderCrash(where, error, game?.id)`. The AST guard in `lib/boundaryError.test.ts` now fails any
  `svelte:boundary` whose onerror does not always call `reportRenderCrash`, so a future boundary cannot forget it. The comment in `lib/boundaryError.ts` that called this an open
  question is replaced: it says the report is a separate call, and why it can't live in that file. The Playwright
  suite imports `boundaryError.ts` and cannot load `$app/environment`.
- **Nothing else is reported.** Transport failures, 4xx and 5xx responses from other calls are untouched; only a
  throw during rendering, which is all a boundary catches, reaches `reportRenderCrash`.

## How it was verified

Against this worktree's own code, on a private database `uneasy_claude_render_crash_report_62a4e3`
(created through the db container, to be dropped after the run). Not the shared `uneasy_test`.

- `make check-fast`: passes. Go unit tests, svelte-check (807 files, 0 errors, 0 warnings), Vitest (120 files, 2415
  tests), the hook tests, and `tracker.py --check`.
- `make test-integration` against the private database: passes (the handler package takes about seven minutes on the
  shared Postgres). The new file is `handler/render_crash_integration_test.go`, with these tests:
  - `WritesThenRateLimited`: writes a report and reads back every field and the account; with the count at the
    limit the next report is 429 and is not inserted.
  - `AllowsUpToTheLimit`, `LimitIsPerAccount`, `NotCountedAgainstFeedback`, `RequiresLogin`,
    `RequiresBoundaryAndMessage`, `OptionalStackAndGame`.
  - `DeletedGameStoresNull`, `UnseatedGameStoresNull`, `SeatedGameIsKept`: the seat check.
- Vitest: `lib/renderCrash.test.ts` (10 tests). They include one POST per crash handed to it, two crashes making two
  calls, the payload fields, the length caps, and that a failed or refused report is swallowed without output.
- **Each test bites.** Frontend: a duplicate `submitRenderCrash` call makes the once-per-crash tests fail (8 of 10
  red). Removing the `reportRenderCrash` call from the table page makes the AST guard fail
  (`every <svelte:boundary>'s onerror calls reportRenderCrash`). Dropping the `.trim()` on the message fails the
  whitespace case. Go: with the rate-limit check disabled, `WritesThenRateLimited` fails; with the seat check
  disabled, `DeletedGameStoresNull` and `UnseatedGameStoresNull` fail. Each restored file passes.
- **Integration suite** on the private database, run again after these fixes: see the PR body.
- **Not run:** `make test-e2e` and a browser check of a real render crash. The table boundary's new onerror body is
  covered by the existing AST guard (`boundaryError.test.ts`), which passes; nothing forces a crash in a live page.
  No preview server was started: the change is not observable without forcing a throw.

## Behaviour changes to look at before merging

- **Every table render crash now makes one authenticated POST** to `/api/render-crashes`. A crash on a page where the
  player is logged out files nothing (401, swallowed).
- **A report for a game the account is not seated in, or one that was deleted, is stored without its game link.**
  The crash itself is kept.
- **Stored text may carry game content.** A thrown message or stack could in principle quote in-game text (secret
  text included, if a view throws while holding it). The rows are read only by the owner, by hand; no other
  player can read them, and nothing in the game queries the table. This is the same exposure feedback accepts.
- **The report is wired in the page, not in `logBoundaryError`.** `logBoundaryError` can't call it (the
  Playwright import problem above). A future `svelte:boundary` must call `reportRenderCrash` as well as
  `logBoundaryError`. The AST guard checks only the second, so a new boundary without the report would pass.
- **No pruning.** Rows accumulate. At 10 an hour per account the table is small, but nothing deletes old rows yet.

## Defaults to change if wrong

- **10 reports per account per hour.** A default, not a question: generous for a real session and small for a
  render loop. It is one constant, `renderCrashRateLimit` in `handler/render_crash.go`.

Not reported to Discord. The owner's decision on client-error reporting (2026-11-26) settles that: client errors are not auto-reported, and render-boundary
crashes go to their own endpoint and table.

## Noticed, not fixed

- `make check-fast` once exited 2 while a second `make test-integration` (another session's, on another database)
  was running on the same Postgres container. A rerun exited 0 with the same code. Not investigated; it may be the
  shared container, not this change.
