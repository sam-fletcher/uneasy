# E2E: fail a test whose page shows the table's error-boundary fallback

## What changed and why
The table page's `<svelte:boundary>` logs what it catches as `[uneasy:boundary]`
(`logBoundaryError`), but a spec's own assertions can keep passing over the fallback
it draws, which is how the 2026-11-08 crash passed the nightly run. Now any test
whose page logs that line fails, naming the page and the logged error.

- **`frontend/tests/e2e/fixtures.ts`** extends Playwright's `test` with an *auto*
  fixture (it runs for every test, so no spec can forget it). It listens to the
  console of every context a test uses and fails the test, in fixture teardown, if a
  line contains the prefix. The prefix is imported from `src/lib/boundaryError.ts`,
  so renaming it there can't leave the listener behind.
  - A **console listener**, not a DOM scan, as the tracker item asked: it hears the
    line the moment it is logged, so a page the spec closed, or a fallback the
    player dismissed with "Try again", still fails the test.
  - Contexts come from the `page`/`context` fixtures **and** from
    `browser.newContext()` / `browser.newPage()`, which specs and helpers call for
    every extra player. The guard listens to the browser's public `context` event for
    the length of one test (`browser.on` / `browser.off`; the `browser` is shared by
    the whole worker). **That event fires before `newContext()` returns**, so the
    console listener is on before anything can open a page in the context: I checked
    the order in Playwright 1.60 (the server announces the context, then answers the
    call; the event handler ran first on every try) and the canaries cover it.
  - It first wrapped `browser.newContext`/`newPage` instead. That worked, but
    Playwright attributes each API call to the first stack frame outside Playwright,
    so every such call showed in traces, reports and a failed `newContext`'s code
    snippet at `fixtures.ts`, not at the spec line. The event doesn't do that (I
    checked both: a failing `newContext` now shows the spec's own line).
  - A page opened in `beforeAll`/`afterAll` is not watched (outside any one test; no
    spec uses either). `docs/DEVELOPMENT.md` and the file's header say so.
  - `export * from '@playwright/test'` passes everything else through, so a spec's
    change is the module path on its import line.
- **47 files** (all 38 existing specs, plus `helpers.ts` and eight files in `helpers/`) now
  import `test` from `./fixtures` (`../fixtures` in `helpers/`). Only the import
  line changed in each. The files that import only `expect`, `request` or types from
  Playwright were left alone.
- **`src/lib/e2eFixtures.test.ts`**, in `make check-fast`, fails when any file under
  `tests/e2e/` imports `test` straight from Playwright, so a new spec can't skip the
  check. It matches a named import of `test`, a namespace or default import, a
  re-export, **the module under both its names** (`@playwright/test` and
  `playwright/test`, which re-exports it) **and `test` as the default export**
  (`import { default as t }`, `export { default }`). It failed first, listing all 47
  files, before the swap; the matcher has its own cases for each form, and each new
  case fails against the narrower matcher it replaced.
- **`tests/e2e/fixtures.spec.ts`**: six canaries. Five use `test.fail()` to require
  that each way of getting a page fails the test (the `page` fixture,
  `browser.newContext()`, `browser.newPage()`, a second page in one context, a page
  closed before the test ends); one checks an ordinary console error does not.
  If the guard goes blind, they report "Expected to fail, but passed". `test.fail()`
  counts *any* failure as the expected one, so each canary body runs inside
  `unlessGuarded`, which swallows (and prints) the body's own errors: a body that
  breaks for its own reasons now also reads "expected to fail, but passed" instead
  of a false green (checked by pointing one at an unreachable URL).
- A paragraph in `docs/DEVELOPMENT.md` and a sentence in the `test-worktree` skill
  tell spec authors to import from `./fixtures`.
- The tracker item `e2e-fail-on-boundary-fallback.md` is deleted, and the gaps below
  are filed as `docs/work/next/e2e-boundary-guard-gaps.md`.

## How it was verified
All against this branch's own build (`make build`, the production binary the
nightly also uses), on a private database, port 8147. The docker `:8090` stack was
not used.

- **Before the swap**, the unmodified suite: **235 passed** (4.3 min).
- **After the swap**, the same 235 tests: **235 passed** (4.4 min). So **no existing
  spec passes with the fallback showing**: no test-only opt-out was needed, and no
  real crash is filed. (There is deliberately no opt-out mechanism yet; the first
  test that has to provoke the boundary on purpose should add a narrow, named one
  rather than inherit one nobody uses.)
- **Final state**, with the canaries and a clean rebuild: **241 passed** (4.2 min).
  Run again after the review changes (the `context` event, the canary wrappers):
  **241 passed** (4.2 min).
- **The guard fails first and bites.** The unit test was red on 47 files before the
  swap and is green after. With the guard disabled the canaries fail (5 of 6, as
  intended). Removing the `browser.on('context')` listener fails the three canaries
  that open pages with `browser.newContext()`/`newPage()`. Removing the `context`
  override fails none: the event hears the `page` fixture's context too. It stays as
  insurance that the listener exists before that context is made, whatever order
  Playwright sets fixtures up in; the canaries test the outcome for each route.
- **Bite proof with product code**, done twice (once for each version of the guard,
  the `browser` patch and then the event). A temporary `throw` at the top of
  `MainEventView.svelte`'s script (removed; `git diff -- frontend/src` is empty).
  With the old imports (a copy of the suite from `HEAD`), **83 of 235 tests still
  passed** over the crashed phase view (152 failed, from missing UI, in 25.9 min).
  Running the fixture suite on those 83: **40 now fail**, the same 40 both times, with
  `table phase view (main_event) Error: bite proof…`, the page URL and the stack.
  The other 43 never rendered a main-event view (lobby, prologue, Shake-Up, style
  guide, profile pages), so they correctly still pass. Nothing passed that hadn't
  passed before. Spec files hit: `chat`, `public_record`, `seen_across_devices`,
  `connection_notice`, `table_shell`, `retinue_pieces`, `back_dismiss`,
  `make_demands`, `profile_tables`, `spread_rumors`.
- `make check-fast` on the branch rebased onto `main` at `e262af57`: **passed**
  (Go vet and unit tests, svelte-check 0 errors, Vitest 1924 tests, tracker check).

**Did not run:** the e2e suite against the docker stack on `:8090` (it serves
`main`, not this branch), and anything under Vite's dev server: all runs used the
production build. `make test-integration` was not run: no Go changed.

## Behaviour changes to look at before merging
- A test that logs the boundary line now fails in teardown. The failure is
  reported *next to* the test's own error (often a timeout waiting for something the
  fallback hides), so read the fixture's message first: it names the page and the
  thrown error. Under the production build the stack is minified.
- A spec that deliberately provokes the boundary would fail. None does today.
- Every test now also runs the `boundaryGuard` fixture, which depends on `browser`,
  so a test that only used `request` now makes the worker start its browser (it is
  already running for the rest of the suite). The cost per test is one event
  listener on the browser and one console listener per context.

## Open questions for the owner
None.

## Noticed, not fixed
Filed as `docs/work/next/e2e-boundary-guard-gaps.md`:
- The guard hears only `logBoundaryError`. `boundaryError.test.ts` already requires
  every `<svelte:boundary>` to have an `onerror`, but not that it be this one, so a
  future boundary logging some other way would be invisible to the fixture.
- Uncaught page errors (`page.on('pageerror')`) and other `console.error` lines are
  not failed on. Doing so would be the same fixture with a wider net, but it would
  need a pass over what the app logs today to see how noisy it is; left alone as out
  of scope.
- A page opened in `beforeAll`/`afterAll` isn't watched.

Also: the tracker item said 22 specs; there are 47 files to switch now.
