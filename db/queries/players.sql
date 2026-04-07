-- sqlc query file for players and user_tokens.
-- Run `sqlc generate` to produce typed Go from these.
-- For Phase 1, the store.go file implements these by hand with raw pgx.

-- name: UpsertUserToken :one
INSERT INTO user_tokens (token, display_name)
VALUES ($1, $2)
ON CONFLICT (token) DO UPDATE SET display_name = EXCLUDED.display_name
RETURNING *;

-- name: GetUserToken :one
SELECT * FROM user_tokens WHERE token = $1;

-- name: CreatePlayer :one
INSERT INTO players (game_id, display_name, cookie_token, is_facilitator)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPlayerByToken :one
SELECT * FROM players WHERE cookie_token = $1;

-- name: GetPlayersByGame :many
SELECT * FROM players WHERE game_id = $1 ORDER BY joined_at;
