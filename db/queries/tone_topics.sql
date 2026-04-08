-- sqlc query file for tone-setting topics.

-- name: CreateToneTopic :one
INSERT INTO tone_topics (game_id, topic, status)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListToneTopics :many
SELECT * FROM tone_topics WHERE game_id = $1 ORDER BY id;

-- name: UpdateToneTopicStatus :exec
UPDATE tone_topics SET status = $2 WHERE id = $1;

-- name: GetToneTopic :one
SELECT * FROM tone_topics WHERE id = $1;
