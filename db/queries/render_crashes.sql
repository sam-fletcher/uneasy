-- sqlc query file for render_crash_reports (migration 060).
--
-- Written by POST /api/render-crashes when a svelte:boundary catches a render
-- throw in the browser. The durable record is this row; nothing else reads it.

-- name: InsertRenderCrashReport :one
INSERT INTO render_crash_reports (
  account_id, game_id, boundary, message, stack, route, build
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CountRecentRenderCrashReportsByAccount :one
-- Backs the per-account render-crash rate limit: a DB count, like the feedback
-- one, but over this table only, so the two budgets never share a count.
SELECT count(*) FROM render_crash_reports
WHERE account_id = $1
  AND created_at > now() - interval '1 hour';
