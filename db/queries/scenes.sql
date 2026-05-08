-- sqlc query file for scenes and scene_peers.
-- See SCENES_PLAN.md for the design.

-- name: CreateScene :one
INSERT INTO scenes (
  game_id, row_number, focus_player_id,
  location_holding_id, location_custom,
  time_elapsed, time_note,
  prompt, resolved_plan_id
) VALUES (
  $1, $2, $3,
  $4, $5,
  $6, $7,
  $8, $9
)
RETURNING *;

-- name: GetActiveScene :one
-- Returns the at-most-one active (not yet ended) scene for a game, or no rows.
SELECT * FROM scenes
WHERE game_id = $1 AND ended_at IS NULL
LIMIT 1;

-- name: GetSceneByID :one
SELECT * FROM scenes WHERE id = $1;

-- name: ListScenesForRow :many
SELECT * FROM scenes
WHERE game_id = $1 AND row_number = $2
ORDER BY started_at;

-- name: EndScene :exec
UPDATE scenes
SET ended_at = now()
WHERE id = $1 AND ended_at IS NULL;

-- name: InsertScenePeer :exec
INSERT INTO scene_peers (scene_id, peer_asset_id, controller_player_id)
VALUES ($1, $2, $3);

-- name: ListScenePeers :many
SELECT * FROM scene_peers WHERE scene_id = $1;

-- name: GetScenePeer :one
SELECT * FROM scene_peers
WHERE scene_id = $1 AND peer_asset_id = $2;

-- name: ClaimScenePeer :execrows
-- Atomically claim a previously-unclaimed peer for the caller. Caller is
-- expected to have already verified the peer is owned by the focus player;
-- this query just enforces the "still unclaimed" invariant.
UPDATE scene_peers
SET controller_player_id = $3
WHERE scene_id = $1
  AND peer_asset_id = $2
  AND controller_player_id IS NULL;

-- name: GetMostRecentResolvedPlanOnRow :one
-- Used to look up the follow-on prompt source when a focus player creates
-- a new scene on a row where a plan just resolved.
SELECT * FROM plans
WHERE game_id = $1
  AND row_number = $2
  AND status = 'resolved'
ORDER BY resolved_at DESC NULLS LAST, id DESC
LIMIT 1;
