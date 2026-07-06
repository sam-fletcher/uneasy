# Development Guide

Go + Svelte web adaptation of the TTRPG. This document covers everything
you need to set up, run, and test the project locally. For the project
pitch and license, see the [top-level README](../README.md); for running
and administering a live instance, see [OPERATIONS.md](OPERATIONS.md).

## First-time setup

You need: **Go 1.26+**, **Node 20+**, **Docker Desktop**, and **golangci-lint**.

```bash
# Install golangci-lint (dev linter)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install Air (Go hot-reload)
go install github.com/air-verse/air@latest

# Download Go dependencies
go mod download

# Install frontend dependencies
cd frontend && npm install && cd ..
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
E2E specs can call `/api/dev/reset` freely without disturbing it.

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

Note: `make test-integration` and `make test-e2e` both target
`uneasy_test`, so don't run them concurrently. `make check` runs them
sequentially, which is fine.

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
The production `Dockerfile` already does this. If you ever add a CI pipeline,
use `npm ci` there too. `npm install` is fine for local dev (adding new
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
| `UNEASY_DEV`   | unset                                                | If `1`, mounts `/api/dev/*` shortcuts (see below) and Go profiling at `/debug/pprof/*` |
| `PUBLIC_ORIGIN`| unset                                                | Public URL the server is reachable at, e.g. `https://uneasy.example`. Unset = dev behavior (cookies without `Secure`, no HSTS, WebSocket accepts any Origin). When set with an `https://` scheme: session cookies get `Secure`, responses get HSTS, and the WebSocket handshake only accepts that host as Origin. |

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
| `POST /api/dev/seed` | Create a game in a given phase (`main_event` / `shake_up`) with named players, each holding one asset of every type. See the `DevSeed` doc comment for the body shape. |
| `POST /api/dev/advance-row` | Jump a game's `current_row` — `{"plan_id":N}` (to that plan's row) or `{"game_id":N,"row":R}`. Lands you on a prepared plan's resolution row without clicking through the rows between. |
| `POST /api/dev/delete-game` | `{"game_id":N}` — hard-delete a single game and all its data (cascade), leaving accounts and other games intact. The everyday cleanup tool. |

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
