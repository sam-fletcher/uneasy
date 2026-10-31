# e2e specs off the hard-coded :8090

## What changed and why
- New `frontend/tests/e2e/baseUrl.ts` exports `E2E` (`E2E_BASE_URL`, default `http://localhost:8090`). `playwright.config.ts` and every spec import it, replacing 30+ literal `baseURL: 'http://localhost:8090'` and three local consts. One definition means a new spec can't drift.
- chat.spec's seven `pwRequest.newContext()` calls had no baseURL at all; they now pass `E2E` explicitly rather than relying on how the standalone request factory resolves relative URLs.
- Updated the `test-worktree` skill (dropped the "specs still hard-code 8090" caveat) and `docs/DEVELOPMENT.md`. Tracker item 5 removed, later items renumbered.

## How it was verified
- `grep -rn 8090 frontend/tests frontend/playwright.config.ts` finds only `baseUrl.ts`.
- Full suite (25 tests) passes against docker `server-e2e` (:8090) and against this worktree's own server (`make build`, port 8192, private DB, `E2E_BASE_URL=http://localhost:8192`); the server log showed the dev API traffic. Server stopped, DB dropped.
- Not run: `make check-fast` / integration (no product or Go code touched).

## Behaviour changes to look at before merging
None for the default run.

## Open questions for the owner
None.

## Noticed, not fixed
- `frontend/tests/shot.mjs` defaults to the dev server `:8080` (not an e2e port) and already takes `--base`; left as is. `cmd/plandriver` likewise has `-base`.
