//go:build integration

// handler/sessions_expiry_integration_test.go — coverage for the Session-2
// server-side session expiry: GetSessionWithAccount must stop resolving a
// session once its last_seen is more than 365 days stale, and
// DeleteExpiredSessions must actually remove such rows.

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

func TestExpiredSessionIsExcludedAndDeleted(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(t)
	q := dbgen.New(pool)

	account, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username:     "expiry-" + randSuffix(),
		PasswordHash: "not-a-real-hash",
	})
	require.NoError(t, err)

	token, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: token, AccountID: account.ID})
	require.NoError(t, err)

	// A fresh session resolves normally.
	_, err = q.GetSessionWithAccount(ctx, token)
	require.NoError(t, err)

	// Backdate last_seen past the 365-day cutoff, as if the account had been
	// abandoned for over a year (TouchSession never ran again to refresh it).
	_, err = pool.Exec(ctx,
		`UPDATE sessions SET last_seen = now() - interval '366 days' WHERE token = $1`, token)
	require.NoError(t, err)

	_, err = q.GetSessionWithAccount(ctx, token)
	require.Error(t, err, "a session stale for over 365 days must not resolve via the accounts join")

	require.NoError(t, q.DeleteExpiredSessions(ctx))

	var remaining int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM sessions WHERE token = $1`, token).Scan(&remaining))
	require.Equal(t, 0, remaining, "DeleteExpiredSessions must remove the stale row outright")
}

func TestActiveSessionSurvivesExpiredSessionsCleanup(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(t)
	q := dbgen.New(pool)

	account, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username:     "active-" + randSuffix(),
		PasswordHash: "not-a-real-hash",
	})
	require.NoError(t, err)

	token, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: token, AccountID: account.ID})
	require.NoError(t, err)

	require.NoError(t, q.DeleteExpiredSessions(ctx))

	_, err = q.GetSessionWithAccount(ctx, token)
	require.NoError(t, err, "a recently-touched session must survive the cleanup sweep")
}

// EnsureSession used to write last_seen on every authenticated request. It now
// writes at most once an hour (middleware.sessionTouchInterval), because the
// only thing that reads last_seen is the 365-day expiry above — so a
// synchronous write per request bought nothing observable.
//
// The risk that buys is real though: throttle it wrongly and an actively-used
// session silently stops being refreshed, then expires out from under a player
// who never left. Both directions are pinned here.
func TestEnsureSessionThrottlesLastSeenWrites(t *testing.T) {
	ctx := context.Background()
	pool := openTestDB(t)
	q := dbgen.New(pool)

	account, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username:     "touch-" + randSuffix(),
		PasswordHash: "not-a-real-hash",
	})
	require.NoError(t, err)

	token, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: token, AccountID: account.ID})
	require.NoError(t, err)

	lastSeen := func() time.Time {
		var ts time.Time
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT last_seen FROM sessions WHERE token = $1`, token).Scan(&ts))
		return ts
	}
	// Drive the real middleware, so this tracks EnsureSession rather than a
	// reimplementation of its rule.
	var sawAccount bool
	handler := appMiddleware.EnsureSession(q)(http.HandlerFunc(
		func(_ http.ResponseWriter, r *http.Request) {
			sawAccount = appMiddleware.AccountFromContext(r.Context()) != nil
		}))
	request := func() {
		sawAccount = false
		req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
		req.AddCookie(&http.Cookie{Name: "player_token", Value: token})
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	// Fresh session: resolved, but not rewritten.
	before := lastSeen()
	request()
	require.True(t, sawAccount, "a valid session must still resolve the account")
	require.Equal(t, before, lastSeen(), "a session touched moments ago must not be written again")

	// Several more requests in quick succession — still no write. This is the
	// case that used to cost one write per request.
	for i := 0; i < 5; i++ {
		request()
	}
	require.Equal(t, before, lastSeen(), "repeated requests inside the interval must not write")

	// Past the interval: the write must happen, or an active session would
	// drift towards the 365-day cutoff and eventually expire mid-use.
	_, err = pool.Exec(ctx,
		`UPDATE sessions SET last_seen = now() - interval '2 hours' WHERE token = $1`, token)
	require.NoError(t, err)
	stale := lastSeen()

	request()
	require.True(t, sawAccount)
	refreshed := lastSeen()
	require.True(t, refreshed.After(stale),
		"a session older than the touch interval must be refreshed (was %s, now %s)", stale, refreshed)
	require.WithinDuration(t, time.Now(), refreshed, time.Minute,
		"the refresh must set last_seen to now, not merely nudge it")
}
