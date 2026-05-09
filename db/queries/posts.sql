-- sqlc query file for the unified chat feed.
--
-- scene_posts is one game-wide stream. Each row is either:
--   - a player message (author_id NOT NULL, system_code NULL, severity 0)
--   - a system post    (author_id NULL,    system_code NOT NULL, severity > 0)
--
-- Severity is an integer scale (see model/severity.go for named constants).
-- row_number / plan_id / scene_id are optional anchors the client uses for
-- the "jump to" UI in the Public Record sidebar.
--
-- scene_entries (the public-record one-line summaries) are unchanged here.

-- name: CreatePlayerMessage :one
INSERT INTO scene_posts (
  game_id, author_id, body, row_number, plan_id, scene_id, severity, speaking_as_asset_id
) VALUES ($1, $2, $3, $4, $5, $6, 0, $7)
RETURNING *;

-- name: CreateSystemPost :one
INSERT INTO scene_posts (
  game_id, body, row_number, plan_id, scene_id, severity, system_code, system_data
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListGamePosts :many
SELECT * FROM scene_posts
WHERE game_id = $1
ORDER BY id ASC;

-- name: ListGamePostsAfter :many
SELECT * FROM scene_posts
WHERE game_id = $1 AND id > $2
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
