-- sqlc query file for players.

-- name: CreatePlayer :one
INSERT INTO players (game_id, display_name, account_id, is_facilitator)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPlayerByAccountAndGame :one
SELECT * FROM players WHERE account_id = $1 AND game_id = $2;

-- name: GetPlayerByID :one
SELECT * FROM players WHERE id = $1;

-- name: GetPlayersByGame :many
SELECT * FROM players WHERE game_id = $1 ORDER BY joined_at;

-- name: ListPlayersByGames :many
-- Rosters for several games at once, for the profile page's table list — that
-- handler called GetPlayersByGame once per table, so a player in a dozen games
-- paid a round trip per card just to render the seats.
--
-- Ordered by game then joined_at so each group arrives in exactly the order
-- GetPlayersByGame returns; the caller groups in one pass. Keep the two
-- orderings in step, since the roster order is the seat order the cards render.
SELECT * FROM players
WHERE game_id = ANY(sqlc.arg(game_ids)::BIGINT[])
ORDER BY game_id, joined_at;

-- name: ListPlayersByAccount :many
SELECT p.*, g.join_code, g.phase
FROM players p
JOIN games g ON g.id = p.game_id
WHERE p.account_id = $1
ORDER BY p.joined_at DESC;

-- name: IsPlayerInGame :one
SELECT EXISTS (
  SELECT 1 FROM players WHERE game_id = $1 AND account_id = $2
) AS exists;

-- name: UpdateDisplayNameByAccount :exec
-- Propagates an account username change to the denormalized display_name
-- copy held by every player seat that account occupies, across all games.
UPDATE players SET display_name = $2 WHERE account_id = $1;

-- name: SetPlayerTokenColor :exec
UPDATE players SET token_color = $2 WHERE id = $1;

-- name: SetPlayerSeatOrder :exec
UPDATE players SET seat_order = $2 WHERE id = $1;

-- name: UpdateReadMarker :one
-- Monotonic: never moves the marker backwards, and clamps the requested
-- value to the game's newest post id so a stale/forged id can't overshoot.
UPDATE players AS p
SET last_read_post_id = GREATEST(
  p.last_read_post_id,
  LEAST(
    sqlc.arg(requested_id)::BIGINT,
    COALESCE((SELECT MAX(sp.id) FROM scene_posts sp WHERE sp.game_id = sqlc.arg(game_id)), 0)
  )
)
WHERE p.id = sqlc.arg(player_id)
RETURNING p.last_read_post_id;

-- name: TouchPlayerActivity :exec
-- Records that this player just had the table on screen (migration 055).
--
-- The throttle lives in the WHERE clause rather than in a read-then-write pair,
-- so the common case — a player flipping back to an already-fresh tab — costs
-- one no-op UPDATE and no round trip to decide it. Same tradeoff as
-- middleware's sessionTouchInterval: an hour of slack is invisible at the
-- coarse buckets the header renders ("last here 3h ago"), and it drops writes
-- by orders of magnitude on an active table, which matters on a database
-- billed by compute time.
UPDATE players
SET last_active_at = now()
WHERE id = sqlc.arg(player_id)
  AND (
    last_active_at IS NULL
    OR last_active_at < now() - make_interval(mins => sqlc.arg(throttle_minutes)::int)
  );

-- name: ListPlayerActivityByGame :many
-- Everything the Retinue header's presence and reminder lines need, for the
-- whole table, in one round trip.
--
-- reminder_due_at comes from pending_notifications, whose rows exist ONLY for
-- players ComputeWaitState currently names AND whose account has a cadence set
-- (handler/push_notifications.go) — so a NULL here is not "no reminder ever",
-- it's "no reminder pending right now". The caller pairs it with the cadence
-- and device columns to say which of those it is.
--
-- has_push_device is the one signal that catches the silent failure: a player
-- who set a cadence but never granted browser permission has notify_cadence_hours
-- set and zero subscriptions, believes they are covered, and is not. A browser
-- -level "denied" never reaches us directly, but it kills the live subscription,
-- so it lands in this same column.
--
-- reminder_exhausted is the second way a pending row can mean "no ping is
-- coming": the wait has run past the give-up horizon, so
-- ListDueNotificationsWithSubscriptions no longer selects it. The row survives
-- and keeps a due_at in the past, so without this column the header would read
-- a timer that will never fire and promise a reminder. give_up_days must be the
-- same value both queries use — it comes from reminderGiveUpDays in
-- handler/push_notifications.go.
SELECT
  p.id AS player_id,
  p.last_active_at,
  a.notify_cadence_hours,
  EXISTS (
    SELECT 1 FROM push_subscriptions ps WHERE ps.account_id = a.id
  ) AS has_push_device,
  pn.due_at AS reminder_due_at,
  COALESCE(
    pn.first_waiting_at <= now() - make_interval(days => sqlc.arg(give_up_days)::int),
    FALSE
  )::boolean AS reminder_exhausted
FROM players p
JOIN accounts a ON a.id = p.account_id
LEFT JOIN pending_notifications pn ON pn.player_id = p.id
WHERE p.game_id = sqlc.arg(game_id)::BIGINT
ORDER BY p.id;

-- name: GetNextFocusPlayer :one
-- Returns the next player in join order after the current focus player.
-- Caller must wrap around (use GetFirstFocusPlayer) when no row is returned.
SELECT p.* FROM players p
WHERE p.game_id = $1
  AND p.joined_at > COALESCE(
    (SELECT p2.joined_at FROM players p2 WHERE p2.id = $2),
    'epoch'::timestamptz
  )
ORDER BY p.joined_at ASC
LIMIT 1;

-- name: GetFirstFocusPlayer :one
-- Returns the player who joined first (the facilitator, in practice).
SELECT * FROM players
WHERE game_id = $1
ORDER BY joined_at ASC
LIMIT 1;
