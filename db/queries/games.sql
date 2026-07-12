-- sqlc query file for games.

-- name: CreateGame :one
INSERT INTO games (join_code) VALUES ($1) RETURNING *;

-- name: SetFacilitator :exec
UPDATE games SET facilitator_id = $1 WHERE id = $2;

-- name: GetGameByID :one
SELECT * FROM games WHERE id = $1;

-- name: GetGameByJoinCode :one
SELECT * FROM games WHERE join_code = $1;

-- name: SetGamePhase :exec
UPDATE games SET phase = $2 WHERE id = $1;

-- name: SetFocusPlayer :exec
UPDATE games SET focus_player_id = $2 WHERE id = $1;

-- name: SetCurrentRow :exec
UPDATE games SET current_row = $2 WHERE id = $1;

-- name: AdvanceRow :one
UPDATE games SET current_row = current_row + 1 WHERE id = $1 RETURNING current_row;

-- name: CountPlayersInGame :one
SELECT count(*) FROM players WHERE game_id = $1;

-- name: SetEndingMode :exec
UPDATE games SET ending_mode = $2 WHERE id = $1;

-- name: EstablishThrone :exec
-- Trips the throne_established gate the first time a monarch title is claimed
-- (ADR-007). Idempotent and one-way: it never flips back to false, so a later
-- destroy of the monarch's asset can't erase that the throne ever existed.
UPDATE games SET throne_established = TRUE WHERE id = $1;

-- name: ListGamesNeedingNotificationReconcile :many
-- Every game the Session 3 notification ticker reconciles each tick: all
-- non-ended games (ComputeWaitState may name waitees), PLUS any ended game
-- that still has pending_notifications rows left over from before it ended —
-- reconcileWaitees correctly clears those (ComputeWaitState returns
-- WaitKindNobody for 'ended'), but only if the tick still visits the game.
-- Without the union, an ended game's stale rows are never revisited and
-- their owners get pinged forever.
SELECT id FROM games WHERE phase != 'ended'
UNION
SELECT DISTINCT game_id FROM pending_notifications
ORDER BY id;
