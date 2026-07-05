-- sqlc query file for the Shake-Up (Phase 4c).

-- ── games (category/step state) ──────────────────────────────────────────────

-- name: SetShakeUpStep :exec
UPDATE games SET shake_up_category = $2, shake_up_step = $3 WHERE id = $1;

-- ── players.shake_up_tokens ──────────────────────────────────────────────────

-- name: ZeroShakeUpTokens :exec
UPDATE players SET shake_up_tokens = 0 WHERE game_id = $1;

-- name: AddShakeUpTokens :one
UPDATE players SET shake_up_tokens = shake_up_tokens + $2 WHERE id = $1
RETURNING shake_up_tokens;

-- name: SubShakeUpTokens :one
UPDATE players SET shake_up_tokens = shake_up_tokens - $2 WHERE id = $1
RETURNING shake_up_tokens;

-- name: GetShakeUpTokens :one
SELECT shake_up_tokens FROM players WHERE id = $1;

-- name: ListShakeUpTokensByGame :many
SELECT id, shake_up_tokens FROM players WHERE game_id = $1 ORDER BY id;

-- ── shake_up_spends ──────────────────────────────────────────────────────────

-- name: CreateShakeUpSpend :one
INSERT INTO shake_up_spends (
  game_id, player_id, category, option_key, target_asset_id, target_marginalia_id, target_player_id,
  target_title_id, title_flavor, base_cost
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetShakeUpSpend :one
SELECT * FROM shake_up_spends WHERE id = $1;

-- name: GetOpenShakeUpSpend :one
SELECT * FROM shake_up_spends
WHERE game_id = $1 AND committed_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: CommitShakeUpSpend :exec
UPDATE shake_up_spends
SET final_cost = $2, committed_at = now(), applied = TRUE
WHERE id = $1;

-- name: GetLastCommittedShakeUpSpend :one
SELECT * FROM shake_up_spends
WHERE game_id = $1 AND category = $2 AND committed_at IS NOT NULL
ORDER BY committed_at DESC, id DESC
LIMIT 1;

-- ── shake_up_cost_adjustments ────────────────────────────────────────────────

-- name: CreateShakeUpAdjustment :one
INSERT INTO shake_up_cost_adjustments (spend_id, player_id, adjustment)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListAdjustmentsForSpend :many
SELECT * FROM shake_up_cost_adjustments WHERE spend_id = $1 ORDER BY created_at;

-- name: SumAdjustmentsForSpend :one
SELECT COALESCE(SUM(adjustment), 0)::SMALLINT AS total
FROM shake_up_cost_adjustments WHERE spend_id = $1;

-- ── shake_up_spend_passes ────────────────────────────────────────────────────

-- name: CreateShakeUpPass :one
INSERT INTO shake_up_spend_passes (spend_id, player_id)
VALUES ($1, $2)
ON CONFLICT (spend_id, player_id) DO UPDATE SET spend_id = EXCLUDED.spend_id
RETURNING *;

-- name: ListPassesForSpend :many
SELECT * FROM shake_up_spend_passes WHERE spend_id = $1 ORDER BY created_at;

-- name: DeletePassesForSpend :exec
DELETE FROM shake_up_spend_passes WHERE spend_id = $1;
