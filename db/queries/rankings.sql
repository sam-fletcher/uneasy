-- sqlc query file for rankings.

-- name: UpsertRanking :exec
INSERT INTO rankings (game_id, player_id, category, rank)
VALUES ($1, $2, $3, $4)
ON CONFLICT (game_id, category, rank)
DO UPDATE SET player_id = EXCLUDED.player_id;

-- name: ListRankingsByGame :many
SELECT * FROM rankings WHERE game_id = $1 ORDER BY category, rank;

-- name: GetRanking :one
SELECT * FROM rankings
WHERE game_id = $1 AND player_id = $2 AND category = $3;

-- name: GetRankByPosition :one
SELECT * FROM rankings
WHERE game_id = $1 AND category = $2 AND rank = $3;

-- name: DeleteRankingsByGame :exec
DELETE FROM rankings WHERE game_id = $1;
