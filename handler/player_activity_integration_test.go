//go:build integration

package handler

// player_activity_integration_test.go — DB-driven coverage for the activity
// heartbeat (migration 055) and the presence/reminder summary GetGameState
// serves alongside the roster. The verdict logic itself is DB-free and lives
// in player_activity_test.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// activityHarness wires the real activity + state routes with one seeded
// session per player, so requests authenticate the way a browser's cookie
// does (same shape as newAssetHarness).
type activityHarness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	tg     testGame
	router http.Handler
	tokens []string
}

func newActivityHarness(t *testing.T, n int) *activityHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, n)
	store := db.NewStore(pool)

	tokens := make([]string, n)
	for i, p := range tg.Players {
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
			Token: tok, AccountID: p.AccountID,
		})
		require.NoError(t, err)
		tokens[i] = tok
	}

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/activity", TouchActivity(store))
	r.Post("/api/tables/{id}/reminder-mute", SetReminderMute(store))
	r.Get("/api/tables/{id}/state", GetGameState(store))

	return &activityHarness{t: t, pool: pool, q: q, tg: tg, router: r, tokens: tokens}
}

func (h *activityHarness) do(method, path string, playerIdx int) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

// ping posts the heartbeat as players[idx], the way the table page does on
// mount and on tab-visible.
func (h *activityHarness) ping(playerIdx int) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.do(http.MethodPost, tablePath(h.tg.Game.ID, "activity"), playerIdx)
}

// doBody is do() with a JSON request body, for the endpoints that take one.
func (h *activityHarness) doBody(method, path string, playerIdx int, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func tablePath(gameID int64, suffix string) string {
	return fmt.Sprintf("/api/tables/%d/%s", gameID, suffix)
}

// lastActiveOf reads the column directly — the point of these tests is what
// landed in the database, not what a query object reported about it.
func lastActiveOf(t *testing.T, pool *pgxpool.Pool, playerID int64) *time.Time {
	t.Helper()
	var ts *time.Time
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT last_active_at FROM players WHERE id = $1`, playerID).Scan(&ts))
	return ts
}

// A fresh seat has never been recorded, the first ping records it, and a
// second ping inside the throttle window leaves the value alone — that last
// part is the whole reason the throttle lives in the UPDATE's WHERE clause.
func TestTouchActivity_RecordsThenThrottles(t *testing.T) {
	h := newActivityHarness(t, 2)
	player := h.tg.Players[0]

	require.Nil(t, lastActiveOf(t, h.pool, player.ID), "a seeded seat has no recorded visit")

	require.Equal(t, http.StatusNoContent, h.ping(0).Code)
	first := lastActiveOf(t, h.pool, player.ID)
	require.NotNil(t, first, "the first ping must record a visit")

	require.Equal(t, http.StatusNoContent, h.ping(0).Code)
	second := lastActiveOf(t, h.pool, player.ID)
	require.NotNil(t, second)
	assert.True(t, first.Equal(*second),
		"a second ping inside the throttle window must not rewrite the timestamp")
}

// Once the stored value is older than the throttle, the next ping refreshes
// it. Backdating simulates the client's hourly cadence without waiting an hour.
func TestTouchActivity_RefreshesOncePastTheThrottle(t *testing.T) {
	h := newActivityHarness(t, 2)
	player := h.tg.Players[0]

	require.Equal(t, http.StatusNoContent, h.ping(0).Code)
	stale := lastActiveOf(t, h.pool, player.ID)
	require.NotNil(t, stale)

	_, err := h.pool.Exec(context.Background(),
		`UPDATE players SET last_active_at = now() - interval '2 hours' WHERE id = $1`, player.ID)
	require.NoError(t, err)

	require.Equal(t, http.StatusNoContent, h.ping(0).Code)
	fresh := lastActiveOf(t, h.pool, player.ID)
	require.NotNil(t, fresh)
	assert.WithinDuration(t, time.Now(), *fresh, time.Minute,
		"a ping past the throttle must move the timestamp back to now")
}

// The endpoint is a table-membership boundary like every other /tables route:
// an account with no seat here must not be able to write to it.
func TestTouchActivity_RejectsNonMember(t *testing.T) {
	h := newActivityHarness(t, 2)

	// A second table in the SAME database — openTestDB truncates, so a second
	// harness would wipe the first game and hand the "outsider" a seat at the
	// only table left.
	other := newTestGame(t, h.q, 2)
	outsider := other.Players[0]
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = h.q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: outsider.AccountID,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, tablePath(h.tg.Game.ID, "activity"), nil)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Nil(t, lastActiveOf(t, h.pool, outsider.ID),
		"a rejected ping must not record anything")
	assert.Nil(t, lastActiveOf(t, h.pool, h.tg.Players[0].ID),
		"and must not touch a seat at the target table either")
}

func (h *activityHarness) activityFromState(playerIdx int) map[int64]model.PlayerActivity {
	h.t.Helper()
	rec := h.do(http.MethodGet, tablePath(h.tg.Game.ID, "state"), playerIdx)
	require.Equal(h.t, http.StatusOK, rec.Code)

	var resp struct {
		PlayerActivity []model.PlayerActivity `json:"player_activity"`
	}
	require.NoError(h.t, json.Unmarshal(rec.Body.Bytes(), &resp))

	out := map[int64]model.PlayerActivity{}
	for _, a := range resp.PlayerActivity {
		out[a.PlayerID] = a
	}
	return out
}

// GetGameState carries the per-seat summary, and the reminder verdict reflects
// each account's real notification setup — two seats, two different verdicts,
// one response.
func TestGetGameState_ServesPlayerActivity(t *testing.T) {
	h := newActivityHarness(t, 2)
	ctx := context.Background()
	viewer, other := h.tg.Players[0], h.tg.Players[1]

	require.Equal(t, http.StatusNoContent, h.ping(0).Code)

	_, err := h.q.UpdateAccountNotifyCadence(ctx, dbgen.UpdateAccountNotifyCadenceParams{
		ID: other.AccountID, NotifyCadenceHours: nil,
	})
	require.NoError(t, err)

	byPlayer := h.activityFromState(0)
	require.Len(t, byPlayer, 2)

	mine := byPlayer[viewer.ID]
	require.NotNil(t, mine.LastActiveAt, "the seat that just pinged must carry a timestamp")
	assert.WithinDuration(t, time.Now(), *mine.LastActiveAt, time.Minute)
	// Seeded accounts keep the default 24h cadence but never subscribe a
	// device — precisely the silent failure the no_device verdict exists for.
	assert.Equal(t, model.ReminderNoDevice, mine.Reminder)

	theirs := byPlayer[other.ID]
	assert.Nil(t, theirs.LastActiveAt, "a seat that never opened the table stays null")
	assert.Equal(t, model.ReminderOff, theirs.Reminder)
	assert.Nil(t, theirs.ReminderDueAt)
}

// With a real subscription and a pending timer, the summary reports the due
// time — the "reminder due in ~6h" case, and the one that proves the
// pending_notifications join reads the right column.
func TestGetGameState_ReportsScheduledReminder(t *testing.T) {
	h := newActivityHarness(t, 2)
	ctx := context.Background()
	waitee := h.tg.Players[0] // scene_setting focus player, per seedBase

	p256dh, auth := testSubscriptionKeys(t)
	_, err := h.q.UpsertPushSubscription(ctx, dbgen.UpsertPushSubscriptionParams{
		AccountID: waitee.AccountID, Endpoint: "https://push.example/abc",
		P256dh: p256dh, Auth: auth,
	})
	require.NoError(t, err)
	require.NoError(t, reconcileWaitees(ctx, h.q, h.tg.Game.ID))

	got := h.activityFromState(0)[waitee.ID]
	assert.Equal(t, model.ReminderScheduled, got.Reminder)
	if assert.NotNil(t, got.ReminderDueAt, "a scheduled reminder must carry its due time") {
		// Seeded accounts default to a 24h cadence.
		assert.WithinDuration(t, time.Now().Add(24*time.Hour), *got.ReminderDueAt, time.Minute)
	}
}

// TestGetGameState_ReportsExhaustedReminder is the end-to-end version of the
// promise this state exists to stop us making: a wait past the give-up horizon
// still has a pending_notifications row, and that row still has a due_at, but
// nothing will ever be sent for it again. Read naively the header would count
// down to a ping that is not coming — and the stale due_at is in the past, so
// it would read as "due shortly", the most wrong answer available.
func TestGetGameState_ReportsExhaustedReminder(t *testing.T) {
	h := newActivityHarness(t, 2)
	ctx := context.Background()
	waitee := h.tg.Players[0] // scene_setting focus player, per seedBase

	p256dh, auth := testSubscriptionKeys(t)
	_, err := h.q.UpsertPushSubscription(ctx, dbgen.UpsertPushSubscriptionParams{
		AccountID: waitee.AccountID, Endpoint: "https://push.example/abc",
		P256dh: p256dh, Auth: auth,
	})
	require.NoError(t, err)
	require.NoError(t, reconcileWaitees(ctx, h.q, h.tg.Game.ID))

	_, execErr := h.pool.Exec(ctx,
		`UPDATE pending_notifications
		 SET first_waiting_at = now() - make_interval(days => $2),
		     due_at = now() - interval '3 days'
		 WHERE player_id = $1`, waitee.ID, reminderGiveUpDays+1)
	require.NoError(t, execErr)

	got := h.activityFromState(0)[waitee.ID]
	assert.Equal(t, model.ReminderExhausted, got.Reminder)
	assert.Nil(t, got.ReminderDueAt,
		"an exhausted wait must not serve its stale due time — there is nothing to count down to")
}

// TestSetReminderMute_SilencesTheCurrentWait drives the bell's endpoint the way
// the profile card does, and checks the flag actually landed on the row rather
// than trusting the response.
func TestSetReminderMute_SilencesTheCurrentWait(t *testing.T) {
	h := newActivityHarness(t, 2)
	ctx := context.Background()
	waitee := h.tg.Players[0] // scene_setting focus player, per seedBase
	require.NoError(t, reconcileWaitees(ctx, h.q, h.tg.Game.ID))

	rec := h.doBody(http.MethodPost, tablePath(h.tg.Game.ID, "reminder-mute"), 0, `{"muted":true}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Muted bool `json:"muted"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.True(t, body.Muted)

	row, err := h.q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)
	assert.True(t, row.Muted)

	rec = h.doBody(http.MethodPost, tablePath(h.tg.Game.ID, "reminder-mute"), 0, `{"muted":false}`)
	require.Equal(t, http.StatusOK, rec.Code)
	row, err = h.q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)
	assert.False(t, row.Muted)
}

// With no reminder pending there is nothing to silence — the table moved on
// between the card being drawn and the bell being tapped. The endpoint answers
// with the quiet it is actually keeping (none), so the card can correct itself
// instead of showing a struck bell that means nothing.
func TestSetReminderMute_ReportsFalseWhenNothingIsPending(t *testing.T) {
	h := newActivityHarness(t, 2)

	// players[1] is not the focus player, so reconcile never gave them a row.
	rec := h.doBody(http.MethodPost, tablePath(h.tg.Game.ID, "reminder-mute"), 1, `{"muted":true}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Muted bool `json:"muted"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Muted, "no pending reminder means nothing was silenced")
}

func TestSetReminderMute_RejectsNonMemberAndBadBody(t *testing.T) {
	h := newActivityHarness(t, 2)

	rec := h.doBody(http.MethodPost, tablePath(h.tg.Game.ID, "reminder-mute"), 0, `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "muted is required, not defaulted to false")

	// A seat at a different table in the same database (openTestDB truncates,
	// so a second harness would wipe this one's game).
	other := newTestGame(t, h.q, 2)
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = h.q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: other.Players[0].AccountID,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, tablePath(h.tg.Game.ID, "reminder-mute"), strings.NewReader(`{"muted":true}`))
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec = httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
