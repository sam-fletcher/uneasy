-- sqlc query file for posts.

-- name: CreatePost :one
INSERT INTO posts (game_id, author_id, body)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListPosts :many
SELECT * FROM posts WHERE game_id = $1 ORDER BY created_at ASC;

-- name: ListPostsAfter :many
SELECT * FROM posts
WHERE game_id = $1 AND id > $2
ORDER BY created_at ASC;
