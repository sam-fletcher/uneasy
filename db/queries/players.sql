-- sqlc query file for players.

-- name: CreatePlayer :one
INSERT INTO players (game_id, display_name, cookie_token, is_facilitator)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPlayerByToken :one
SELECT * FROM players WHERE cookie_token = $1;

-- name: GetPlayerByID :one
SELECT * FROM players WHERE id = $1;

-- name: GetPlayersByGame :many
SELECT * FROM players WHERE game_id = $1 ORDER BY joined_at;

-- name: IsPlayerInGame :one
SELECT EXISTS (
  SELECT 1 FROM players WHERE game_id = $1 AND cookie_token = $2
) AS exists;

-- name: SetPlayerTokenColor :exec
UPDATE players SET token_color = $2 WHERE id = $1;

-- name: SetPlayerSeatOrder :exec
UPDATE players SET seat_order = $2 WHERE id = $1;

-- name: GetNextFocusPlayer :one
-- Returns the player with the next seat_order after the current focus player.
-- Wraps around to the lowest seat_order when the end is reached.
SELECT p.* FROM players p
WHERE p.game_id = $1 AND p.seat_order IS NOT NULL
  AND p.seat_order > COALESCE(
    (SELECT p2.seat_order FROM players p2 WHERE p2.id = $2),
    -1
  )
ORDER BY p.seat_order ASC
LIMIT 1;

-- name: GetFirstFocusPlayer :one
-- Returns the player with the lowest seat_order (for wrapping).
SELECT * FROM players
WHERE game_id = $1 AND seat_order IS NOT NULL
ORDER BY seat_order ASC
LIMIT 1;
