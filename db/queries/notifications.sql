-- sqlc query file for web-push notifications (adr/NOTIFICATIONS_PLAN.md
-- Session 3): push_subscriptions (one row per browser/device a player has
-- opted push into) and pending_notifications (one row per (player, game)
-- currently named by ComputeWaitState — the reconciler's per-waitee timer;
-- see handler/push_notifications.go).

-- name: UpsertPushSubscription :one
INSERT INTO push_subscriptions (account_id, endpoint, p256dh, auth)
VALUES ($1, $2, $3, $4)
ON CONFLICT (endpoint) DO UPDATE SET
  account_id = EXCLUDED.account_id,
  p256dh = EXCLUDED.p256dh,
  auth = EXCLUDED.auth
RETURNING *;

-- name: DeletePushSubscriptionByEndpoint :exec
DELETE FROM push_subscriptions WHERE endpoint = $1 AND account_id = $2;

-- name: DeletePushSubscriptionByID :exec
DELETE FROM push_subscriptions WHERE id = $1;

-- name: UpsertPendingNotification :exec
-- Starts a waitee's reminder timer, skipping accounts whose cadence is NULL
-- (off) via the join — if they enable notifications mid-wait, the next tick
-- picks them up since ON CONFLICT DO NOTHING only no-ops when a row already
-- exists, not when the join comes up empty.
INSERT INTO pending_notifications (player_id, game_id, due_at)
SELECT p.id, sqlc.arg(game_id)::BIGINT,
       now() + make_interval(hours => a.notify_cadence_hours::int)
FROM players p
JOIN accounts a ON a.id = p.account_id
WHERE p.id = sqlc.arg(player_id)::BIGINT
  AND a.notify_cadence_hours IS NOT NULL
ON CONFLICT (player_id) DO NOTHING;

-- name: DeleteDepartedPendingNotifications :exec
-- Clears the timer for anyone in game_id no longer named by ComputeWaitState
-- — clear-on-act falls out for free; acting then re-blocking starts a fresh
-- timer, which is correct.
DELETE FROM pending_notifications
WHERE game_id = sqlc.arg(game_id)::BIGINT
  AND player_id != ALL(sqlc.arg(player_ids)::BIGINT[]);

-- name: DeletePendingNotification :exec
-- Clears a single player's timer outright — used when their account's
-- cadence has gone NULL (disabled) by send time: nothing to re-bump to, and
-- re-enabling while still waiting re-inserts on the next reconcile tick,
-- mirroring the enable-mid-wait behavior above.
DELETE FROM pending_notifications WHERE player_id = $1;

-- name: ListDueNotificationsWithSubscriptions :many
-- One row per (due notification, push_subscription) pair. A player with no
-- subscriptions yet still gets exactly one row (subscription fields NULL)
-- via the LEFT JOIN, so the caller can still re-bump/clear their timer even
-- though there's nothing to actually send.
--
-- give_up_days is the point at which we stop reminding about a single
-- uninterrupted wait altogether. Without it a table nobody intends to finish
-- pings its last waitee forever: the row is only ever deleted by acting or by
-- the game ending, and neither happens to a game the group has quietly
-- abandoned. Rows past the horizon are simply not selected, so they are never
-- sent and never re-bumped — inert, but still present, which is what lets
-- ListPlayerActivityByGame report the state rather than silently promising a
-- ping that will never come. The constant lives in Go
-- (reminderGiveUpDays, handler/push_notifications.go) because that query and
-- this one must agree on it.
SELECT
  pn.player_id,
  pn.game_id,
  g.join_code,
  a.id AS account_id,
  a.notify_cadence_hours,
  ps.id AS subscription_id,
  ps.endpoint,
  ps.p256dh,
  ps.auth
FROM pending_notifications pn
JOIN games g ON g.id = pn.game_id
JOIN players p ON p.id = pn.player_id
JOIN accounts a ON a.id = p.account_id
LEFT JOIN push_subscriptions ps ON ps.account_id = a.id
WHERE pn.due_at <= now()
  AND pn.first_waiting_at > now() - make_interval(days => sqlc.arg(give_up_days)::int)
ORDER BY pn.player_id;

-- name: RebumpPendingNotification :exec
-- Schedules the next reminder for a wait that is still unanswered, backing
-- off as it ages.
--
-- The row itself is the "same unanswered ask" identity: UpsertPendingNotification
-- leaves an existing row untouched (ON CONFLICT DO NOTHING) and
-- DeleteDepartedPendingNotifications removes it the moment ComputeWaitState
-- stops naming the player, so a surviving row means this player has been
-- blocking this game continuously since first_waiting_at, and every ping sent
-- in that window repeated the same request. The delay can therefore be a
-- function of the wait's own age, with no send counter to maintain.
--
-- age/2 grows the gap by 1.5x per reminder. The two clamps are what make that
-- humane at both ends:
--   * GREATEST(cadence) — early reminders still arrive at the rate the player
--     asked for, so someone on "every hour" is not stretched to three-hourly
--     before lunch.
--   * LEAST(72 hours) — never slower than the slowest cadence anyone can
--     choose, so backoff can't quietly become silence. Going quiet is the
--     give-up horizon's job (see ListDueNotificationsWithSubscriptions), and
--     it should be the only thing that does it.
-- Both parameters are intrinsic to this one expression, which is why they sit
-- here rather than in Go alongside reminderGiveUpDays.
UPDATE pending_notifications
SET due_at = now() + GREATEST(
      make_interval(hours => sqlc.arg(cadence_hours)::int),
      LEAST((now() - first_waiting_at) / 2, interval '72 hours')
    )
WHERE player_id = sqlc.arg(player_id)::BIGINT;

-- name: GetPendingNotification :one
SELECT * FROM pending_notifications WHERE player_id = $1;

-- name: ListPendingNotificationsByGame :many
SELECT * FROM pending_notifications WHERE game_id = $1 ORDER BY player_id;

-- name: ListPushSubscriptionsByAccount :many
SELECT * FROM push_subscriptions WHERE account_id = $1 ORDER BY id;
