# Ops alerts: database failing, backup overdue

Branch `claude/ops-alerts-fd589e`. Closes the tracker item
`ops-monitoring-gaps` (file deleted).

## What changed and why

`/healthz` is database-free on purpose, so a database outage is invisible to the
uptime pinger, and a switched-off backup workflow can't report itself. Two
Discord alerts (the same webhook as feedback) now cover those gaps. New package
`ops/`:

- **Database failing / recovered.** A pgx tracer (`ops/tracer.go`) on the pool
  config sees every query and every connection attempt, so there is one central
  place and no per-handler code. `ops/dbfailure.go` classifies errors (server
  trouble such as refused connection, EOF, SQLSTATE classes 08/53/57/58 vs normal
  results such as no rows, constraint violations, cancelled requests) and
  `DBMonitor` rate-limits: one message at the first failure, at most one an hour
  after that, one "recovered" on the first success. Any error while *opening a
  connection* counts (a quota refusal or bad login is still an outage). It never
  queries the database. In dev the tracer forwards to the existing per-request
  statement counter, so that keeps working.
- **Backup overdue.** Migration 059 adds `ops_backup_status` (one row, enforced
  by a `CHECK (id)` boolean primary key). The last step of `db-backup.yml`
  upserts `now()` into it with the existing `PROD_DATABASE_URL`. `ops.BackupWatch`
  runs inside the existing 30-minute notify tick and does real work only on ticks in
  the 12:00 UTC hour, once per day (a fixed hour, because an in-memory "24 hours
  since last check" resets on every redeploy and would repeat the message): it
  posts if the row is more than 8 days old, at most one message a day. No
  row yet means quiet. It adds **no** database wake-ups, because the tick already
  queries every 30 minutes (so there was nothing to ask the owner about).
- `handler.PostOpsAlert` posts in a goroutine under the existing 5-second
  timeout, so Discord can never slow a request. `NotifyDiscord`'s HTTP code was
  moved into a shared `sendDiscordContent` unchanged.
- Docs: `docs/OPERATIONS.md` ("Alerts" section) and `docs/DEVELOPMENT.md`.

Decisions of mine, not ruled on:

- **Flap damping.** Failure messages are limited to one an hour in total (not
  one per outage), and a "recovered" message is sent only if the owner's last
  message was a failure. Without this, a database that flaps would post every
  flap.
- **A cancelled or timed-out request** counts as neither failure nor success
  (a slow request is not a down database). Only connection attempts treat
  timeouts as failures.
- **The alert text includes the (trimmed, 300-char) error**, which for a
  connection failure can name the database user and host. The channel is
  private.

## How it was verified

- `ops/*_test.go`: fake clock and fake poster cover first failure, hourly
  repeat, recovery, flapping, concurrency, error classification, tracer routing,
  and the stale-backup check (no row, fresh, 9 days, exactly 8 days, once-a-day
  gating, read error retries, recovers). A real pgx pool pointed at a closed port
  proves the tracer fires on "connection refused".
- Each new test was shown to bite: removing the hourly limit, the
  "told-before-recovery" guard, the once-a-day gate and the no-row guard each made
  tests fail.
- `handler/ops_alert_test.go`: `PostOpsAlert` against a local `httptest` server
  (never Discord), including a hung server (returns at once) and the 2000-char cap.
- `make check-fast` passed. `make test-integration` passed on a private database
  (`uneasy_claude_ops_alerts_fd589e`), including `ops/ops_integration_test.go`,
  which runs the workflow's exact upsert, the no-row read, and the single-row
  constraint. I also ran the workflow's quoted `psql` command in a container
  against that database.
- **Not run:** the workflow itself on GitHub (needs the secrets, and the
  migration must be deployed first); a real Discord post; a real Neon
  suspend/quota error, so the classification of Neon's exact quota error is
  untested (any connect-time error counts, which should cover it).

## Behaviour changes to look at before merging

- **Order matters on first deploy:** the workflow's new last step fails with
  "relation does not exist" if it runs before the server with migration 059 has
  deployed. A failed run emails the owner; just rerun it after the deploy.
- Until that first backup writes a row, no backup-overdue alert can fire.
- A server that can't reach the database at startup exits before it can post,
  so the alert is unlikely to get out then (Railway's crash notices are the
  signal). Documented in OPERATIONS.md.
- With no traffic, an outage is noticed at the next request or the next
  30-minute tick, not instantly.

## Open questions for the owner

None.

## Known limits (reviewed, accepted)

- Alert posts run in separate goroutines, so two messages raised close together
  (failing, then recovered) can arrive out of order.
- The one-an-hour limit is shared across outages, so a second outage within an
  hour of the first's message is not announced until the hour is up.
- Neon's occasional maintenance restart gives one "failing" and one "recovered"
  message (SQLSTATE 57P01); expected, noted in OPERATIONS.md.

## Noticed, not fixed

- The nightly e2e workflow has the same 60-day scheduled-workflow switch-off,
  and nothing alerts on it. Not fixed, as asked.
- `gofmt -l` lists three pre-existing integration test files in `handler/`
  (`plan_make_war_integration_test.go`, `push_notifications_integration_test.go`,
  `wait_state_integration_test.go`).
