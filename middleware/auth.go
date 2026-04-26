// Package middleware contains HTTP middleware for the Uneasy server.
package middleware

import (
	"context"
	"net/http"

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
}

// EnsureSession reads the player_token cookie on every request. If a valid
// session exists, the associated account is stored in the request context
// and last_seen is bumped. Never rejects requests — handlers gate access
// explicitly via AccountFromContext / LoadPlayer.
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
				_ = q.TouchSession(ctx, cookie.Value)
				ctx = context.WithValue(ctx, accountKey, &Account{
					ID:       row.AID,
					Username: row.Username,
					Email:    row.Email,
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
