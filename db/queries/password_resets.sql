-- sqlc query file for password_reset_tokens (adr/FEEDBACK_AND_RESET_PLAN.md
-- Session 2). token_hash is always a SHA-256 hash of the raw token — never
-- look these up by raw token, to avoid a timing-oracle string compare.

-- name: InsertPasswordResetToken :one
INSERT INTO password_reset_tokens (token_hash, account_id, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPasswordResetToken :one
SELECT * FROM password_reset_tokens WHERE token_hash = $1;

-- Single-use is enforced here, not by the handler's earlier read: the
-- `used_at IS NULL` condition makes the write itself the claim. Two redemptions
-- that both read the token as unused serialise on this row, and the loser's
-- UPDATE finds it already used and affects zero rows — the caller treats that
-- as "already used". :execrows is what lets it tell.
-- name: MarkPasswordResetTokenUsed :execrows
UPDATE password_reset_tokens SET used_at = now()
WHERE token_hash = $1 AND used_at IS NULL;
