# E2E: the guard also fails on uncaught exceptions and unexpected console errors

## What changed and why
`frontend/tests/e2e/fixtures.ts` failed a test only when the table's error boundary
logged `[uneasy:boundary]`. A throw that escapes every boundary (an event handler, an
`$effect`, a promise nobody awaits) leaves no fallback and no boundary line, only a
half-working page that a spec can pass over. Now the same fixture fails a test for three
things, and the failure message has one section per kind that happened:

- **The boundary line**, as before, unchanged.
- **An uncaught exception** (`context.on('weberror')`, the context-wide form of
  `pageerror`). Always a failure, with **no** allow-list.
- **A `console.error` that is not on the allow-list.** The allow-list is
  `frontend/tests/e2e/allowedConsoleErrors.ts`: seven entries, each with a message pattern,
  the spec it belongs to, and a reason. (The brief said "in `fixtures.ts`"; it is a module
  the fixture imports, because it is plain data and a matcher and so the unit suite can
  check its entries without loading Playwright.)

Other pieces:
- **`deleteGame` (`helpers.ts`) calls `tableDeleted()`**, which ends the guard's `console.error`
  counting for that test. Why: a spec deletes its game in `afterEach` while its pages are
  still open; the pages' socket drops, their refetch on reconnecting gets a 403/404 from a
  table that no longer exists, and Chrome logs each as a red console line. That is the
  teardown, not the app, and it was 64 of the 86 lines recorded. I checked the order in the
  server's own request log: `POST /api/dev/delete-game`, then the 403s, then the sockets
  closing. Boundary lines and exceptions still count after the delete.
- **`helpers/make_demands.ts` and `helpers/propose_duel.ts`** deleted their games with a raw
  POST, which bypassed that. They now call `deleteGame` (three lines). One unit test now
  fails any file under `tests/e2e/` other than `helpers.ts` that names `/api/dev/delete-game`.
- **`src/lib/boundaryError.test.ts`**: every `<svelte:boundary>`'s `onerror` must now *call*
  `logBoundaryError`, and the component must import it from `boundaryError`. `onerror={() => {}}`,
  `console.error(error)`, a handler defined elsewhere, and a same-named local function all fail.
  The matcher is its own function with nine cases.
- **`src/lib/e2eFixtures.test.ts`** (in `make check-fast`) checks the allow-list itself: at most
  ten entries, each with a reason, a spec that still exists, a pattern that rejects an
  unrelated line, and, for a bug entry, a tracker item that still exists (so the entry has to
  leave with the fix). It also checks the matcher and that the fixture still listens for
  `weberror` and `console.error`.
- **`tests/e2e/fixtures.spec.ts`**: canaries went from 6 to 14. New: an uncaught exception from
  a timer on the `page` fixture and on a `browser.newContext()` page, an unhandled promise
  rejection, a `console.error` not on the list (two routes), an allow-listed line that must
  *not* fail, the quiet period after `tableDeleted()`, and a boundary line and an exception that
  still fail after it. The old canary "an ordinary console error does not fail the test" is
  gone: it is no longer true.
- The failure `Error` now has an empty `stack`. Playwright prints every stack frame it finds in
  a message a second time at the end, which was already doubling the single boundary line and
  would have scrambled a three-section message.
- `docs/DEVELOPMENT.md` and the `test-worktree` skill describe the three things and the
  allow-list.
- **Tracker:** `next/e2e-boundary-guard-gaps.md` is deleted. Its `beforeAll`/`afterAll` part is left
  as documented (the header of `fixtures.ts` and `DEVELOPMENT.md` now also say what to do if a spec
  ever uses them), as the brief asked. One real bug is filed:
  `next/demand-prepared-plan-id.md`.

## The recording run: what the suite logs today
The fixture first only *recorded* `weberror` and `console.error` (to a file, nothing failing),
and the whole suite ran on this branch's own build: **243 passed**, 4.6 min.

**0 uncaught exceptions. 86 `console.error` lines**, all Chrome's own network lines, WebSocket
refusals, or the old canary's. Every distinct message, with its decision:

| Count | Message (URL normalised) | Specs | Page | Decision |
|---|---|---|---|---|
| 26 | 403 `GET /api/tables/N/laws` | 9 specs | `/table/N` | Harness: teardown |
| 25 | 403 `GET /api/tables/N/rumors` | 8 specs | `/table/N` | Harness: teardown |
| 3 | 403 `GET /api/tables/N/wars` | chronicle_histories, profile_tables, propose_decree | `/table/N` | Harness: teardown |
| 2 | 403 `GET /api/tables/N/plans` | chronicle_histories | `/table/N` | Harness: teardown |
| 2 | 403 `GET /api/tables/N/activity` | profile_tables, propose_decree | `/table/N` | Harness: teardown |
| 1 + 1 | 403 `GET /api/tables/N/assets`, `.../plan-tokens` | chronicle_histories | `/table/N` | Harness: teardown |
| 1 | 404 `GET /api/plans/N` | chronicle_histories | `/table/N` | Harness: teardown |
| 2 | 404 `GET /api/plans/N/war-state` | make_war | `/table/N` | Harness: teardown |
| 1 | 500 `GET /api/plans/N/war-state` | make_war | `/table/N` | Harness: teardown (see Noticed) |
| 9 | `WebSocket connection to 'wss://127.0.0.1:PORT/' failed … ERR_CONNECTION_REFUSED` | connection_notice | `/table/N` | **Intended** |
| 2 | `net::ERR_FAILED` `GET /api/tables/N/state` | connection_notice | `/table/N` | **Intended** |
| 2 | 500 `POST /api/tables/N/posts` | chat | `/table/N` | **Intended** |
| 1 | 500 `GET /api/tables/N/wars` | make_war | `/table/N` | **Intended** |
| 4 | 401 `GET /api/accounts/me` | styleguide | `/dev/styleguide` | **Intended** |
| 3 | 400 `GET /api/plans/undefined` | make_demands | `/table/N` | **Real app bug**, filed |
| 1 | `some other error, nothing to do with the boundary` | fixtures | `about:blank` | The old canary |

The decisions, one by one:

- **Teardown (64 lines): fixed in the harness**, as above. Not allow-listed: a pattern for "403 on
  `/laws`" would also have hidden a real "not a member" bug in a live test.
- **`connection_notice` WebSocket refusals and `/state` abort: intended.** The spec points the page's
  socket at a dead port and aborts the first `/state` fetch after the reconnect, to test the
  notice. Two entries, scoped to that spec.
- **`chat` 500 on `POST /posts`: intended.** The spec fulfils it with a 500 to check that a failed
  send restores the text. Server log shows no such request.
- **`make_war` 500 on `GET /wars`: intended.** The spec fulfils it (a failed refresh must not reopen
  a closed sheet). Not the same lines as the `war-state` 500 above, which the server really sent.
- **`styleguide` 401 on `/accounts/me`: intended.** The page is opened logged out, and the app's own
  `getMe` treats the 401 as the answer ("not a failure").
- **`make_demands` 400 on `/plans/undefined`: a real app bug.** `demand.prepared` carries
  `{ plan: {…} }` and the client reads `plan_id`, so every client at the table fetches
  `/api/plans/undefined`, and the fallback to reloading all plans hides it. I confirmed it with a
  throwaway probe on that test: three `demand.prepared` frames with the plan nested, then three
  requests. Filed as `next/demand-prepared-plan-id.md`. Its allow-list entry names that item, so
  the suite stays green and the bug stays visible; a unit test fails if the item file is deleted
  while the entry remains.
- **The old canary:** replaced, as above.

## How it was verified
All against this branch's own production build (`make build`), a private database and port 8163
(cwd of the listener checked). The docker `:8090` stack was not used.

- **The suite, before and after, on the clean build.**
  - Before (recording only, nothing new failing): **243 passed**, 4.6 min.
  - After, first full run: **250 passed, 1 failed**: `reaim.spec.ts` › "make demands: the demand
    offers the plans it can still be aimed at…", a 403 on `/laws` and `/rumors`. Same teardown
    noise as above, in a path I had left alone (`helpers/make_demands.ts` deleted the game with a
    raw POST before the spec's own `afterEach` closed its pages). It had not shown in the
    recording run: it depends on timing, which is why the fix is at the one place a game is
    deleted rather than a list of lines. Fixed by routing the two helpers through `deleteGame`,
    and by the unit test that stops a raw delete coming back.
  - After the fix: **251 passed** (4.8 min) and, on a fresh `make build` of the final tree
    (the temporary throw removed), **251 passed** again (4.1 min). The 251 are the 243 plus 8
    new canaries.
- **The guard bites on a real page.** A temporary
  `$effect(() => { if (game) throw new Error('bite proof: …') })` in
  `routes/table/[id]/+page.svelte`, outside the `<svelte:boundary>` (removed; `git diff -- frontend/src`
  is `boundaryError.test.ts` and `e2eFixtures.test.ts` only). Same crashed build, both suites:
  - `main`'s suite (the fixtures from `HEAD`, extracted with `git archive`): **243 passed**, 4.1 min.
    Nothing noticed.
  - The new suite: **214 failed, 37 passed**, 6.2 min. All 214 failures are the uncaught-exception
    section naming `Error: bite proof: $effect outside every boundary`, the page URL and the
    (minified) stack; none has a console.error or boundary section. The 37 passes are 12 canaries
    that pass by failing as required, and 25 tests that never load a game on the table page
    (profile, style guide, API-only). Before, the same 214 passed over the crash.
- **Each canary goes blind when its detection is removed.** Removing the `weberror` listener turns
  4 canaries red ("Expected to fail, but passed"); removing the `console.error` branch, 2; ignoring
  the allow-list, the allow-listed canary; ignoring `tableDeleted()`, the quiet-period canary. The
  unmutated control is 14 passed.
- **The new unit checks fail first.** The tightened `boundaryError.test.ts`: against the real table
  page with `onerror={() => {}}` and with `onerror={(error) => console.error(error)}` it goes red
  naming `routes/table/[id]/+page.svelte:1124`, while the old test (`HEAD`'s copy) stays green on
  both. The allow-list checks: a misspelt spec, an item file that is gone, a one-word reason, a
  catch-all pattern and an eleventh entry each turn exactly their own test red; and the
  only-`deleteGame` test named both raw-POST helpers before they were fixed.
- `make check-fast`: **passed** (Go build, vet, lint and unit tests; svelte-check 742 files, 0 errors;
  Vitest 103 files, 1997 tests; the git-guard cases; the tracker check). `tsc` over the project
  includes `tests/e2e/` and reports nothing for it.

**Did not run:** the e2e suite against docker `:8090` (it serves `main`, not this branch), anything
under Vite's dev server, and `make test-integration` (no Go changed). The after runs are three full
runs, not a soak: a `console.error` that appears only sometimes would need more to show.

## Behaviour changes to look at before merging
- **A new `console.error` in any spec now fails it**, in teardown, with a message that says what to
  do. The nightly can go red for something that is not a crash. That was the point of the brief, but
  it does raise flake exposure: the `reaim` failure above was exactly such a line, and it showed on
  one of three runs. If the nightly shows another intermittent one, the first question is whether
  it is teardown (route the delete through `deleteGame`) before it is a new allow-list entry.
- **`console.error` stops counting once a test calls `deleteGame`.** A real error logged in that
  window (after the game is deleted) is missed. Boundary lines and exceptions still count.
- The allow-list is capped at ten by a unit test (seven today: six for the app's own specs, one for the canary).
- `propose_duel`'s helper used to ignore the delete's status; `deleteGame` warns on a non-2xx. Nothing
  else about those deletes changed.

## Open questions for the owner
None. `console.error` is on, the list is short, and there is no question to file.

## Noticed, not fixed
- **Filed:** `demand.prepared` makes every client fetch `/api/plans/undefined`
  (`next/demand-prepared-plan-id.md`). The e2e suite exercised only that one event from the
  group that reads `plan_id`; the other 28 were not audited against their Go payloads.
- `GET /api/plans/N/war-state` answered **500** ("could not build war state", `no rows in result
  set`) once, in the moment the game was deleted underneath a page. The handler had found the plan
  and then lost the war row. Where the plan itself is gone it correctly answers 404. Probably only a
  deleted-game race, but a "not found" that is a 500 is worth a look in the war-state handler.
- Every logged-out visitor's console gets a red 401 from `/accounts/me`, which the client treats
  as the ordinary answer. Harmless; a 200 with no account would keep it out of the console.
- The server logs `level=ERROR msg="writePump: close connection" … use of closed network
  connection` for every client that drops (a hundred or so per suite run), which reads as a fault.
  Out of scope here (it is the Go server's log, not the browser's).
- `helpers/make_demands.ts` and `propose_duel.ts` have their own `cleanup…AfterEach`, separate from
  `cleanupGameAfterEach`. Not touched beyond the delete call.
