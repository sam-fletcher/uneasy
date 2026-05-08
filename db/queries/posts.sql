-- sqlc query file for the unified chat feed.
--
-- scene_posts is one game-wide stream of three kinds of entries:
--   - 'message'  : free-text written by a player (author_id required)
--   - 'log'      : system-emitted action-log entry (severity required)
--   - 'boundary' : system-emitted phase/row/plan/scene transition marker
--
-- Reads are flat and chronological by id. row_number / plan_id / system_code
-- remain on each row as metadata that the client can use for "jump to" UI.
--
-- scene_entries (the public-record one-line summaries) are unchanged here.

-- name: CreatePlayerMessage :one
INSERT INTO scene_posts (
  game_id, author_id, body, row_number, plan_id, kind, speaking_as_asset_id
) VALUES ($1, $2, $3, $4, $5, 'message', $6)
RETURNING *;

-- name: CreateBoundaryPost :one
INSERT INTO scene_posts (
  game_id, body, row_number, plan_id, kind, severity, system_code, system_data
) VALUES ($1, $2, $3, $4, 'boundary', 'important', $5, $6)
RETURNING *;

-- name: CreateLogPost :one
INSERT INTO scene_posts (
  game_id, body, row_number, plan_id, kind, severity, system_code, system_data, author_id
) VALUES ($1, $2, $3, $4, 'log', $5, $6, $7, $8)
RETURNING *;

-- name: ListGamePosts :many
SELECT * FROM scene_posts
WHERE game_id = $1
ORDER BY id ASC;

-- name: ListGamePostsAfter :many
SELECT * FROM scene_posts
WHERE game_id = $1 AND id > $2
ORDER BY id ASC;

-- name: ListGameBoundaries :many
SELECT * FROM scene_posts
WHERE game_id = $1 AND kind = 'boundary'
ORDER BY id ASC;

-- name: CreateSceneEntry :one
INSERT INTO scene_entries (game_id, row_number, author_id, body)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListSceneEntries :many
SELECT * FROM scene_entries
WHERE game_id = $1
ORDER BY row_number ASC, created_at ASC;

-- name: ListSceneEntriesByRow :many
SELECT * FROM scene_entries
WHERE game_id = $1 AND row_number = $2
ORDER BY created_at ASC;
