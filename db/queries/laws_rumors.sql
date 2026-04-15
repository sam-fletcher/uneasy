-- sqlc query file for laws and rumors.

-- ── Laws ─────────────────────────────────────────────────────────────

-- name: CreateLaw :one
INSERT INTO laws (game_id, text, addendum, origin_plan_id, signatory_id, display_order)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListLaws :many
SELECT * FROM laws WHERE game_id = $1 AND is_active = TRUE
ORDER BY display_order ASC, created_at ASC;

-- name: DeactivateLaw :exec
UPDATE laws SET is_active = FALSE WHERE id = $1;

-- ── Rumors ───────────────────────────────────────────────────────────

-- name: CreateRumor :one
INSERT INTO rumors (game_id, text, target_asset_id, origin_plan_id, source_player_id, display_order)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListRumors :many
SELECT * FROM rumors WHERE game_id = $1 AND is_active = TRUE
ORDER BY display_order ASC, created_at ASC;

-- name: DeactivateRumor :exec
UPDATE rumors SET is_active = FALSE WHERE id = $1;

-- name: SetRumorSourceHidden :exec
-- Remove the source attribution from a rumor (Spread Rumors hide-source option).
UPDATE rumors SET source_player_id = NULL WHERE id = $1;

-- name: GetRumorByID :one
SELECT * FROM rumors WHERE id = $1;
