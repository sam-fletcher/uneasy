---
name: test-worktree
description: Run the app, the integration suite, or the Playwright e2e specs against the code in a git worktree (anything under .claude/worktrees/) instead of the main checkout. Use before reporting worktree work as verified, whenever you need a browser or an API to hit a worktree's own code, and when choosing which database a test run may use.
---

# Testing a worktree's own code

The docker stack does **not** run your worktree. The `server` (`:8080`),
`server-e2e` (`:8090`) and `frontend` containers bind-mount the **main
checkout**, so anything that talks to those ports exercises `main`'s code,
whichever folder you run the command from. A test that hits them from a
worktree either passes for the wrong reason or fails for one.

What does and doesn't need a private server:

| Check | Tests your worktree as-is? |
|---|---|
| `make check-fast` (lint, Go unit, svelte-check, Vitest) | **Yes** — runs in-process from your folder |
| `make test-integration` | **Yes** — in-process — but mind the database (below) |
| `make test-e2e`, `cmd/plandriver`, `frontend/tests/shot.mjs` at their default ports | **No** — they hit the containers, i.e. `main` |

## Databases

`uneasy_test` is shared by everyone: the integration suite **TRUNCATEs** it,
and the `server-e2e` container serves it. Two sessions using it at once
corrupt each other's runs. So in a worktree, always use a private database
named after your branch, and drop it when you're done.

There is no `createdb` on the host; create and drop through the db container:

```bash
DB=uneasy_$(git branch --show-current | tr -c 'a-z0-9\n' '_')
docker exec uneasy-db-1 psql -U uneasy -d postgres -c "CREATE DATABASE $DB;"
# … run things …
docker exec uneasy-db-1 psql -U uneasy -d postgres -c "DROP DATABASE $DB WITH (FORCE);"
```

Migrations apply themselves when the server starts and when the integration
harness connects, so a fresh empty database is fine.

## Integration suite on a private database

```bash
make test-integration TEST_DATABASE_URL="postgres://uneasy:uneasy@localhost:5432/$DB?sslmode=disable"
```

## Serving the worktree (for e2e, plandriver, shot.mjs, a browser)

`make build` builds the frontend and embeds it in `./server`, so you need no
Vite. Pick a port nobody else is using (8100–8199; not 8080/8090). Run the
server in the background so it outlives the command:

```bash
make build
UNEASY_DEV=1 PORT=8192 \
  DATABASE_URL="postgres://uneasy:uneasy@localhost:5432/$DB?sslmode=disable" \
  ./server
```

Then point every tool at it:

- **e2e:** `cd frontend && E2E_BASE_URL=http://localhost:8192 npx playwright test [spec]`
- **plandriver:** `go run ./cmd/plandriver setup <plan> -base http://localhost:8192`
- **shot.mjs:** `node tests/shot.mjs --base http://localhost:8192 --game <id> --as <player>`

Every spec, helper and the Playwright config take their base URL from
`E2E_BASE_URL` (via `frontend/tests/e2e/baseUrl.ts`), defaulting to `:8090`. A new
spec must import `E2E` from there rather than writing a URL, or it will quietly
test `main` against a private server (`grep -rn 8090 frontend/tests` should
find only `baseUrl.ts`).

Stop the server and drop the database when you finish. Never leave a server
running on a port another session might pick.

## Reporting

Say exactly which checks ran against *your* code and which didn't — "e2e
passed" means nothing if it ran against `:8090`.
