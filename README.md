# Uneasy Lies the Head — Server

Go + Svelte web adaptation of the TTRPG. See `../PLANNING.md` and `../PHASE1_SPEC.md` for context.

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
make test               # fast unit tests
make test-integration   # needs Postgres; see "Test database" below
make vet                # go vet, both with and without the integration tag
make check-frontend     # svelte-check on the frontend
make check-fast         # vet + unit + frontend (no DB needed)
make check              # everything, including integration
```

`make check` is the pre-commit gate. Use `make check-fast` if you don't
have Postgres up.

### Test database

When the dev stack is running (`docker compose up -d`) the Postgres
container exposes 5432 on the host. Create a one-time test database
beside your dev one:

```bash
docker exec uneasy-db-1 psql -U uneasy -d postgres \
  -c "CREATE DATABASE uneasy_test;"
```

The Make targets default `TEST_DATABASE_URL` to that local instance.
Override it via `make test-integration TEST_DATABASE_URL=...` or by
exporting the variable.

### Known-failing tests

A handful of pre-existing integration tests fail on a fresh checkout —
mostly individual test bugs (missing FK fields on `CreatePlanToken`,
off-by-one secret-count assertions, etc.) rather than real regressions.
Tracked separately; if `make test-integration` shows red, check whether
your failing tests overlap with this list before assuming it's your
change.

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
| `UNEASY_DEV`   | unset                                                | If `1`, mounts `/api/dev/*` shortcuts (see below) |

## Dev testing workflow

Manual UI testing usually means juggling several "players" at once. With
`UNEASY_DEV=1` (already set in `docker-compose.yml`) the server exposes
two shortcuts that make this painless:

| Endpoint | Purpose |
|---|---|
| `POST /api/dev/login?username=foo` | Find or create the account `foo`, open a session, set the cookie. No code required. |
| `POST /api/dev/reset` | `TRUNCATE accounts, games CASCADE` — wipes all account, session, and game data. Schema is untouched. |

These routes are only mounted when `UNEASY_DEV=1`, so production binaries
will 404 on them.

### A quick note on POST vs GET

Both dev endpoints are `POST`, not `GET`. Typing a URL into the browser's
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

### Resetting between test runs

Same idea — `/api/dev/reset` is also a POST. Either run it from any
browser console:

```js
await fetch('/api/dev/reset', { method: 'POST' });
```

Or from a terminal:

```bash
curl -X POST http://localhost:8080/api/dev/reset
```

Faster than restarting the DB container; preserves the schema so you
don't re-run migrations. After resetting, you'll need to re-run the
dev-login step in each browser profile to seed new sessions.

### Logging in normally

The dev shortcut bypasses the code check, but the regular sign-up /
login flow at `/signup` and `/login` still works in dev. Use it when
you're testing the auth UI itself; use the dev shortcut when you just
need a session and don't care which one.
