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

-- name: GetTurnScene :one
-- Returns the focus player's turn-scene for a given row: the scene they set
-- up at the start of their turn (resolved_plan_id IS NULL), regardless of
-- whether it has ended. Used by /game-state so a refreshing client can tell
-- whether the focus player is mid-scene or in their post-scene action step.
-- Distinct from plan-resolution scenes (kind='plan'), which never carry
-- resolved_plan_id — the kind filter is defense-in-depth against the case
-- where a plan's preparer happens to also be the row's current focus player.
SELECT * FROM scenes
WHERE game_id = $1
  AND row_number = $2
  AND focus_player_id = $3
  AND kind = 'turn'
  AND resolved_plan_id IS NULL
LIMIT 1;

-- name: CreatePlanScene :one
-- Opens a plan-scene (adr/CHAT_OVERHAUL_PLAN.md Phase 5) at the moment a
-- roleplay-heavy plan flips to resolving. No location/time setup step —
-- those columns stay NULL, per the scenes_location_by_kind CHECK.
INSERT INTO scenes (
  game_id, row_number, focus_player_id, kind, plan_id
) VALUES (
  $1, $2, $3, 'plan', $4
)
RETURNING *;

-- name: GetScenePeerByOwner :one
-- Finds a scene's peer row for whichever asset is currently owned by
-- playerID, regardless of which specific asset id it references. Used to
-- repoint a stale peer row after a main-character swap/replacement — the
-- caller doesn't need to know the old asset id, just whose row it was.
SELECT sp.* FROM scene_peers sp
JOIN assets a ON a.id = sp.peer_asset_id
WHERE sp.scene_id = $1 AND a.owner_id = $2
LIMIT 1;

-- name: UpdateScenePeerAsset :execrows
-- Repoints a scene peer row at a new asset id (a main-character swap while a
-- scene is active) without disturbing controller_player_id.
UPDATE scene_peers
SET peer_asset_id = sqlc.arg(new_peer_asset_id)
WHERE scene_id = sqlc.arg(scene_id)
  AND peer_asset_id = sqlc.arg(old_peer_asset_id);

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
