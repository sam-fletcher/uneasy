-- sqlc query file for players and user_tokens.

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

-- name: SetPlayerTokenColor :exec
UPDATE players SET token_color = $2 WHERE id = $1;

-- name: SetPlayerSeatOrder :exec
UPDATE players SET seat_order = $2 WHERE id = $1;

-- name: GetPlayerByID :one
SELECT * FROM players WHERE id = $1;

-- name: GetNextFocusPlayer :one
-- Returns the player with the next seat_order after the current focus player.
-- Wraps around to the lowest seat_order when the end is reached.
SELECT * FROM players
WHERE game_id = $1 AND seat_order IS NOT NULL
  AND seat_order > COALESCE(
    (SELECT seat_order FROM players WHERE id = $2),
    -1
  )
ORDER BY seat_order ASC
LIMIT 1;

-- name: GetFirstFocusPlayer :one
-- Returns the player with the lowest seat_order (for wrapping).
SELECT * FROM players
WHERE game_id = $1 AND seat_order IS NOT NULL
ORDER BY seat_order ASC
LIMIT 1;
