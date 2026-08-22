//go:build integration

package handler

// push_notifications_integration_test.go — DB-driven coverage for the
// Session 3 reconciler (adr/NOTIFICATIONS_PLAN.md): waitee transitions
// against a real driven game, cadence-off/enable-mid-wait, and the full
// send+rebump / 410-prune flow against an httptest.Server relay.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
)

// TestReconcileWaitees_TracksWaiteeTransitions drives a fresh two-player game
// (default cadence 24h on both accounts): the scene_setting focus player
// gets a timer; passing focus clears the old one and starts a new one for
// the new focus player.
func TestReconcileWaitees_TracksWaiteeTransitions(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rows, err := q.ListPendingNotificationsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, tg.Players[0].ID, rows[0].PlayerID)

	// Advance focus to players[1] and reconcile again: the old timer clears,
	// a fresh one starts for the new focus player.
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{ID: tg.Game.ID, FocusPlayerID: &tg.Players[1].ID}))
	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rows, err = q.ListPendingNotificationsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, tg.Players[1].ID, rows[0].PlayerID, "old waitee's timer cleared, new waitee's timer started")
}

// TestReconcileWaitees_NullCadenceInsertsNothing_ThenEnablingMidWaitInserts:
// an account with push off never gets a timer row even while it's a named
// waitee; enabling later while still waiting picks it up on the next tick.
func TestReconcileWaitees_NullCadenceInsertsNothing_ThenEnablingMidWaitInserts(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	waitee := tg.Players[0] // scene_setting focus player, per seedBase

	_, err := q.UpdateAccountNotifyCadence(ctx, dbgen.UpdateAccountNotifyCadenceParams{
		ID: waitee.AccountID, NotifyCadenceHours: nil,
	})
	require.NoError(t, err)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rows, err := q.ListPendingNotificationsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, rows, "cadence off → no timer, even though this player is the named waitee")

	cadence := int16(3)
	_, err = q.UpdateAccountNotifyCadence(ctx, dbgen.UpdateAccountNotifyCadenceParams{
		ID: waitee.AccountID, NotifyCadenceHours: &cadence,
	})
	require.NoError(t, err)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rows, err = q.ListPendingNotificationsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "enabling mid-wait inserts on the next reconcile tick")
	assert.Equal(t, waitee.ID, rows[0].PlayerID)
}

// TestReconcileWaitees_NobodyWaitingClearsEveryTimer exercises the pgx
// nil-vs-empty-slice pitfall directly: once nobody is named (ended phase),
// reconcile must delete every existing timer for the game, not leave them
// running because a nil slice got encoded as SQL NULL.
func TestReconcileWaitees_NobodyWaitingClearsEveryTimer(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rows, err := q.ListPendingNotificationsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{ID: tg.Game.ID, Phase: "ended"}))
	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rows, err = q.ListPendingNotificationsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, rows, "nobody waiting → every timer for the game is cleared")
}

// TestSendDueNotifications_SendsAndRebumps drives a full send: a due
// notification with a real (test-generated) subscription pointing at an
// httptest.Server relay gets sent, and its timer is re-bumped into the
// future afterward.
func TestSendDueNotifications_SendsAndRebumps(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	waitee := tg.Players[0]

	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	_, err = q.UpsertPushSubscription(ctx, dbgen.UpsertPushSubscriptionParams{
		AccountID: waitee.AccountID, Endpoint: srv.URL, P256dh: p256dh, Auth: auth,
	})
	require.NoError(t, err)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	before, err := q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)

	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications SET due_at = now() - interval '1 minute' WHERE player_id = $1`, waitee.ID)
	require.NoError(t, execErr)

	sendDueNotifications(ctx, store, slog.Default())

	assert.Equal(t, 1, hits, "the relay received exactly one push")
	after, err := q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)
	assert.True(t, after.DueAt.Time.After(before.DueAt.Time), "due_at re-bumped forward past its original value")
}

// TestSendDueNotifications_PrunesOn410 exercises the dead-subscription path:
// a relay responding 410 Gone causes the push_subscriptions row to be
// deleted, while the pending_notifications row is still re-bumped (not
// deleted) since the account's cadence is still on.
func TestSendDueNotifications_PrunesOn410(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	waitee := tg.Players[0]

	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	sub, err := q.UpsertPushSubscription(ctx, dbgen.UpsertPushSubscriptionParams{
		AccountID: waitee.AccountID, Endpoint: srv.URL, P256dh: p256dh, Auth: auth,
	})
	require.NoError(t, err)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	before, err := q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)
	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications SET due_at = now() - interval '1 minute' WHERE player_id = $1`, waitee.ID)
	require.NoError(t, execErr)

	sendDueNotifications(ctx, store, slog.Default())

	subs, err := q.ListPushSubscriptionsByAccount(ctx, waitee.AccountID)
	require.NoError(t, err)
	for _, s := range subs {
		assert.NotEqual(t, sub.ID, s.ID, "the 410'd subscription was pruned")
	}

	after, err := q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)
	assert.True(t, after.DueAt.Time.After(before.DueAt.Time),
		"timer still re-bumped forward even with no subscriptions left")
}

// TestRunNotificationTick_EndedGameWithLeftoverRowGetsCleared is the
// regression test for the bug where a game ending mid-wait left its waitees'
// pending_notifications rows behind forever: ListGamesNeedingNotificationReconcile
// only walked non-ended games, so reconcileWaitees (which correctly clears
// every timer once ComputeWaitState returns WaitKindNobody for 'ended') never
// got a chance to run on this game again. Drives the full tick — not
// reconcileWaitees directly — so the game-selection query is exercised too. A
// real subscription behind an httptest relay confirms the ended game's due
// row is cleared without ever being sent.
func TestRunNotificationTick_EndedGameWithLeftoverRowGetsCleared(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	waitee := tg.Players[0] // scene_setting focus player, per seedBase

	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	_, err = q.UpsertPushSubscription(ctx, dbgen.UpsertPushSubscriptionParams{
		AccountID: waitee.AccountID, Endpoint: srv.URL, P256dh: p256dh, Auth: auth,
	})
	require.NoError(t, err)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rows, err := q.ListPendingNotificationsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "waitee has a timer before the game ends")

	// Simulate the row falling due while the game is still live, then the
	// game ending before the next tick fires — the bug scenario.
	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications SET due_at = now() - interval '1 minute' WHERE player_id = $1`, waitee.ID)
	require.NoError(t, execErr)
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{ID: tg.Game.ID, Phase: "ended"}))

	RunNotificationTick(ctx, store, slog.Default())

	assert.Equal(t, 0, hits, "an ended game's leftover due row must never be sent")
	_, err = q.GetPendingNotification(ctx, waitee.ID)
	assert.True(t, errors.Is(err, pgx.ErrNoRows), "ended game's leftover row is cleared, not left to fire forever")
}

// TestSendDueNotifications_DisabledCadenceClearsInsteadOfRebumping: if an
// account's cadence goes NULL between insertion and send, there's nothing to
// re-bump to — the row is deleted outright instead.
func TestSendDueNotifications_DisabledCadenceClearsInsteadOfRebumping(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	waitee := tg.Players[0]

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications SET due_at = now() - interval '1 minute' WHERE player_id = $1`, waitee.ID)
	require.NoError(t, execErr)

	_, err := q.UpdateAccountNotifyCadence(ctx, dbgen.UpdateAccountNotifyCadenceParams{
		ID: waitee.AccountID, NotifyCadenceHours: nil,
	})
	require.NoError(t, err)

	sendDueNotifications(ctx, store, slog.Default())

	_, err = q.GetPendingNotification(ctx, waitee.ID)
	assert.True(t, errors.Is(err, pgx.ErrNoRows), "disabled cadence at send time clears the row instead of re-bumping it")
}

// TestSendDueNotifications_BacksOffAsTheWaitAges pins the backoff curve in
// RebumpPendingNotification: the gap to the next reminder is half the age of
// the wait so far, floored at the player's cadence and capped at 72 hours.
//
// The age is the whole input, which is what makes the curve possible without a
// send counter — a surviving pending_notifications row means this player has
// been blocking this game continuously since first_waiting_at (see that
// query's comment). Driven through sendDueNotifications rather than the query
// directly so the cadence actually comes from the account, as it does in
// production.
func TestSendDueNotifications_BacksOffAsTheWaitAges(t *testing.T) {
	cases := []struct {
		name      string
		ageHours  int
		wantHours float64
		why       string
	}{
		{
			name: "a fresh wait re-bumps by the cadence the player chose",
			// age/2 is still under 24h, so the floor decides. Someone on
			// "once a day" gets their second reminder a day later, not sooner
			// and not later.
			ageHours: 24, wantHours: 24,
			why: "floored at cadence",
		},
		{
			name:     "a wait several days old stretches to half its age",
			ageHours: 120, wantHours: 60,
			why:      "the backoff proper: 1.5x the total wait per reminder",
		},
		{
			name: "a long wait is capped rather than stretching without limit",
			// The cap binds once half the age exceeds 72h, i.e. from day 6
			// until the give-up horizon ends the reminders altogether at day
			// 30 — so 25 days in, where half the age would be nearly a
			// fortnight, the gap is still 72h. Backoff never becomes silence
			// by accident; that is the horizon's job, tested separately.
			ageHours: 600, wantHours: 72,
			why:      "capped at 72h",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := openTestDB(t)
			q := dbgen.New(pool)
			store := db.NewStore(pool)
			ctx := context.Background()
			tg := newTestGame(t, q, 2)
			waitee := tg.Players[0] // scene_setting focus player, per seedBase

			require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))

			// Age the wait and make it due. Both columns move together: a row
			// that has been waiting this long has necessarily had its due_at
			// re-bumped to somewhere inside that window.
			_, execErr := pool.Exec(ctx,
				`UPDATE pending_notifications
				 SET first_waiting_at = now() - make_interval(hours => $2),
				     due_at = now() - interval '1 minute'
				 WHERE player_id = $1`, waitee.ID, tc.ageHours)
			require.NoError(t, execErr)

			sendDueNotifications(ctx, store, slog.Default())

			after, err := q.GetPendingNotification(ctx, waitee.ID)
			require.NoError(t, err)
			gotHours := time.Until(after.DueAt.Time).Hours()
			assert.InDelta(t, tc.wantHours, gotHours, 0.5,
				"a %dh-old wait on a 24h cadence should next fire in ~%.0fh (%s)",
				tc.ageHours, tc.wantHours, tc.why)
		})
	}
}

// TestSendDueNotifications_StopsAfterGiveUpHorizon is the regression test for
// the reason all of this exists: a table the group has drifted away from used
// to ping its last waitee at their cadence forever, because the only things
// that ever clear a timer are the player acting and the game ending, and an
// abandoned game does neither.
//
// Past reminderGiveUpDays the row is no longer selected at all — not sent, and
// deliberately not re-bumped either, so its due_at stays in the past as the
// signal that this wait has been given up on (reminderState reads exactly that
// to report ReminderExhausted).
func TestSendDueNotifications_StopsAfterGiveUpHorizon(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	waitee := tg.Players[0]

	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	_, err = q.UpsertPushSubscription(ctx, dbgen.UpsertPushSubscriptionParams{
		AccountID: waitee.AccountID, Endpoint: srv.URL, P256dh: p256dh, Auth: auth,
	})
	require.NoError(t, err)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications
		 SET first_waiting_at = now() - make_interval(days => $2),
		     due_at = now() - interval '1 minute'
		 WHERE player_id = $1`, waitee.ID, reminderGiveUpDays+1)
	require.NoError(t, execErr)
	before, err := q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)

	sendDueNotifications(ctx, store, slog.Default())

	assert.Equal(t, 0, hits, "a wait past the give-up horizon is never sent")
	after, err := q.GetPendingNotification(ctx, waitee.ID)
	require.NoError(t, err)
	assert.Equal(t, before.DueAt.Time, after.DueAt.Time,
		"the row is left untouched — not re-bumped — so it stays inert and readable as exhausted")
	assert.True(t, after.DueAt.Time.Before(time.Now()),
		"an exhausted row keeps a due_at in the past, which is what reminderState keys on")
}

// TestReconcileWaitees_ActingResetsTheBackoffClock: the backoff and the
// give-up both read first_waiting_at, so a player who acts must get a genuinely
// new row rather than an aged one — otherwise a revived table would carry its
// old silence forward and keep under-reminding (or not reminding at all) the
// players who came back to it.
//
// This falls out of the reconciler's existing shape rather than needing a reset
// path of its own: leaving the waitee set deletes the row, and re-entering it
// inserts a fresh one with first_waiting_at defaulting to now.
func TestReconcileWaitees_ActingResetsTheBackoffClock(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications SET first_waiting_at = now() - interval '20 days'
		 WHERE player_id = $1`, tg.Players[0].ID)
	require.NoError(t, execErr)

	// The wait ends (focus passes), then this player blocks the table again.
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{ID: tg.Game.ID, FocusPlayerID: &tg.Players[1].ID}))
	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{ID: tg.Game.ID, FocusPlayerID: &tg.Players[0].ID}))
	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))

	row, err := q.GetPendingNotification(ctx, tg.Players[0].ID)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), row.FirstWaitingAt.Time, time.Minute,
		"a new wait starts a new clock — the 20-day-old one must not carry over")
}

// TestSendDueNotifications_MutedWaitIsNeverSent: the profile card's bell sets
// pending_notifications.muted, and a muted row is skipped by the send query
// while still being re-bumped by nothing at all — it simply sits there, silent,
// for as long as the wait lasts.
func TestSendDueNotifications_MutedWaitIsNeverSent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	waitee := tg.Players[0] // scene_setting focus player, per seedBase

	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	_, err = q.UpsertPushSubscription(ctx, dbgen.UpsertPushSubscriptionParams{
		AccountID: waitee.AccountID, Endpoint: srv.URL, P256dh: p256dh, Auth: auth,
	})
	require.NoError(t, err)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	rowsAffected, err := q.SetPendingNotificationMuted(ctx, dbgen.SetPendingNotificationMutedParams{
		PlayerID: waitee.ID, Muted: true,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rowsAffected)
	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications SET due_at = now() - interval '1 minute' WHERE player_id = $1`, waitee.ID)
	require.NoError(t, execErr)

	sendDueNotifications(ctx, store, slog.Default())
	assert.Equal(t, 0, hits, "a muted wait sends nothing")

	// Un-muting resumes it, without needing the timer rebuilt.
	_, err = q.SetPendingNotificationMuted(ctx, dbgen.SetPendingNotificationMutedParams{
		PlayerID: waitee.ID, Muted: false,
	})
	require.NoError(t, err)
	sendDueNotifications(ctx, store, slog.Default())
	assert.Equal(t, 1, hits, "un-muting resumes the same wait's reminders")
}

// TestReconcileWaitees_MuteSurvivesTicksButNotTheWait pins the two halves of
// the per-wait scope, which is the whole reason the flag lives on this row
// rather than on players:
//
//   - The reconciler runs every tick and must not clear a mute (its upsert is
//     ON CONFLICT DO NOTHING, so the row — flag and all — is left alone).
//   - When the table finally moves past this player their row is deleted, so
//     the next time it comes back to them they are audible again, with nobody
//     having had to remember to un-mute. A table that never moves stays quiet,
//     which is the case the bell exists for.
func TestReconcileWaitees_MuteSurvivesTicksButNotTheWait(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)

	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	_, err := q.SetPendingNotificationMuted(ctx, dbgen.SetPendingNotificationMutedParams{
		PlayerID: tg.Players[0].ID, Muted: true,
	})
	require.NoError(t, err)

	// Many ticks pass with the table still stuck on the same player.
	for range 3 {
		require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	}
	row, err := q.GetPendingNotification(ctx, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, row.Muted, "reconciling must not un-mute a wait that is still going")

	// The table moves on, then comes back round to them.
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{ID: tg.Game.ID, FocusPlayerID: &tg.Players[1].ID}))
	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{ID: tg.Game.ID, FocusPlayerID: &tg.Players[0].ID}))
	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))

	row, err = q.GetPendingNotification(ctx, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, row.Muted, "a new wait is a new decision — the old mute must not carry over")
}

// TestSetPendingNotificationMuted_NoPendingRowMutesNothing: the game can move on
// between the profile page drawing a bell and the player tapping it. There is
// then nothing to silence, and the handler reports that rather than claiming a
// quiet it isn't keeping.
func TestSetPendingNotificationMuted_NoPendingRowMutesNothing(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)

	rowsAffected, err := q.SetPendingNotificationMuted(ctx, dbgen.SetPendingNotificationMutedParams{
		PlayerID: tg.Players[1].ID, Muted: true, // never a waitee, so never a row
	})
	require.NoError(t, err)
	assert.EqualValues(t, 0, rowsAffected)
}
