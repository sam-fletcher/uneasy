-- sqlc query file for user tokens (pre-game identity).

-- name: UpsertUserToken :one
INSERT INTO user_tokens (token, display_name)
VALUES ($1, $2)
ON CONFLICT (token) DO UPDATE SET display_name = EXCLUDED.display_name
RETURNING *;

-- name: GetUserToken :one
SELECT * FROM user_tokens WHERE token = $1;
