-- sqlc query file for sessions (cookie-token-to-account).

-- name: CreateSession :one
INSERT INTO sessions (token, account_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetSessionWithAccount :one
SELECT
  s.token, s.account_id, s.created_at, s.last_seen,
  a.id AS a_id, a.username, a.password_hash, a.email,
  a.created_at AS a_created_at, a.updated_at AS a_updated_at
FROM sessions s
JOIN accounts a ON a.id = s.account_id
WHERE s.token = $1
  AND s.last_seen > now() - interval '365 days';

-- name: TouchSession :exec
UPDATE sessions SET last_seen = now() WHERE token = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE last_seen < now() - interval '365 days';
