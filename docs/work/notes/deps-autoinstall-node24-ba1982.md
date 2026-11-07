# deps-autoinstall-node24-ba1982

## What changed and why

Two environment drifts that made local checks disagree with CI and production.

**1. `npm ci` runs by itself when the lockfile changes.** Nothing reinstalled a
checkout's `frontend/node_modules` after a lockfile bump. The owner's host copy
sat at SvelteKit 2.56.1 / Svelte 5.55.1 / Vite 6.4.2 while the lockfile (and CI, the dev
container, production) were at 2.70.3 / 5.57.3 / 6.4.5, so `make check` tested an older
stack than CI. Some worktrees were stale the same way.

- `Makefile`: a new `frontend-install` target compares a SHA-256 of
  `frontend/package-lock.json` with the one the last install wrote to
  `frontend/node_modules/.installed`, and runs `npm ci` and rewrites the stamp only
  when they differ. `frontend`, `check-frontend`, `test-frontend-unit` and `test-e2e`
  depend on it, so `check-fast` and `check` get it too. `frontend` used to run `npm ci`
  itself; it now depends on `frontend-install` instead, which is how CI's `make
  frontend` then `make check-fast` installs once.
- **The stamp is a content hash, not a timestamp.** My first version was the brief's
  suggestion (a file target depending on the lockfile). Testing it showed a flaw: the
  dev `frontend` container runs `npm install` at every start, which rewrites
  `package-lock.json` with identical bytes and a new mtime, so the host would have run a
  needless `npm ci` after every container restart. Checked in a scratch copy and
  against the real bind mount. The hash ignores the timestamp.
- Only the lockfile is hashed, not `package.json`: `npm ci` fails if the two disagree,
  so a dependency edit always reaches the lockfile first.

**2. One Node major, 24.** Host and CI were on 24; the production `Dockerfile` and the
`frontend` service in `docker-compose.yml` were on 20, so CI did not test the build
that ships.

- `Dockerfile` and `docker-compose.yml`: `node:20-alpine` -> `node:24-alpine`.
- `frontend/.nvmrc` (new) holds `24`. `frontend/package.json` gets
  `"engines": {"node": ">=24"}`, a warning only: no `engine-strict` anywhere.
- `frontend/package-lock.json` gains the matching `engines` line in its root entry.
  This is outside the brief's file list but a direct consequence: without it the
  dev container's `npm install` (it runs `npm install`, not `npm ci`) rewrites the
  lockfile on the owner's disk. See verification below.
- `docs/DEVELOPMENT.md`: Node 24 in the setup list, a new "Frontend dependencies and
  Node" section, and two stale sentences fixed (the "`npm install` is whatever your
  last install left" line, and "if you ever add a CI pipeline").
- `docs/work/next/dependency-upgrades.md`: dropped the bullet saying the Dockerfile must
  leave Node 20, which is now done.

**How Railway builds this repo:** the repo has no `railway.toml`/`railway.json`, no
`nixpacks.toml` and no `Procfile`, only a root `Dockerfile`, which Railway builds when it
finds one. So `main` ships the `Dockerfile` as written. That is inferred from the repo;
I can't see the Railway dashboard, where a builder or Dockerfile path could be set
differently. The runtime image is `alpine` plus the Go binary, so Node is build-time only.

## How it was verified

Against this branch's code:

- **Stamp, in this worktree** (no `node_modules` to start, all re-run on the final
  hash-based version): `make check-frontend` installs, then checks; a second run
  installs nothing; `touch frontend/package-lock.json` installs nothing (timestamp
  only); a real container `npm install` in the bind mount bumps the lockfile's mtime
  with identical content and installs nothing on the host; appending a byte to the
  lockfile installs, and restoring the committed file installs again, then up to date.
  Installed versions matched the lockfile (kit 2.70.3, svelte 5.57.3, vite 6.4.5), and
  `npm ls` shows no extraneous or invalid entries, so the stamp in `node_modules`
  bothers nobody.
- **CI sequence**: deleted `frontend/node_modules`, ran `make frontend` (one `npm ci`,
  build), then `make check-fast`: zero `npm ci` lines, exit 0, 677 unit tests passed,
  and `git diff --exit-code` clean (CI's "no unfixed lint changes" step).
- **Failed install**: with a dead registry `npm ci` empties `node_modules` (stamp gone),
  make exits 2, and the next plain run reinstalls.
- **Production image**: `docker build --no-cache` of the `Dockerfile` (from a clean copy
  of the tree, see "Noticed") exited 0: `node:24-alpine` (Node 24.22.2, npm 11.19.1)
  `npm ci` installed 86 packages, `vite v6.4.5` built, the Go stage built, and the
  final 38.7 MB image's `./server` starts and asks for `DATABASE_URL`. Image removed
  afterwards.
- **Dev compose service on Node 24**: a separate project (`-p ba1982dep`, port 15173,
  fresh volumes, torn down with `down -v`) started the new `frontend` service: Node
  24.22.2, `npm install` then `VITE v6.4.5 ready`, `GET /` 200. Its `npm install` left
  `git status` and the lockfile unchanged.
- **Host `npm ci` against the container's mounts**: a throwaway container bind-mounting
  `frontend/` with a volume on `/app/node_modules` (the dev shape). The host `npm ci`
  emptied the host's `node_modules` (same inode, so the mount point survives); the
  container still saw its volume and neither side saw the other's files. So host
  reinstalls are safe for the running dev stack, as the brief assumed.

Did **not** run against this code: `make test-integration` (no Go code changed), the
Playwright e2e suite, a real Railway deploy, and the default compose project (the
`uneasy-*` containers are untouched and still on Node 20).

## Behaviour changes to look at before merging

- After merge, the first frontend `make` target in each checkout runs `npm ci` once
  (no checkout has a stamp yet, and stale ones are exactly what this fixes). About 3
  seconds with a warm npm cache.
- `make frontend` and `make build` no longer reinstall every time, only when the
  lockfile's contents differ from the last install's.
- `npm install <pkg>` makes the next `make` target run one `npm ci` over it. Intended.
- A new phony target, `make frontend-install`, does just the install check.
- The dev `frontend` container still runs Node 20 until recreated:
  `docker compose up -d frontend`. If it misbehaves (its `node_modules` volume may hold
  packages installed under Node 20), `docker compose down`, `docker volume rm
  uneasy_node_modules`, `docker compose up -d`.
- The next Railway deploy builds the frontend on Node 24.

## Open questions for the owner

None.

## Noticed, not fixed

- **CI could read the pin from the file.** `ci.yml` hardcodes `node-version: 24`;
  `node-version-file: frontend/.nvmrc` would make `.nvmrc` the only place to bump. It is
  a `.github/workflows/` change, outside this brief (another session is editing there).
- **No `.dockerignore`.** `docker build .` from a checkout that has `frontend/node_modules`
  would copy the host's copy over the freshly installed one (`COPY frontend/ ./` follows
  `npm ci`), and `COPY . .` in the Go stage sends the whole tree, including any
  `.claude/worktrees`. Railway builds from a clean clone, so production is unaffected;
  I avoided it by building from a clean copy. Not confirmed beyond reading the Dockerfile.
- **npm 11.19 warns** `install-scripts ... esbuild@0.25.12 (postinstall)` is "not yet
  covered by allowScripts". Harmless today (the build and dev server work), and the host's
  npm 11.11.0 doesn't print it. A later npm may skip install scripts by default, so
  watch it when upgrading Node or npm.
- **Playwright's browser is not versioned by `npm ci`.** A lockfile bump of
  `@playwright/test` still needs `npx playwright install chromium` by hand.
- **The dev container runs `npm install`, not `npm ci`**, so it can drift from the lockfile
  in ways the host's `npm ci` can't.
