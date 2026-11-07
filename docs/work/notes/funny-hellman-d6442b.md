# Keep the host and the Vite container apart; fail the e2e run fast

## What changed and why

`make test-e2e` once passed 13 of 159 specs, with ~146 slow 30s timeouts. The
cause was environmental, not the product: the `frontend` container
bind-mounts `./frontend`, so it shared `frontend/.svelte-kit/` with the host.
The host's `node_modules` was at an older SvelteKit than the container's, and
a host `vitest run` (`make check-fast` runs one) rewrote
`.svelte-kit/generated/server/internal.js` in the older format. The
container's Vite then returned a 500 for every page. Two changes, one per
half of that:

- **`.svelte-kit` is a named volume** (`docker-compose.yml`, `svelte_kit`,
  mounted at `/app/.svelte-kit`, same pattern as `node_modules`). Host and
  container now each have their own generated files, so no host command can
  break the container's Vite.
- **`"prepare": "svelte-kit sync || echo ''"`** in `frontend/package.json`, so
  `npm install`/`npm ci` creates `.svelte-kit/tsconfig.json` (which
  `frontend/tsconfig.json` extends) on a fresh clone without the container's
  help. The `|| echo ''` is SvelteKit's own template guard: `npm ci` can run
  where `svelte.config.js` isn't copied yet (the production Dockerfile's first
  stage), and the guard keeps a failed sync from failing the install.
- **E2E preflight** (`frontend/tests/e2e/globalSetup.ts`, wired in
  `playwright.config.ts`): fetches `E2E` once before any spec and aborts the
  whole run with one message if the app is unreachable, hangs for 15s, or
  returns non-2xx (status plus the first line of the body). Silent when
  healthy. The hint text depends on whether `E2E_BASE_URL` is set: docker
  commands for the default `:8090`, "start your own server" for a worktree's.
- `docs/DEVELOPMENT.md` describes the volume and the preflight.

The message does not mention the shared `.svelte-kit`: after this change it
cannot be the cause, once the container is recreated.

## How it was verified

Nothing touched the owner's `uneasy-*` containers or their ports `:5173`,
`:8080` and `:8090`; everything below used scratch ports and scratch compose
project names, torn down afterwards.

- **The volume, before and after** (the real failure, reproduced). Two scratch
  copies of `frontend/` outside the repo, each with its host `node_modules` at
  SvelteKit 2.56.1 (the owner's host version) and a container at 2.70.3 (the
  lockfile), one under `main`'s compose file and one under this branch's, as
  separate compose projects. Both containers served 200. After the host ran
  `vitest run` (what `make check-fast` does): the `main` container returned
  **500** with the owner's exact `TypeError … Received
  '\x00virtual:__sveltekit/environment'`, and its host and container
  `internal.js` had the same checksum (shared). This branch's container still
  returned **200**, with different checksums (separate).
- **`docker compose config`** is valid and shows `svelte_kit` mounted at
  `/app/.svelte-kit`.
- **`prepare` and the production build.** `docker build --target
  frontend-builder` on the real `Dockerfile` (the stage where `svelte.config.js`
  isn't copied yet when `npm ci` runs): `npm ci` printed "No Svelte config file
  found … using default configuration", exited 0, and `npm run build`
  succeeded. `npm ci` in this worktree creates `.svelte-kit/tsconfig.json`. CI's
  `make frontend` runs `npm ci` with the config present, the easier case.
  `Dockerfile.dev` doesn't run npm. The dev container's `npm install` runs the
  hook with the config present.
- **Preflight**, run through `playwright test` (message shown in the PR body):
  closed port → aborts in 1.0s; a stub returning 500 → aborts in 0.6s with the
  status and first body line; a real Vite 500 (corrupted generated file on a
  scratch Vite) → aborts in 0.7s showing `RollupError: Parse failure…`; the same
  Vite once restored → passes silently. A server that accepts and never answers
  → aborts after the 15s timeout, with the "restart it" hint rather than "start
  it" (called directly, as is the docker wording for the default base, so the
  owner's `:8090` was never queried).
- **Healthy path, full suite.** `make build`, then the worktree's own server on
  `:8192` with a private database, `E2E_BASE_URL=http://localhost:8192 npx
  playwright test`: **159 passed (2.1m)** with the preflight wired in.
- `make check-fast`: passed (677 Vitest tests, svelte-check, lint, tracker).

Not run against this code: `make test-e2e` at its default port (it would hit
`main`'s containers), `make test-integration` (no Go code changed), and the
recreated `uneasy-frontend-1` container itself, since I was told not to
recreate it. No unit test for the preflight: its logic is a fetch plus message
formatting, and Vitest only collects under `src/` while a `.test.ts` under
`tests/e2e/` would be picked up by Playwright.

## Behaviour changes to look at before merging

- **After merging, the owner must recreate the frontend container once:**
  `docker compose up -d frontend`. Until then the container still runs the old
  mounts and the old failure can recur. (The `sync_main` hook will say "the dev
  stack's own config changed", as it does for any compose edit.)
- The first start after that is slower: the new volume is empty, so Vite
  regenerates `.svelte-kit` (a few seconds).
- `make test-e2e` now stops immediately with a message when the stack is down
  or broken, where it used to run to the end of every timeout.

## Open questions for the owner

None.

## Noticed, not fixed

- The owner's host `frontend/node_modules` is at SvelteKit 2.56.1, the lockfile
  says 2.70.3. The volume makes that harmless for the container, but
  `cd frontend && npm ci` on the host would stop `make check-fast` testing
  against a different SvelteKit than CI and production use. Out of scope here,
  by the brief.
- `frontend/tests/shot.mjs` and `cmd/plandriver` have no equivalent preflight;
  they fail on the first request, which is already fast.
