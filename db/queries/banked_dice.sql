-- sqlc query file for banked_dice (Clandestinely Liaise leverage_partner option).

-- name: CreateBankedDie :one
INSERT INTO banked_dice (game_id, player_id, source)
VALUES ($1, $2, $3)
RETURNING id, game_id, player_id, source, created_at, used_at, used_roll_id;

-- name: GetBankedDie :one
SELECT id, game_id, player_id, source, created_at, used_at, used_roll_id
FROM banked_dice
WHERE id = $1;

-- name: ListBankedDiceByPlayer :many
-- Returns all unused banked dice for a player in a game.
SELECT id, game_id, player_id, source, created_at, used_at, used_roll_id
FROM banked_dice
WHERE game_id = $1 AND player_id = $2 AND used_at IS NULL
ORDER BY created_at ASC;

-- name: CountUnspentBankedDiceByPlayer :one
SELECT count(*)::bigint AS dice_count
FROM banked_dice
WHERE game_id = $1 AND player_id = $2 AND used_at IS NULL;

-- name: MarkBankedDieUsed :exec
UPDATE banked_dice
SET used_at = now(), used_roll_id = $2
WHERE id = $1;

-- name: DeleteUnspentBankedDiceBySource :exec
-- Discards every unspent banked die of a given source in a game. Used by
-- Propose Decree to clear the ephemeral 'decree' dice a joiner did not spend
-- on the council roll, so they cannot leak onto a later, unrelated roll.
DELETE FROM banked_dice
WHERE game_id = $1 AND source = $2 AND used_at IS NULL;
