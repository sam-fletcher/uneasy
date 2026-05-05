-- sqlc query file for players.

-- name: CreatePlayer :one
INSERT INTO players (game_id, display_name, account_id, is_facilitator)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPlayerByAccountAndGame :one
SELECT * FROM players WHERE account_id = $1 AND game_id = $2;

-- name: GetPlayerByID :one
SELECT * FROM players WHERE id = $1;

-- name: GetPlayersByGame :many
SELECT * FROM players WHERE game_id = $1 ORDER BY joined_at;

-- name: ListPlayersByAccount :many
SELECT p.*, g.join_code
FROM players p
JOIN games g ON g.id = p.game_id
WHERE p.account_id = $1
ORDER BY p.joined_at DESC;

-- name: IsPlayerInGame :one
SELECT EXISTS (
  SELECT 1 FROM players WHERE game_id = $1 AND account_id = $2
) AS exists;

-- name: SetPlayerTokenColor :exec
UPDATE players SET token_color = $2 WHERE id = $1;

-- name: SetPlayerSeatOrder :exec
UPDATE players SET seat_order = $2 WHERE id = $1;

-- name: GetNextFocusPlayer :one
-- Returns the next player in join order after the current focus player.
-- Caller must wrap around (use GetFirstFocusPlayer) when no row is returned.
SELECT p.* FROM players p
WHERE p.game_id = $1
  AND p.joined_at > COALESCE(
    (SELECT p2.joined_at FROM players p2 WHERE p2.id = $2),
    'epoch'::timestamptz
  )
ORDER BY p.joined_at ASC
LIMIT 1;

-- name: GetFirstFocusPlayer :one
-- Returns the player who joined first (the facilitator, in practice).
SELECT * FROM players
WHERE game_id = $1
ORDER BY joined_at ASC
LIMIT 1;
