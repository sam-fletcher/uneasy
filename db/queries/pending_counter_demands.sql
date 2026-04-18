-- sqlc queries for pending counter-demands (Phase 3d — Make Demands).
--
-- A row is inserted when the target of a marred demand elects to defer
-- their free counter-demand to "the next plan the demander prepares".
-- It is consumed the next time that player successfully prepares any
-- plan; a synthetic Make Demands plan targeting the new plan is created
-- and this row is marked resolved.

-- name: CreatePendingCounterDemand :one
INSERT INTO pending_counter_demands (
  game_id, demanding_player_id, target_player_id, origin_plan_id
) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPendingCounterDemand :one
SELECT * FROM pending_counter_demands WHERE id = $1;

-- name: ListOpenPendingCounterDemandsForPlayer :many
-- All unresolved pending counter-demands waiting on this player (the
-- "demanding" side — i.e. the player whose next-prepared plan will be
-- counter-demanded).
SELECT * FROM pending_counter_demands
WHERE demanding_player_id = $1 AND resolved_at IS NULL
ORDER BY created_at ASC;

-- name: ConsumePendingCounterDemand :one
-- Returns the oldest unresolved pending counter-demand for this player,
-- if any. Caller is responsible for creating the synthesized demand
-- plan and then calling ResolvePendingCounterDemand to close it.
SELECT * FROM pending_counter_demands
WHERE demanding_player_id = $1 AND resolved_at IS NULL
ORDER BY created_at ASC
LIMIT 1;

-- name: ResolvePendingCounterDemand :exec
UPDATE pending_counter_demands
SET resolved_at = now(), resolved_plan_id = $2
WHERE id = $1;
