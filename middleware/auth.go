// Package middleware contains HTTP middleware for the Uneasy server.
package middleware

import (
	"context"
	"net/http"
	"time"

	dbgen "uneasy/db/gen"
)

type contextKey string

const (
	accountKey contextKey = "account"
)

// Account is the request-scoped view of the logged-in account, hydrated
// from the sessions+accounts join.
type Account struct {
	ID       int64
	Username string
	Email    *string
	// NotifyCadenceHours is the account's web-push reminder cadence
	// (adr/NOTIFICATIONS_PLAN.md); nil means notifications are off.
	NotifyCadenceHours *int16
}

// sessionTouchInterval is how stale last_seen may get before EnsureSession
// writes it back.
//
// last_seen has exactly one consumer: the 365-day session expiry, enforced by
// GetSessionWithAccount's WHERE clause and swept by DeleteExpiredSessions.
// Nothing renders it and nothing else reads it — presence is tracked
// separately, in the hub. So bumping it on every single authenticated request
// bought no accuracy that anything could observe, while costing a synchronous
// Postgres WRITE on every request, including reads. An hour of slack against a
// 365-day window is immaterial to the expiry, and drops those writes by
// something like three orders of magnitude on an active table — which matters
// twice over on a serverless database billed by compute time.
const sessionTouchInterval = time.Hour

// EnsureSession reads the player_token cookie on every request. If a valid
// session exists, the associated account is stored in the request context
// and last_seen is bumped (at most once per sessionTouchInterval). Never
// rejects requests — handlers gate access explicitly via AccountFromContext /
// LoadPlayer.
func EnsureSession(q *dbgen.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("player_token")
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			row, err := q.GetSessionWithAccount(ctx, cookie.Value)
			if err == nil {
				// GetSessionWithAccount already returned last_seen, so the
				// staleness check needs no extra round trip. A zero/invalid
				// timestamp reads as infinitely stale and writes, which is the
				// safe direction: worst case we do what the old code always did.
				if !row.LastSeen.Valid || time.Since(row.LastSeen.Time) > sessionTouchInterval {
					_ = q.TouchSession(ctx, cookie.Value)
				}
				ctx = context.WithValue(ctx, accountKey, &Account{
					ID:                 row.AID,
					Username:           row.Username,
					Email:              row.Email,
					NotifyCadenceHours: row.NotifyCadenceHours,
				})
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AccountFromContext returns the logged-in account, or nil.
func AccountFromContext(ctx context.Context) *Account {
	v, _ := ctx.Value(accountKey).(*Account)
	return v
}

// AccountContext returns a copy of ctx carrying acct, as if EnsureSession had
// resolved it from a cookie. For handler unit tests that need to simulate a
// logged-in request without a real session/cookie round-trip.
func AccountContext(ctx context.Context, acct *Account) context.Context {
	return context.WithValue(ctx, accountKey, acct)
}

// LoadPlayer returns the player row for the given account at the given
// game, or nil if the account is not seated at that table.
func LoadPlayer(ctx context.Context, q *dbgen.Queries, accountID, gameID int64) *dbgen.Player {
	p, err := q.GetPlayerByAccountAndGame(ctx, dbgen.GetPlayerByAccountAndGameParams{
		AccountID: accountID,
		GameID:    gameID,
	})
	if err != nil {
		return nil
	}
	return &p
}

// RawTokenFromRequest extracts the raw cookie value from the request.
func RawTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("player_token")
	if err != nil {
		return ""
	}
	return cookie.Value
}
