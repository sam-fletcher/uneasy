-- sqlc query file for scene posts (replaces Phase 1 flat posts).

-- name: CreateScenePost :one
INSERT INTO scene_posts (game_id, row_number, plan_id, author_id, body)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListScenePostsByRow :many
SELECT * FROM scene_posts
WHERE game_id = $1 AND row_number = $2
ORDER BY created_at ASC;

-- name: ListScenePostsByRowAndPlan :many
SELECT * FROM scene_posts
WHERE game_id = $1 AND row_number = $2 AND plan_id = $3
ORDER BY created_at ASC;

-- name: ListScenePostsByRowOpenScene :many
SELECT * FROM scene_posts
WHERE game_id = $1 AND row_number = $2 AND plan_id IS NULL
ORDER BY created_at ASC;

-- name: ListScenePostsAfter :many
SELECT * FROM scene_posts
WHERE game_id = $1 AND id > $2
ORDER BY created_at ASC;

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
