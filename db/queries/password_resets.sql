-- sqlc query file for password_reset_tokens (adr/FEEDBACK_AND_RESET_PLAN.md
-- Session 2). token_hash is always a SHA-256 hash of the raw token — never
-- look these up by raw token, to avoid a timing-oracle string compare.

-- name: InsertPasswordResetToken :one
INSERT INTO password_reset_tokens (token_hash, account_id, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordResetToken :one
SELECT * FROM password_reset_tokens WHERE token_hash = $1;

-- name: MarkPasswordResetTokenUsed :exec
UPDATE password_reset_tokens SET used_at = now() WHERE token_hash = $1;
