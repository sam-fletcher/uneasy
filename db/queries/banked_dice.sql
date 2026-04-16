-- sqlc query file for banked_dice (Clandestinely Liaise leverage_partner option).

-- name: CreateBankedDie :one
INSERT INTO banked_dice (game_id, player_id, face, source)
VALUES ($1, $2, $3, $4)
RETURNING id, game_id, player_id, face, source, created_at, used_at, used_roll_id;

-- name: GetBankedDie :one
SELECT id, game_id, player_id, face, source, created_at, used_at, used_roll_id
FROM banked_dice
WHERE id = $1;

-- name: ListBankedDiceByPlayer :many
-- Returns all unused banked dice for a player in a game.
SELECT id, game_id, player_id, face, source, created_at, used_at, used_roll_id
FROM banked_dice
WHERE game_id = $1 AND player_id = $2 AND used_at IS NULL
ORDER BY created_at ASC;

-- name: MarkBankedDieUsed :exec
UPDATE banked_dice
SET used_at = now(), used_roll_id = $2
WHERE id = $1;
