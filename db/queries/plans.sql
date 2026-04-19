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

-- name: SetPlanResultPreserveStatus :exec
-- Sets result without transitioning status. Used by plans without a dice roll
-- (Make War) to pre-record a narrative result so CompletePlan has something
-- to store when the focus player finalises the plan.
UPDATE plans SET result = $2 WHERE id = $1;

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

-- name: GetPlansTargeting :many
-- Returns Make Demands plans whose targeted_plan_id points at the given
-- plan. Used to locate an active demand on a plan (for asset-recipient
-- redirection, leverage control, etc.) and to cascade cancels.
SELECT * FROM plans
WHERE targeted_plan_id = $1
ORDER BY id;

-- name: ClearTargetedPlan :exec
-- Clears targeted_plan_id on a demand plan. Used when the target plan is
-- cancelled and the demand cascade-cancels with it.
UPDATE plans SET targeted_plan_id = NULL WHERE id = $1;

-- name: SetDemandOptionWinners :exec
-- Persists the four draft-pick winners on the demand plan row once the
-- draft completes. Read by the target plan's resolution path.
UPDATE plans SET demand_option_winners = $2 WHERE id = $1;

-- name: SetPlanTargets :exec
-- Updates target_player_id and target_asset_id on a plan. Used by the
-- Make Demands keep_or_change_target winner to retarget a plan via the
-- demand-retarget endpoint.
UPDATE plans
SET target_player_id = $2, target_asset_id = $3
WHERE id = $1;

-- name: SetPlanTargetedPlan :exec
-- Sets targeted_plan_id on a Make Demands plan row. Called from OnPrepare
-- after the plan row has been created.
UPDATE plans SET targeted_plan_id = $2 WHERE id = $1;

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
