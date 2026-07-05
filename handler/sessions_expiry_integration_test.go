//go:build integration

// handler/sessions_expiry_integration_test.go — coverage for the Session-2
// server-side session expiry: GetSessionWithAccount must stop resolving a
// session once its last_seen is more than 365 days stale, and
// DeleteExpiredSessions must actually remove such rows.

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
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
