-- sqlc query file for games.

-- name: CreateGame :one
INSERT INTO games (join_code) VALUES ($1) RETURNING *;

-- name: SetFacilitator :exec
UPDATE games SET facilitator_id = $1 WHERE id = $2;

-- name: GetGameByID :one
SELECT * FROM games WHERE id = $1;

-- name: GetGameByJoinCode :one
SELECT * FROM games WHERE join_code = $1;
