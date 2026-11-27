# check-fast in a fresh worktree builds the frontend first

## What changed and why

- **Makefile:** `cmd/server/frontend_dist/_app` is now a file target that runs
  `make frontend`. `check-fast` lists it as its first prerequisite, so it runs
  before `vet`, `deadcode` and `test`. When `_app` exists the target has no
  prerequisites to be stale against, so it does nothing and costs nothing.
- **Why:** `cmd/server/frontend_dist` is gitignored. In a fresh worktree the
  three `cmd/server` tests that read the embedded frontend (`frontend_test.go`
  twice, `warmup_test.go` once) failed with "no immutable .js asset in the
  embedded build", and every session paid for a failed run and a rebuild. CI
  never saw it because its explicit `make frontend` step runs first. That step
  stays as it is.
- **Why a file target, not a shell `if`:** make's own up-to-date check is the
  condition, so there's no recipe logic to get wrong and nothing new to
  document beyond the target name.
- **Not checked:** freshness. A stale `_app` is kept. The tests need some
  immutable asset, not the latest one. After a frontend change you want the
  embedded server to show, `make frontend` (or `make build`) is still needed.
- **docs/DEVELOPMENT.md:** the setup section already says the first
  frontend-tooling target runs `npm ci` for you. A matching paragraph now says
  the embedded build works the same way, and that it isn't rebuilt after a
  frontend edit.

## How it was verified

Against this worktree's own code, with a fresh checkout (no `frontend_dist`
build, `frontend/node_modules` removed before the first run):

- **Before the change**, `make check-fast` failed at `go test` with the three
  `cmd/server` failures above (`frontend_test.go:65`, `frontend_test.go:84`,
  `warmup_test.go:152`). The log is quoted in the PR description.
- **First run after the change**, with no other step first: exit 0. It ran
  `npm ci`, then `make frontend` once, then vet, deadcode, all Go packages
  (`cmd/server` passes), `npm run check`, the Vitest unit suite, the hooks
  tests and the tracker check.
- **Second run:** exit 0, and the log contains no `npm ci`, `npm run build` or
  `make frontend`. The frontend is not rebuilt.
- `python3 docs/work/tracker.py --check` passes after the item file was
  deleted.

Not run: `make test-integration` and `make check`/e2e. This change touches only
the Makefile and docs; no Go code or database code changed, so the integration
suite and the e2e specs exercise nothing different from `main`.

"Prove each new test bites": there is no new test. The before/after pair above
is the evidence the prerequisite is what makes `check-fast` pass.

## Behaviour changes to look at before merging

- `make check-fast` and `make check` now run `make frontend` first in any
  checkout whose `cmd/server/frontend_dist/_app` is missing. That's a one-off
  `npm ci` plus a Vite build (the first run took about 40 seconds of wall time
  here, including `npm ci`).
- A stale `_app` from an earlier build is used as is. This is deliberate, per
  the brief, and is the main thing to know if a Go test ever reads a frontend
  asset that changed since the last build.
- `make clean` removes `_app`, so the next `check-fast` rebuilds.

## Open questions for the owner

None.

## Noticed, not fixed

- `make test` on its own still fails in a fresh worktree, for the same three
  tests. Only `check-fast` (and so `check`) got the prerequisite, because the
  brief asked for that. If anyone runs `make test` alone, they hit the same
  failure. Giving `test` the same prerequisite is a one-line change if the
  owner wants it.
