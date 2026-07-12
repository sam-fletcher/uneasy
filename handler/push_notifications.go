package handler

// push_notifications.go — the Session 3 notification ticker
// (adr/NOTIFICATIONS_PLAN.md): reconciles pending_notifications against
// ComputeWaitState's current waitee set for every non-ended game (plus any
// ended game with leftover rows), then sends (and re-bumps, or prunes dead
// subscriptions for) every row past due_at.
//
// NOT notify.go — that's the unrelated Discord webhook for feedback/reset-
// request submissions (adr/FEEDBACK_AND_RESET_PLAN.md). This file's sole
// caller is the minute-ticker goroutine in cmd/server/main.go (the same
// pattern as expireSessionsDaily); it never runs on the request path.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"uneasy/db"
	dbgen "uneasy/db/gen"
)

// vapidPublicKey, vapidPrivateKey, and vapidSubject are set once at startup
// by SetVAPIDKeys, mirroring the discordWebhookURL pattern in notify.go.
// Left empty in dev (and any deploy that hasn't set the VAPID_* env vars
// yet) — sendGroup falls back to a structured stdout log instead of sending.
var (
	vapidPublicKey  string
	vapidPrivateKey string
	vapidSubject    string
)

// SetVAPIDKeys configures the VAPID keypair used to sign push messages and
// the "sub" claim identifying this application server to the push service
// (an operator email or https:// URL). Call once from main.go before serving
// requests.
func SetVAPIDKeys(publicKey, privateKey, subject string) {
	vapidPublicKey = publicKey
	vapidPrivateKey = privateKey
	vapidSubject = subject
}

// pushPostTimeout bounds a single push send. Encryption is CPU-bound and
// fast; this is really a network timeout against the push relay (FCM,
// Mozilla autopush, Apple).
const pushPostTimeout = 10 * time.Second

// pushTTLSeconds tells the push relay how long to hold an undelivered
// message before giving up. A day is plenty — a repeat ping fires at the
// player's cadence regardless, so nothing is lost by not holding longer.
const pushTTLSeconds = 24 * 60 * 60

// pushTitle is fixed across every notification (settled decisions table,
// adr/NOTIFICATIONS_PLAN.md) — v1 has exactly one notification shape.
const pushTitle = "Uneasy — your move"

// pushPayload is the JSON body of every push message this server sends. The
// Session 4 service worker reads title/body directly into showNotification,
// tag so a repeat ping replaces rather than stacks, and url as the deep link
// for notificationclick.
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Tag   string `json:"tag"`
	URL   string `json:"url"`
}

// buildPushPayload renders the fixed copy for gameID/joinCode. Split out
// from sendGroup so tests can assert on the encoding without a network round
// trip (mirrors formatDiscordMessage in notify.go).
func buildPushPayload(joinCode string, gameID int64) ([]byte, error) {
	return json.Marshal(pushPayload{
		Title: pushTitle,
		Body:  fmt.Sprintf("Table %s is waiting on you.", joinCode),
		Tag:   fmt.Sprintf("game-%d", gameID),
		URL:   fmt.Sprintf("/table/%d", gameID),
	})
}

// RunNotificationTick is one full reconcile+send pass across every game that
// needs it — every non-ended game, plus any ended game with leftover
// pending_notifications rows (reconcileWaitees clears those; see
// ListGamesNeedingNotificationReconcile) — the body of the minute-ticker
// goroutine in cmd/server/main.go. Exported so that goroutine (and tests) can
// drive it directly.
func RunNotificationTick(ctx context.Context, store *db.Store, logger *slog.Logger) {
	gameIDs, err := store.Q.ListGamesNeedingNotificationReconcile(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "notification tick: list games failed", "err", err)
		return
	}
	for _, gameID := range gameIDs {
		if err := reconcileWaitees(ctx, store.Q, gameID); err != nil {
			logger.ErrorContext(ctx, "notification tick: reconcile failed", "game_id", gameID, "err", err)
		}
	}
	sendDueNotifications(ctx, store, logger)
}

// reconcileWaitees upserts a pending_notifications row for every player
// ComputeWaitState currently names for gameID (skipping accounts whose
// cadence is off), then deletes any row for a player no longer named —
// clear-on-act falls out for free, and re-blocking later starts a fresh
// timer.
func reconcileWaitees(ctx context.Context, q *dbgen.Queries, gameID int64) error {
	ws, err := ComputeWaitState(ctx, q, gameID)
	if err != nil {
		return err
	}
	for _, playerID := range ws.ActingPlayerIDs {
		if err := q.UpsertPendingNotification(ctx, dbgen.UpsertPendingNotificationParams{
			GameID:   gameID,
			PlayerID: playerID,
		}); err != nil {
			return err
		}
	}
	// != ALL(NULL::bigint[]) is NULL (not true) for every row, which would
	// leave every timer running instead of clearing them — pgx encodes a nil
	// slice as SQL NULL, so an empty-but-non-nil slice must be passed
	// explicitly when nobody is currently waiting.
	playerIDs := ws.ActingPlayerIDs
	if playerIDs == nil {
		playerIDs = []int64{}
	}
	return q.DeleteDepartedPendingNotifications(ctx, dbgen.DeleteDepartedPendingNotificationsParams{
		GameID:    gameID,
		PlayerIds: playerIDs,
	})
}

// dueGroup collapses ListDueNotificationsWithSubscriptions's one-row-per-
// subscription result into one entry per due player, since sending and
// re-bumping both operate at player granularity.
type dueGroup struct {
	playerID     int64
	gameID       int64
	joinCode     string
	cadenceHours *int16
	subs         []dueSub
}

type dueSub struct {
	id                     int64
	endpoint, p256dh, auth string
}

// groupDueNotifications collapses rows (one per (notification, subscription)
// pair, per the LEFT JOIN in ListDueNotificationsWithSubscriptions) into one
// dueGroup per player, preserving row order.
func groupDueNotifications(rows []dbgen.ListDueNotificationsWithSubscriptionsRow) []dueGroup {
	groups := make(map[int64]*dueGroup, len(rows))
	order := make([]int64, 0, len(rows))
	for _, row := range rows {
		g, ok := groups[row.PlayerID]
		if !ok {
			g = &dueGroup{
				playerID:     row.PlayerID,
				gameID:       row.GameID,
				joinCode:     row.JoinCode,
				cadenceHours: row.NotifyCadenceHours,
			}
			groups[row.PlayerID] = g
			order = append(order, row.PlayerID)
		}
		if row.SubscriptionID != nil {
			g.subs = append(g.subs, dueSub{
				id:       *row.SubscriptionID,
				endpoint: *row.Endpoint,
				p256dh:   *row.P256dh,
				auth:     *row.Auth,
			})
		}
	}
	out := make([]dueGroup, 0, len(order))
	for _, playerID := range order {
		out = append(out, *groups[playerID])
	}
	return out
}

// sendDueNotifications sends (or dev-logs) every group past due_at, then
// re-bumps or clears its timer regardless of send outcome — a failed or
// subscription-less send still needs its timer moved forward so it doesn't
// fire again every tick until the cadence elapses.
func sendDueNotifications(ctx context.Context, store *db.Store, logger *slog.Logger) {
	rows, err := store.Q.ListDueNotificationsWithSubscriptions(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "notification tick: list due failed", "err", err)
		return
	}
	for _, g := range groupDueNotifications(rows) {
		sendGroup(ctx, store, logger, g)
	}
}

func sendGroup(ctx context.Context, store *db.Store, logger *slog.Logger, g dueGroup) {
	if vapidPublicKey == "" || vapidPrivateKey == "" {
		logger.InfoContext(ctx, "push notification (VAPID keys unset)",
			"player_id", g.playerID, "game_id", g.gameID, "subscriptions", len(g.subs))
	} else if payload, err := buildPushPayload(g.joinCode, g.gameID); err != nil {
		logger.ErrorContext(ctx, "notification tick: encode payload failed", "err", err, "player_id", g.playerID)
	} else {
		for _, sub := range g.subs {
			prune, sendErr := sendPush(ctx, sub.endpoint, sub.p256dh, sub.auth, payload)
			if sendErr != nil {
				logger.WarnContext(ctx, "push notify: send failed", "err", sendErr, "subscription_id", sub.id)
				continue
			}
			if prune {
				if err := store.Q.DeletePushSubscriptionByID(ctx, sub.id); err != nil {
					logger.WarnContext(ctx, "push notify: prune failed", "err", err, "subscription_id", sub.id)
				}
			}
		}
	}
	rebumpOrClear(ctx, store, logger, g.playerID, g.cadenceHours)
}

// sendPush POSTs one encrypted push message to a subscription's endpoint.
// Returns prune=true when the relay responds 404/410 — the standard signal
// that the subscription is dead and its row should be deleted, per the Web
// Push protocol. No retries: the caller re-bumps due_at regardless, so a
// failed send is simply tried again next cadence.
func sendPush(ctx context.Context, endpoint, p256dh, auth string, payload []byte) (prune bool, err error) {
	sendCtx, cancel := context.WithTimeout(ctx, pushPostTimeout)
	defer cancel()

	resp, err := webpush.SendNotificationWithContext(sendCtx, payload, &webpush.Subscription{
		Endpoint: endpoint,
		Keys:     webpush.Keys{Auth: auth, P256dh: p256dh},
	}, &webpush.Options{
		Subscriber:      vapidSubject,
		VAPIDPublicKey:  vapidPublicKey,
		VAPIDPrivateKey: vapidPrivateKey,
		TTL:             pushTTLSeconds,
	})
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone, nil
}

// rebumpOrClear advances playerID's timer to now+cadence, or — if the
// account's cadence has gone NULL (disabled) since the row was inserted —
// deletes the row outright. There is nothing to re-bump to for a disabled
// cadence, and re-enabling while still waiting re-inserts on the next
// reconcile tick, mirroring the enable-mid-wait behavior in
// UpsertPendingNotification.
func rebumpOrClear(ctx context.Context, store *db.Store, logger *slog.Logger, playerID int64, cadenceHours *int16) {
	if cadenceHours == nil {
		if err := store.Q.DeletePendingNotification(ctx, playerID); err != nil {
			logger.WarnContext(ctx, "push notify: clear disabled-cadence timer failed",
				"err", err, "player_id", playerID)
		}
		return
	}
	if err := store.Q.RebumpPendingNotification(ctx, dbgen.RebumpPendingNotificationParams{
		CadenceHours: int32(*cadenceHours),
		PlayerID:     playerID,
	}); err != nil {
		logger.WarnContext(ctx, "push notify: rebump failed", "err", err, "player_id", playerID)
	}
}
