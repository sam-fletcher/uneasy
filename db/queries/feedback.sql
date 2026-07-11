-- sqlc query file for feedback_submissions (adr/FEEDBACK_AND_RESET_PLAN.md).
--
-- One table backs both the login-gated feedback form (kind='feedback') and
-- the logged-out reset-request intake (kind='reset_request'). The Discord
-- webhook notifier (handler/notify.go) is best-effort; this table is the
-- durable record.

-- name: InsertFeedbackSubmission :one
INSERT INTO feedback_submissions (
  kind, account_id, username, game_id, body, contact, context
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CountRecentFeedbackByAccount :one
-- Backs the 5/account/hour feedback rate limit — a DB count instead of new
-- middleware state, since feedback traffic is low-volume by nature.
SELECT count(*) FROM feedback_submissions
WHERE account_id = $1
  AND kind = 'feedback'
  AND created_at > now() - interval '1 hour';
