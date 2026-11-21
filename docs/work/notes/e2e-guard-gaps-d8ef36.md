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
  `frontend/tests/e2e/allowedConsoleErrors.ts`: eight entries, each with a message pattern,
  the spec it belongs to (required), a reason, and, for the three that stand for one faked
  failure (chat's failed send, make_war's failed refresh, connection_notice's aborted
  `/state`), the one test that makes the line. (The brief said "in `fixtures.ts`"; it is a module
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
  **A delete made in the middle of a test must not start that quiet period**, or the rest of
  the test would go unchecked, so `deleteGame(request, id, { pagesOpen: false })` skips it.
  `deleteDuel` uses it: it runs inside `duelWithDice`'s reseed loop, with its contexts already
  closed, so all eight `duelWithDice` specs keep their `console.error` check. (My first version
  called plain `deleteGame` there and would have silenced them; the review caught it.)
- **`helpers/make_demands.ts` and `helpers/propose_duel.ts`** deleted their games with a raw
  POST, which bypassed that. They now call `deleteGame` (three lines). One unit test now
  fails any file under `tests/e2e/` other than `helpers.ts` that names `/api/dev/delete-game`.
- **`src/lib/boundaryError.test.ts`**: every `<svelte:boundary>`'s `onerror` must now *call*
  `logBoundaryError`, and the component must import it from `boundaryError`. `onerror={() => {}}`,
  `console.error(error)`, a handler defined elsewhere, and a same-named local function all fail.
  So does a call that may never run: inside an `if`, after a `&&`, in a ternary, or in a function
  that is only defined. The call must be the arrow's expression body or a top-level statement of
  its block (a `return` of it counts). The matcher is its own function with eleven cases, and
  the failure message says the call must be inline.
- **`src/lib/e2eFixtures.test.ts`** (in `make check-fast`) checks the allow-list itself: at most
  ten entries, each with a reason, a spec that still exists (and always one: with none an entry
  would allow its line in every spec), for a scoped entry a test title the spec still has, no
  pattern that matches a generic line (a bare `Failed to fetch`, a 500 or 403 on some other
  route, …), and, for a bug entry, a tracker item that still exists (so the entry has to leave
  with the fix). It also checks the matcher; that the cross-spec canary's line really is allowed
  for chat's test and nowhere else; what the failure report prints; and that the fixture still
  listens for `weberror` and `console.error`.
- **`tests/e2e/fixtures.spec.ts`**: canaries went from 6 to 18. New: an uncaught exception from
  a timer on the `page` fixture and on a `browser.newContext()` page, an unhandled promise
  rejection, an uncaught throw of a string (not an Error), a `console.error` not on the list
  (two routes), a line allowed for chat's test that must still fail here (the same test title,
  so only the spec file differs), an allow-listed line that must *not* fail, `deleteGame` ending
  the counting (a stub stands in for the endpoint) and `{ pagesOpen: false }` leaving it on, the
  quiet period after `tableDeleted()`, and a boundary line and an exception that still fail
  after it. The old canary "an ordinary console error does not fail the test" is
  gone: it is no longer true.
- **`tests/e2e/guardReport.ts`** holds the failure-message code (moved out of `fixtures.ts` so the
  unit suite can test it). A thrown value that is not an Error (`throw 'x'`) reaches the guard with
  an empty `stack` and the value as the `message` (checked: a string, a number and `null` arrive
  that way, and an object arrives as just "Object"), so the report now uses `stack ||` message;
  the first version's `??` printed a blank line and folded every such throw on a page into one.
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
  unmutated control passed (14 canaries then; 18 now, see the review round).
- **The new unit checks fail first.** The tightened `boundaryError.test.ts`: against the real table
  page with `onerror={() => {}}` and with `onerror={(error) => console.error(error)}` it goes red
  naming `routes/table/[id]/+page.svelte:1124`, while the old test (`HEAD`'s copy) stays green on
  both. The allow-list checks: a misspelt spec, an item file that is gone, a one-word reason, a
  catch-all pattern and an eleventh entry each turn exactly their own test red; and the
  only-`deleteGame` test named both raw-POST helpers before they were fixed.
- `make check-fast`: **passed** (Go build, vet, lint and unit tests; svelte-check 742 files, 0 errors;
  Vitest 103 files, 1997 tests; the git-guard cases; the tracker check). `tsc` over the project
  includes `tests/e2e/` and reports nothing for it.

### The review round (what the reviewer found, and what was checked)
The review of PR #116 found three real faults and a few smaller ones, and all are fixed:

1. **`deleteDuel` silenced `console.error` for the rest of the test** (`duelWithDice` reseeds
   mid-test, in eight specs). Fixed with `deleteGame(…, { pagesOpen: false })`. Two canaries now
   pin it directly: plain `deleteGame` ends the counting (a stub stands in for the endpoint), and
   `{ pagesOpen: false }` leaves it on. Removing the option's check turns the second red; removing
   the `tableDeleted()` call turns the first red.
2. **The boundary matcher accepted a call that may never run** (`if (x) log()`, `x && log()`,
   `const later = () => log()`, a ternary). Those four are now rejected. They were red against the
   old matcher and are green with the stricter one, and the real table page still passes.
3. **A thrown string printed a blank line** (`stack ?? message` with Playwright's empty `stack`).
   Now `||`, in `guardReport.ts`, with unit tests that fail against `??` (two red), and a canary
   that throws a string.
4. **`spec` is required** on every entry (the type, the matcher and a unit test), and the narrowness
   test now rejects a bare `Failed to fetch`, a 500, 403, 404 or 400 on some other route, and a
   script error, for every entry. Each fails exactly its own test when I plant a bad entry.
5. **`demand-prepared-plan-id.md` names the second emitter** (the counter-demand synthesis in
   `handler/plan_make_demands_counter.go`, which sends `plan.prepared` and `demand.prepared` with the
   same payload), and says the fix and its check must cover both.
6. **A cross-spec canary:** chat's faked 500 is allowed for chat's own test, and must still fail a
   test in another spec. Its title is built from chat's entry, so only the spec file differs; the
   first draft passed even with spec-matching removed, because the per-test scope blocked it too,
   which I found by that mutation and fixed. A unit test pins that the canary's line really is
   allowed for chat's test and nowhere else.

Judgement calls: I scoped the three entries that stand for one faked failure to their test
(adopted; the unit test checks each title still exists in its spec), renamed the guard's kind
`pageerror` to `uncaught` since it labels a `weberror`, and pinned `deleteGame` → `tableDeleted()`
directly by canary. **Not adopted:** a time limit on a hanging canary body. I tried it, and it
turns out that in Playwright 1.60 a timeout under `test.fail()` is reported as a real failure
("Test timeout of 30000ms exceeded", the suite goes red), not as the expected one, so a hang cannot
make a canary pass. The limit was removed and the comment says why.

**Runs after the review changes, and what interrupted them.** The Mac slept in 15-minute stretches
while these ran (the power log shows "Maintenance Sleep" and dark wakes of about a minute, 16 minutes
apart, with the lid open; the app's `turn_end` keep-awake hold was in place and a longer
`session_idle` hold was denied by the auto-mode classifier). A test in flight at each sleep timed out
on wall-clock, so two plain `npx playwright test` runs were not usable (one failed four tests, one
five, all `Test timeout of 30000ms exceeded`; the five rerun alone passed).

So the full run on the final tree was done **spec file by spec file**, each file in its own Playwright
invocation, with any run during which the Mac slept (wall-clock elapsed far above monotonic elapsed)
thrown away and run again. Every test therefore ran once with no suspension inside it; it is not a
single Playwright invocation. Result: **39 spec files, 254 passed, 1 failed** (of 255: the 243 plus 12
new canaries), 5 interrupted attempts discarded, 72 min of wall time and 6.9 awake.
`propose_duel.spec.ts` (11 passed, covering the `duelWithDice` reseeds) and `fixtures.spec.ts` (18)
passed inside it.

**The one failure was a real guard message, not a timeout**, and it is the flake exposure this note
warns about: `public_record.spec.ts` › "Events: a war the server stored as "make" is not shown as a
Make" logged a `404` on `GET /api/plans/N/war-state`. That route answers 404 "no war exists yet for
this plan" by design until a war is declared, and `MakeWarPanel.svelte`'s `refreshWar` (on mount and
on every war or plan event) treats it as "no war yet — fine", so nothing is wrong on screen; the
browser still logs a red 404. The server's request log shows the two fetches finishing before the
test's own `POST /resolve` returned (the page hears the plan's event mid-resolve and asks before the war
exists), so it is a timing race: the test had passed in three full runs, and passed 20 of 20 repeats
afterwards. Decided with the coordinator: an allow-list entry for that one test, naming a tracker
item for the app wart (`next/make-war-state-404.md`: skip the fetch until a war can exist, or change
the answer), so the entry leaves with the fix. No pre-emptive entry for `make_war.spec.ts`, the other
spec that could trip it. Proof that the entry works: with the page forced to fetch `war-state` before
the resolve (a temporary one-line edit, removed), the test **passes with the entry and fails without
it**, with the same guard message as the original failure; a unit test pins that the line is allowed
for that test and for no other test or spec, and the "tracker item still exists" check bites.

After the entry, on the branch rebased onto `main` at `51ed3ba1` (PR #115, which changed frontend
components and added a `host_festivity.spec.ts` test, no Go) and a fresh `make build`:
`fixtures.spec.ts` **18 passed**, `public_record.spec.ts` **8 passed**, `host_festivity.spec.ts`
**10 passed** (including main's new test, now under the guard), none interrupted; and
`make check-fast` **passed** (svelte-check 743 files, 0 errors; Vitest 103 files, 2006 tests; the
git-guard cases; the tracker check). No further full run was needed, since the entry only narrows
what fails; `make test-integration` was not run (no Go changed, on this branch or on main since).

**Did not run:** the e2e suite against docker `:8090` (it serves `main`, not this branch) and
anything under Vite's dev server. The after runs are not a soak: a `console.error` that appears only
sometimes needs more runs to show (this one showed on the fifth full pass).

## Behaviour changes to look at before merging
- **A new `console.error` in any spec now fails it**, in teardown, with a message that says what to
  do. The nightly can go red for something that is not a crash. That was the point of the brief, but
  it does raise flake exposure: the `reaim` failure above was exactly such a line, and it showed on
  one of three runs. If the nightly shows another intermittent one, the first question is whether
  it is teardown (route the delete through `deleteGame`) before it is a new allow-list entry.
- **`console.error` stops counting once a test calls `deleteGame`.** A real error logged in that
  window (after the game is deleted) is missed. Boundary lines and exceptions still count.
- The allow-list is capped at ten by a unit test (eight today: seven for the app's own specs, one for the canary).
- `propose_duel`'s helper used to ignore the delete's status; `deleteGame` warns on a non-2xx.
  `deleteDuel`, the one delete made mid-test, keeps its test's `console.error` check on
  (`pagesOpen: false`). The other deletes (all in `afterEach`) start the quiet period, which is
  the point.

## Open questions for the owner
None. `console.error` is on, the list is short, and there is no question to file.

## Noticed, not fixed
- **Filed:** the Make War panel's `war-state` fetch logs a console 404 before a war exists
  (`next/make-war-state-404.md`; allow-listed for the one `public_record` test that tripped it).
  The server has a second 404 for the same condition, `mwLoadWar` in `handler/plan_make_war.go`.
  A `200` answer would change the response's shape for tabs left open across a deploy, so the item
  prefers skipping the fetch.
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
