# Uneasy Lies the Head — Server

Go + Svelte web adaptation of the TTRPG. See `../PLANNING.md` and `../PHASE1_SPEC.md` for context.

## First-time setup

You need: **Go 1.22+**, **Node 20+**, **Docker Desktop**, and **golangci-lint**.

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

# Test (none yet — coming in later phases)
go test ./...
```

## Generating typed SQL (sqlc)

The `db/queries/*.sql` files define queries that `sqlc` compiles to typed Go.
For Phase 1, `db/store.go` implements these by hand. When ready to switch:

```bash
# Install sqlc
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Generate — outputs to db/gen/
sqlc generate
```

Then replace the manual functions in `db/store.go` with calls to `db/gen/`.

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

### Running multiple players in parallel

Each browser profile (or Chrome incognito window, or different browser) is
its own cookie jar, so each can hold a different player's session. The
fastest way to seed a session is to visit the dev-login URL directly:

```
http://localhost:8080/api/dev/login?username=alice   # in profile A
http://localhost:8080/api/dev/login?username=bob     # in profile B
```

Then navigate to `http://localhost:8080/profile` in each — you'll see
that account's tables. Create a table in one window, copy the join code,
join from the other.

### Resetting between test runs

```bash
curl -X POST http://localhost:8080/api/dev/reset
```

Faster than restarting the DB container; preserves the schema so you
don't re-run migrations.

### Logging in normally

The dev shortcut bypasses the code check, but the regular sign-up /
login flow at `/signup` and `/login` still works in dev. Use it when
you're testing the auth UI itself; use the dev shortcut when you just
need a session and don't care which one.
