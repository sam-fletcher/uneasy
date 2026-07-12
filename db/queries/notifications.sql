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
ORDER BY pn.player_id;

-- name: RebumpPendingNotification :exec
UPDATE pending_notifications
SET due_at = now() + make_interval(hours => sqlc.arg(cadence_hours)::int)
WHERE player_id = sqlc.arg(player_id)::BIGINT;

-- name: GetPendingNotification :one
SELECT * FROM pending_notifications WHERE player_id = $1;

-- name: ListPendingNotificationsByGame :many
SELECT * FROM pending_notifications WHERE game_id = $1 ORDER BY player_id;

-- name: ListPushSubscriptionsByAccount :many
SELECT * FROM push_subscriptions WHERE account_id = $1 ORDER BY id;
