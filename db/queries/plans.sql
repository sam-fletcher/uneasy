-- sqlc query file for plans and plan tokens.

-- ── Plans ────────────────────────────────────────────────────────────

-- name: CreatePlan :one
INSERT INTO plans (
  game_id, plan_type, category, preparer_id,
  target_player_id, target_asset_id,
  row_number, row_order, prepared_at_row, preparation_notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetPlanByID :one
SELECT * FROM plans WHERE id = $1;

-- name: ListPlansByGame :many
SELECT * FROM plans WHERE game_id = $1
ORDER BY row_number ASC, row_order ASC;

-- name: ListPlansByRow :many
SELECT * FROM plans
WHERE game_id = $1 AND row_number = $2
ORDER BY row_order ASC;

-- name: ListPendingPlansByRow :many
SELECT * FROM plans
WHERE game_id = $1 AND row_number = $2 AND status = 'pending'
ORDER BY row_order ASC;

-- name: ListUnresolvedPlans :many
SELECT * FROM plans
WHERE game_id = $1 AND status IN ('pending', 'resolving')
ORDER BY row_number ASC, row_order ASC;

-- name: SetPlanStatus :exec
UPDATE plans SET status = $2 WHERE id = $1;

-- name: SetPlanResult :exec
UPDATE plans SET result = $2, resolved_at = now(), status = 'resolved' WHERE id = $1;

-- name: CountPlansOnRow :one
SELECT count(*) FROM plans WHERE game_id = $1 AND row_number = $2;

-- name: GetResolvingPlanForGame :one
-- Returns the single plan currently in 'resolving' state for a game.
SELECT * FROM plans WHERE game_id = $1 AND status = 'resolving' LIMIT 1;

-- name: SetPlanResolutionData :exec
UPDATE plans SET resolution_data = $2 WHERE id = $1;

-- name: SetPlanRowNumber :exec
-- Updates a plan's row_number. Used by variable-delay plans (CL, MW) after
-- the simultaneous reveal determines the actual delay.
UPDATE plans SET row_number = $2 WHERE id = $1;

-- name: ListRecentPlansByPreparer :many
-- Returns the most recently prepared plans for a player in a game, ordered
-- newest-first. Used for esteem lockout checks (SP mar option b).
SELECT * FROM plans
WHERE game_id = $1 AND preparer_id = $2
ORDER BY prepared_at_row DESC, id DESC
LIMIT 20;

-- ── Plan Tokens ──────────────────────────────────────────────────────

-- name: CreatePlanToken :one
INSERT INTO plan_tokens (game_id, plan_type, player_id, plan_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPlanTokensByGame :many
SELECT * FROM plan_tokens WHERE game_id = $1;

-- name: ListPlanTokensByType :many
SELECT * FROM plan_tokens WHERE game_id = $1 AND plan_type = $2;

-- name: GetPlanTokenByTypeAndPlayer :one
SELECT * FROM plan_tokens
WHERE game_id = $1 AND plan_type = $2 AND player_id = $3;

-- name: DeletePlanTokensByCategory :exec
-- Used during ranking update when all plans on a sheet are filled.
DELETE FROM plan_tokens pt
USING plans p
WHERE pt.plan_id = p.id AND pt.game_id = $1 AND p.category = $2;
