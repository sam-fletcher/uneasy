-- sqlc query file for games.

-- name: CreateGame :one
INSERT INTO games (join_code) VALUES ($1) RETURNING *;

-- name: SetFacilitator :exec
UPDATE games SET facilitator_id = $1 WHERE id = $2;

-- name: GetGameByID :one
SELECT * FROM games WHERE id = $1;

-- name: GetGameByJoinCode :one
SELECT * FROM games WHERE join_code = $1;

-- name: SetGamePhase :exec
UPDATE games SET phase = $2 WHERE id = $1;

-- name: SetFocusPlayer :exec
UPDATE games SET focus_player_id = $2 WHERE id = $1;

-- name: SetCurrentRow :exec
UPDATE games SET current_row = $2 WHERE id = $1;

-- name: AdvanceRow :one
UPDATE games SET current_row = current_row + 1 WHERE id = $1 RETURNING current_row;

-- name: CountPlayersInGame :one
SELECT count(*) FROM players WHERE game_id = $1;
