# Development Guide

Go + Svelte web adaptation of the TTRPG. This document covers everything
you need to set up, run, and test the project locally. For the project
pitch and license, see the [top-level README](../README.md); for running
and administering a live instance, see [OPERATIONS.md](OPERATIONS.md).

## First-time setup

You need: **Go 1.27+**, **Node 24**, **Docker Desktop**, and **golangci-lint**.
`frontend/.nvmrc` pins the Node major (`nvm use`, `fnm use` and similar read it);
see [Frontend dependencies and Node](#frontend-dependencies-and-node).

```bash
# Install golangci-lint (dev linter), pinned to the version in the Makefile.
# Use this rather than `go install` or Homebrew: both compile the linter
# against whatever Go they happen to have, and a linter built against an
# older Go panics outright on a newer stdlib.
make lint-install

# Install Air (Go hot-reload). Only needed to run the server outside
# Docker — `docker compose up` installs Air inside the container.
go install github.com/air-verse/air@latest

# Download Go dependencies
go mod download

# Frontend dependencies need no step of their own: the first `make` target
# that runs frontend tooling (make check-frontend, make check-fast, ...) runs
# `npm ci` for you, and runs it again whenever the lockfile changes.
```

Both `go install` and `make lint-install` drop binaries in `$(go env GOPATH)/bin`,
which macOS does **not** put on `PATH` by default. The Makefile calls the linter
by absolute path so it doesn't care, but if you want to run these tools directly
from a shell, add that directory to your `PATH`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
```

## Running in development

```bash
docker compose up
```

This starts three services:
- **db** — Postgres on port 5432
- **server** — Go API on port 8080 (with Air hot-reload)
- **frontend** — Vite dev server on port 5173 (with HMR)

Open http://localhost:8080. The Go server proxies all non-API requests to Vite,
so you get a single origin with hot module replacement on the frontend.

**The `frontend` container keeps its own `.svelte-kit/`.** `./frontend` is
bind-mounted into the container, so without care the container and your host
would share `frontend/.svelte-kit/` (SvelteKit's generated files). They don't
share a SvelteKit *version*, though: the container installs the lockfile's, and
your host's `node_modules` can lag behind it until a `make` target refreshes it
(see [Frontend dependencies and Node](#frontend-dependencies-and-node)). Any host
command that loads SvelteKit's Vite plugin — `make test-frontend-unit` (so
`make check-fast` and `make check`), `vite dev`, `vite build` — rewrites the
generated files in its own version's format, and the container's Vite then
answers every page with a 500. So `/app/.svelte-kit` is a named volume
(`svelte_kit`, like `node_modules`), and host and container never touch each
other's. After pulling a change to `docker-compose.yml`, recreate the
container once with `docker compose up -d frontend`.

`npm ci` and `npm install` both run `svelte-kit sync` (the `prepare` script), so a fresh
clone gets the `.svelte-kit/tsconfig.json` that `frontend/tsconfig.json`
extends without needing the container.

## Go workflow

```bash
# Lint
golangci-lint run ./...

# Build (without Docker)
go build ./cmd/server
```

## Running tests

The suite is split into two layers:

- **Unit tests** — no DB, no env. Just `make test` (or `go test ./...`).
- **Integration tests** — guarded by the `integration` build tag. They talk
  to a real Postgres and TRUNCATE all user tables between cases, so they
  must point at a dedicated test database — never a dev or prod one.

Common commands:

```bash
make test               # fast Go unit tests
make test-integration   # needs Postgres; see "Test database" below
make test-frontend-unit # Vitest unit tests for pure-TS lib/* modules
make test-e2e           # Playwright end-to-end; needs the dev stack up
make vet                # go vet, both with and without the integration tag
make check-frontend     # svelte-check on the frontend
make check-fast         # vet + unit + frontend (no DB needed)
make check              # everything, including integration and e2e
```

`make check` is the pre-commit gate. Use `make check-fast` if you don't
have Postgres up.

### Test database

The integration and e2e suites run against a dedicated `uneasy_test`
database (kept separate from the dev `uneasy` DB so tests can reset data
freely without touching your manual games). On a **fresh Postgres volume**
it's created automatically by `db/init/01-create-test-database.sql`, which
the image runs on first boot — so a `docker compose down -v` followed by
`docker compose up` recreates it for you.

If you have a pre-existing volume (created before that init script), create
it once by hand:

```bash
docker exec uneasy-db-1 psql -U uneasy -d postgres \
  -c "CREATE DATABASE uneasy_test;"
```

The Make targets default `TEST_DATABASE_URL` to that local instance.
Override it via `make test-integration TEST_DATABASE_URL=...` or by
exporting the variable.

### End-to-end tests (Playwright)

`make test-e2e` drives a real Chromium browser against a dedicated Go
server (`server-e2e` in `docker-compose.yml`) listening on port 8090 and
bound to the `uneasy_test` database. Your manual testing session on
`:8080` uses the dev `uneasy` database and is completely untouched —
E2E specs can seed and delete games (`/api/dev/seed`, `/api/dev/delete-game`)
freely without disturbing it.

**First-time setup** (one-time, after `npm ci`):

```bash
cd frontend
npx playwright install chromium   # ~90MB browser binary, not in node_modules
```

**Running:**

```bash
docker compose up -d          # stack must be running
make test-e2e
```

Specs live in `frontend/tests/e2e/`. Playwright config:
`frontend/playwright.config.ts`.

**Preflight.** Before any spec runs, `frontend/tests/e2e/globalSetup.ts`
fetches the base URL once. A healthy app passes silently. If the app is
unreachable, hangs for 15s, or returns a non-2xx status, the whole run stops
within about a second with one message (the status, the first line of the
response, and what to try), instead of ~150 specs each timing out at 30s. It
reads the same `E2E_BASE_URL` as the specs, so it checks a worktree's own
server when you point at one.

Note: `make test-integration` and `make test-e2e` both target
`uneasy_test`, so don't run them concurrently. `make check` runs them
sequentially, which is fine.

**`make test-e2e` cannot validate a worktree branch.** Both `server-e2e` and
`frontend` bind-mount `.` — the *main* checkout — into the container, so they
serve whatever is in your primary working tree regardless of which worktree
you run the command from. Run it from a branch in `.claude/worktrees/` and you
will be testing `main`'s code against your branch's specs, which either passes
for the wrong reason or fails for one. The Go integration tests have no such
problem: they exercise the handlers in-process, so they do test your branch.

To run e2e against a worktree, serve it yourself from its own binary and a
scratch database (never `uneasy_test`); every spec and `playwright.config.ts` read
`E2E_BASE_URL` (default `http://localhost:8090`, set in
`frontend/tests/e2e/baseUrl.ts`). The step-by-step recipe lives in one place, the
`test-worktree` skill (`.claude/skills/test-worktree/SKILL.md`).

#### The nightly run on GitHub Actions

CI (`ci.yml`) gates pull requests on `make check-fast` and the integration
suite, and does not run Playwright. So
[.github/workflows/e2e-nightly.yml](../.github/workflows/e2e-nightly.yml) runs
the whole e2e suite once a night (06:17 UTC) against the latest commit on
`main`. It never blocks a merge, and `main` is already live by the time it
runs: its job is to catch a stale spec or a bad merge within a day rather
than whenever someone next runs `make test-e2e` by hand. Whether to also gate
PRs on it is still open (`docs/work/questions/ci-e2e-cadence.md`).

The job does what the worktree recipe above does, on a clean runner: `make
build` (the production binary with the frontend embedded, so no Vite), starts
`./server` on `:8090` against a throwaway Postgres container, waits for it to
answer, installs Chromium, then runs `npm run test:e2e` with `CI=true`. The
suite runs serially with **no retries**, deliberately: for the first weeks the
point is to see how flaky it really is, and a retry would hide that.

**What a failure looks like.** The run goes red on the Actions tab. A
*scheduled* failure also opens one GitHub issue, "Nightly e2e run failed on
main", labelled `nightly-e2e`, with the run link, the commit, the step that
failed, the failing spec names and a compare link from the last passing
night. Later failures comment on that same issue rather than opening more,
and the first passing night closes it. Manual runs and the PR self-test never
open or close issues.

**Reading the artifacts.** On a failure the run page's *Artifacts* section
(kept 14 days) has `e2e-failure`, containing:

- `frontend/test-results/<spec>/trace.zip`, one per failed spec. Unzip the
  artifact and, from `frontend/`, run `npx playwright show-trace <trace.zip>`
  to step through the page, the network calls and the console as the test saw
  them.
- `frontend/test-results/report.json`, Playwright's JSON report (the source of
  the spec names in the issue).
- `server.log`, the Go server's output for the whole run.

**Reproducing it locally.** Check out the commit named in the issue in a
worktree and follow the `test-worktree` recipe (private database, `make
build`, your own server, `E2E_BASE_URL`), running just the failing spec.
That is the same server, binary and browser the nightly used, minus the
runner. Add `CI=true` to get the same reporters.

**Running it by hand.** Actions tab → *e2e-nightly* → *Run workflow* (pick a
branch), or `gh workflow run e2e-nightly.yml --ref <branch>`. Editing the
workflow file in a PR also runs it once on that PR, as a self-test.

### Test health

`make test` and `make test-integration` should both pass clean on a fresh
checkout (against a dedicated `uneasy_test` database). If `make
test-integration` shows red, treat it as a real failure to investigate —
there is no longer a list of expected/known-failing tests to discount.

## Generating typed SQL (sqlc)

The `db/queries/*.sql` files define queries that `sqlc` compiles to typed Go.

```bash
# Install sqlc
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

make sqlc      # regenerates db/gen/ from queries + migrations
```

## Project layout

```
cmd/server/   — entry point (main.go)
cmd/hashpw/   — one-off bcrypt-hash generator for manual password resets
db/           — migrations, sqlc query files, manual store, migration runner
handler/      — HTTP handlers (one file per resource)
hub/          — WebSocket hub: Manager + Hub + Client
middleware/   — HTTP middleware (cookie auth)
model/        — shared data types and WebSocket message constants
frontend/     — SvelteKit SPA (Svelte 5, adapter-static, ssr: false)
```

## Frontend dependencies and Node

**Node 24 everywhere.** `frontend/.nvmrc` pins it; the production `Dockerfile`'s
frontend stage, the dev `frontend` container and CI (`node-version` in
`.github/workflows/ci.yml`) all use the same major, so CI tests the Node that
ships. `engines` in `frontend/package.json` says `>=24`: npm warns on an older
Node but does not refuse (`engine-strict` is deliberately not set). A move to a
new Node major means editing each of those places, plus `engines`, together.

**`node_modules` follows the lockfile by itself.** Every `make` target that runs
frontend tooling (`frontend`, `check-frontend`, `test-frontend-unit`, `test-e2e`,
and so `check-fast` and `check`) first runs `make frontend-install`. That
compares a hash of `frontend/package-lock.json` with the one the last `npm ci`
left in `frontend/node_modules/.installed`. When they differ — after a `git
pull` that bumped a dependency, in a fresh worktree, or after you delete
`node_modules` — it runs `npm ci`; when they match, nothing happens. Each
worktree has its own `node_modules`, so each installs once. Before this, nothing
refreshed a checkout's `node_modules` after a lockfile bump, so `make check`
could test an older SvelteKit than CI did.

Things worth knowing:

- Force a reinstall with `rm frontend/node_modules/.installed`.
- It is the lockfile's contents that count, not its timestamp. The dev
  container runs `npm install` every time it starts, which rewrites
  `package-lock.json` with the same bytes and a new timestamp; that does not
  trigger a host reinstall.
- `npm install <package>` changes the lockfile, so the next `make` target runs
  one `npm ci` over it. That is intended: the copy you test then matches what
  CI installs.
- The dev containers are otherwise unaffected. A host `npm ci` empties
  `frontend/node_modules` but leaves the directory in place, so the `frontend`
  container keeps seeing its own `node_modules` volume. After a Node major
  changes, recreate that container with `docker compose up -d frontend`. If it
  then misbehaves (the volume can hold packages built under the old Node), run
  `docker compose down`, `docker volume rm uneasy_node_modules`, then
  `docker compose up -d`.

## Frontend dependency security

npm supply chain attacks are a real and active threat (the axios package was
compromised by a North Korea-nexus actor in March 2026 via a hijacked
maintainer account). Three practices are enforced in this project to limit
exposure:

**1. `frontend/.npmrc` sets `save-exact=true`.**
Any `npm install <package>` you run will write an exact version to
`package.json` (e.g. `"vite": "6.0.7"`) rather than a caret range
(`"vite": "^6.0.7"`). Caret ranges allow silent patch-level updates on a
fresh install; exact pins don't.

**2. `package-lock.json` must be committed to git.**
Never add it to `.gitignore`. The lockfile is the actual version lock.
`npm ci` — used in the production Dockerfile — reads the lockfile exactly
and fails if `package.json` and the lockfile are out of sync. This means
what you develop locally is byte-for-byte what gets deployed.

**3. Use `npm ci`, not `npm install`, in any non-interactive context.**
The production `Dockerfile` and CI already do this, and the `Makefile` does it
for you (see above). `npm install` is fine for local dev (adding new
packages, etc.) but `npm ci` is appropriate anywhere the build should be
reproducible.

**Adding a new package:**
```bash
cd frontend
npm install <package>   # .npmrc ensures exact version is saved
# commit both package.json AND package-lock.json
```

**Checking for a compromised transitive dependency:**
```bash
cd frontend
npm ls <package-name>   # e.g. npm ls axios
```

## Environment variables

| Variable       | Default                                              | Notes                          |
|----------------|------------------------------------------------------|--------------------------------|
| `DATABASE_URL` | required                                             | Postgres connection string     |
| `PORT`         | `8080`                                               | HTTP listen port               |
| `DEV_MODE`     | `false`                                              | Proxy frontend to `VITE_URL`   |
| `VITE_URL`     | `http://localhost:5173`                              | Vite dev server address        |
| `UNEASY_DEV`   | unset                                                | If `1`, mounts `/api/dev/*` shortcuts (see below) and Go profiling at `/debug/pprof/*`, and (like `DEV_MODE`) logs one `db trace` line per API request — `queries=N trips=T db_ms=X`, where `trips` is the serial chain of round trips, the number that decides latency against the remote production database (`middleware/querytrace.go`) |
| `PUBLIC_ORIGIN`| unset                                                | Public URL the server is reachable at, e.g. `https://uneasy.example`. Unset = dev behavior (cookies without `Secure`, no HSTS, WebSocket accepts any Origin). When set with an `https://` scheme: session cookies get `Secure`, responses get HSTS, and the WebSocket handshake only accepts that host as Origin. |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` | unset | Web-push signing keypair (adr/NOTIFICATIONS_PLAN.md). Unset in dev: turn notifications are logged to stdout instead of sent. Generate once with the one-off snippet below. |
| `VAPID_SUBJECT`  | unset | Contact info sent to the push service (a `mailto:` address or an `https://` URL), e.g. `mailto:you@example.com`. |

One-off VAPID keypair generation — save as a scratch `main.go` inside `uneasy/` (it already depends on `webpush-go`) and `go run` it once:

```go
package main

import (
	"fmt"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		panic(err)
	}
	fmt.Println("VAPID_PUBLIC_KEY=" + pub)
	fmt.Println("VAPID_PRIVATE_KEY=" + priv)
}
```

## Health check & database backups

`GET /healthz` returns `200 ok` without touching the database — point hosting
platform health checks here. Keep it DB-free: a health check that queries
Postgres would keep a scale-to-zero database (e.g. Neon) awake around the
clock.

[.github/workflows/db-backup.yml](../.github/workflows/db-backup.yml) takes a
weekly encrypted `pg_dump` of the production database and stores it as a
GitHub Actions artifact (90-day retention). It skips cleanly until two
repository secrets are configured — set these at deploy time:

| Secret | Purpose |
|---|---|
| `PROD_DATABASE_URL` | Postgres connection string for the production DB |
| `BACKUP_PASSPHRASE` | Encrypts dumps (artifacts are visible to anyone who can see the repo) |

Restore instructions are in a comment at the top of the workflow file.

## Dev testing workflow

Manual UI testing usually means juggling several "players" at once. With
`UNEASY_DEV=1` (already set in `docker-compose.yml`) the server exposes a
few shortcuts that make this painless:

| Endpoint | Purpose |
|---|---|
| `POST /api/dev/login?username=foo` | Find or create the account `foo`, open a session, set the cookie. No code required. |
| `POST /api/dev/seed` | Create a game in a given phase (`main_event` / `shake_up` / `prologue_closing`) with named players, each holding one asset of every type. `prologue_closing` parks the game at the prologue's "The Stage is Set" step with no Public Record board yet (so an all-ready advance runs cleanly). Every seeded game also gets one sample law + rumor so the laws/rumors UI isn't blank — pass a `"laws"`/`"rumors"` array to override (`[]` for none). See the `DevSeed` doc comment for the body shape. |
| `POST /api/dev/advance-row` | Jump a game's `current_row` — `{"plan_id":N}` (to that plan's row) or `{"game_id":N,"row":R}`. Lands you on a prepared plan's resolution row without clicking through the rows between. |
| `POST /api/dev/force-roll` | `{"roll_id":N,"outcome":"mar"}` (or `"make"`, or an exact `"result":K`) — force an open dice roll to a chosen outcome and resolve it through the real path. The dice are genuinely random, so without this the **mar** branch of every rolled plan — half of its resolution UI — can't be reached on demand. |
| `POST /api/dev/crown` | `{"game_id":N,"player_id":N}` — seat a monarch at a seeded table the way the Prologue's title claim does: a new peer ("The Crown") owned by that player, bearing the monarch title, with the throne-established gate tripped. Broadcast live and posted to the log. For testing a table with a reigning monarch (Propose Decree's signatory and council). |
| `POST /api/dev/take-asset` | `{"asset_id":N,"player_id":N}` — move an asset to a player the way a plan's take does (secret visibility, `asset.taken` log line, main-character gate). Players have no raw take route. |
| `POST /api/dev/tear-marginalia` | `{"asset_id":N,"position":1-4,"player_id":N}` — tear one marginalia the way a plan's break does (destroys the asset on the last one, logs it). Players have no raw tear route. |
| `POST /api/dev/delete-game` | `{"game_id":N}` — hard-delete a single game and all its data (cascade), leaving accounts and other games intact. The everyday cleanup tool. |

`force-roll` works *with* the production resolution path rather than around
it: it pre-sets the die faces (`rollAndCancelDice` keeps a face that is
already set — the same hook Propose Duel's bout dice use), then runs the real
stage machine, so `roll.resolved`, the chat post, `applyAutoChoiceOnRoll` and
the row-state recompute all fire and a watching browser updates live. It
refuses what it can't deliver: a mar at difficulty 1, or a result above the
actor's dice count. Note that it overwrites *every* face on the roll — forcing
a Propose Duel final roll discards the dice tucked under the staked assets, so
force a duel by picking its bouts instead.

There is deliberately **no** "wipe everything" endpoint: that was too easy
to fire by accident. To wipe your entire local dev database (always
manually, never via an AI tool), use:
`docker exec uneasy-db-1 psql -U uneasy -d uneasy -c "TRUNCATE accounts, games RESTART IDENTITY CASCADE;"`
— or `docker compose down -v` for a full volume reset (note: that also drops
`uneasy_test`, which the init script then recreates on the next `up`).

These routes are only mounted when `UNEASY_DEV=1`, so production binaries
will 404 on them.

### A quick note on POST vs GET

The dev endpoints are `POST`, not `GET`. Typing a URL into the browser's
address bar always sends a `GET`, so visiting
`http://localhost:8080/api/dev/login?username=alice` directly will return
`405 Method Not Allowed` (or 404). You need to actually issue a `POST`.
The two easiest ways are shown below — pick whichever you prefer.

### Running multiple players in parallel

Each browser profile (Chrome incognito window, Firefox container tab, or
a different browser entirely) is its own cookie jar, so each can hold a
different player's session.

**Option A — from the browser's DevTools console (recommended).**
This is the most convenient way, because the cookie the server sets in
the response is automatically stored by the browser you're already using.

1. Open the browser profile you want to be "alice."
2. Navigate to `http://localhost:8080` (any page on that origin works —
   the cookie is scoped to the origin, not the path).
3. Open DevTools (`Cmd+Option+I` on macOS) and click the **Console** tab.
4. Paste and run:

   ```js
   await fetch('/api/dev/login?username=alice', { method: 'POST' });
   ```

   You should see a `Response` object logged with `status: 200`. The
   session cookie is now set in this profile.
5. Navigate to `http://localhost:8080/profile`. You should be logged in
   as alice.

Repeat in a second profile/incognito window with `username=bob`. Now you
have two browsers logged in as different players. Create a table in
alice's window, copy the join code, join it from bob's.

**Option B — from a terminal with curl.**
Useful for scripting, but slightly more steps because you have to move
the cookie into the browser yourself:

```bash
curl -X POST -c cookies.txt http://localhost:8080/api/dev/login?username=alice
```

`-X POST` sets the method; `-c cookies.txt` saves the response cookies
to a file. Open `cookies.txt` and find the `session` (or similar) cookie
value, then in your browser's DevTools go to **Application → Cookies →
http://localhost:8080** and add it manually. Option A is almost always
faster.

### Logging in normally

The dev shortcut bypasses the code check, but the regular sign-up /
login flow at `/signup` and `/login` still works in dev. Use it when
you're testing the auth UI itself; use the dev shortcut when you just
need a session and don't care which one.


### Driving a plan to its resolution (`cmd/plandriver`)

Setting a plan's *resolution* up by hand is a dozen requests, and each plan
wants a different prepare payload. `plandriver` does it one way for all
twelve, so two people looking at the same plan see the same board.

```bash
go run ./cmd/plandriver setup make_demands -force mar
go run ./cmd/plandriver setup chronicle_histories
go run ./cmd/plandriver force 301 mar     # once a pre-roll plan has a roll
go run ./cmd/plandriver cleanup 301
```

It seeds a three-player main-event game, logs everyone in, prepares the plan
with the fields that plan actually validates, fast-forwards the record to its
row, and prints the table URL.

Only Make Demands creates its roll the moment resolution opens, so `-force`
works directly. The other eleven open a pre-roll step first and have no dice
yet: drive that step — through the UI, or over the plan's own extra routes
(`sp-cast-roll`, `sr-cast-roll`, `seek-cast-roll`, …) — then run `force`. Make War and
Clandestinely Liaise have no row at all until their simultaneous delay reveal
is answered. The tool says which kind you got.

A few things it encodes so you don't rediscover them:

- **Preparing a plan auto-passes focus** to the next player, and only the
  focus player may prepare. That is why Make Demands takes two turns: one
  player lays down a target plan (handing focus on), the next demands it. A
  raw `advance-row` does *not* rotate focus.
- Exchange Courtiers needs **both** `target_player_id` and `target_asset_id`.
- A dev-seeded retinue holds exactly one peer — the main character — so
  peer-targeting plans trip the MC-replacement gate unless you add one. The
  tool warns when it has had to fall back to an MC.

### Screenshotting a table as a given player (`frontend/tests/shot.mjs`)

The session cookie is per-origin, so one browser is one player: seeing what
*another* player sees means logging in again, which replaces the cookie in
every tab at once. Each run of `shot.mjs` launches its own Chromium with its
own cookie jar instead, so any number can run at once and nobody becomes
anybody else.

```bash
cd frontend
node tests/shot.mjs --game 331 --as alice --label sp-01-mar-options
node tests/shot.mjs --game 331 --as bob   --label sp-01-bob-view --width 1280
node tests/shot.mjs --game 331 --as carol --text-only
```

Writes `<out>/<label>.png` (full page, 375px wide by default — mobile first)
and prints the page's visible text and any console errors. The text is
usually enough to tell which state you landed in.

`--click <text>` is available for client-side UI state (expanding a section,
opening an overlay). Anything the server records should be driven over the
API instead: every run starts from a fresh context, so a click sequence would
be replayed — and re-submitted — each time.

Needs the Chromium binary (`npx playwright install chromium`, the same
one-off the e2e suite needs).

### Example: jumping to Plan preparation

In the console: 

```js
// 1. Become alice in THIS window
await fetch('/api/dev/login?username=alice', { method: 'POST' });

// 2. Seed a 2-player main-event game (each player gets one asset of every type)
await (await fetch('/api/dev/seed', {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ phase: 'main_event', players: ['alice', 'bob'] })
})).json();   // ← note the game_id it logs

// 3. Jump that game to a resolution row
await (await fetch('/api/dev/advance-row', {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ game_id: 1, row: 9 })   // or { plan_id: N }
})).json();
```
