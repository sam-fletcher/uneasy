-- sqlc queries for Propose Duel (Phase 3d).

-- name: CreateDuelStake :one
INSERT INTO duel_staked_assets (plan_id, player_id, asset_id, hidden_die)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListDuelStakesByPlan :many
SELECT * FROM duel_staked_assets
WHERE plan_id = $1
ORDER BY id;

-- name: ListDuelStakesByPlanPlayer :many
SELECT * FROM duel_staked_assets
WHERE plan_id = $1 AND player_id = $2
ORDER BY id;

-- name: GetDuelStake :one
SELECT * FROM duel_staked_assets WHERE id = $1;

-- name: SetDuelStakeResolved :exec
UPDATE duel_staked_assets
SET is_resolved = true, is_winner = $2
WHERE id = $1;

-- name: CountUnresolvedDuelStakes :one
SELECT COUNT(*) FROM duel_staked_assets
WHERE plan_id = $1 AND player_id = $2 AND is_resolved = false;

-- name: CreateDuelBout :one
INSERT INTO duel_bouts (
  plan_id, bout_number, declarer_id, declarer_stake_id, responder_id, declaration, declarer_die
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ResolveDuelBout :exec
UPDATE duel_bouts
SET responder_stake_id = $2,
    responder_die = $3,
    winner_id = $4,
    is_match = $5,
    resolved_at = now()
WHERE id = $1;

-- name: ListDuelBoutsByPlan :many
SELECT * FROM duel_bouts WHERE plan_id = $1 ORDER BY bout_number;

-- name: GetLatestDuelBout :one
SELECT * FROM duel_bouts
WHERE plan_id = $1
ORDER BY bout_number DESC
LIMIT 1;
