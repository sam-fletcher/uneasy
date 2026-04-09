// Package middleware contains HTTP middleware for the Uneasy server.
package middleware

import (
	"context"
	"net/http"

	dbgen "uneasy/db/gen"
)

// Context keys — unexported type prevents collisions with other packages.
type contextKey string

const (
	userTokenKey contextKey = "user_token"
	playerKey    contextKey = "player"
)

// EnsureToken reads the player_token cookie on every request. If present, it
// looks up the user_token row and (if the person has joined a game) the player
// row, then stores both in the request context.
//
// This middleware never rejects requests — endpoints that require a token or
// game membership must check explicitly using UserTokenFromContext /
// PlayerFromContext.
func EnsureToken(q *dbgen.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("player_token")
			if err != nil {
				// No cookie — continue without identity context.
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()

			// Look up the pre-game identity.
			ut, err := q.GetUserToken(ctx, cookie.Value)
			if err == nil {
				ctx = context.WithValue(ctx, userTokenKey, &ut)
			}

			// Look up the game seat if they've joined one.
			p, err := q.GetPlayerByToken(ctx, cookie.Value)
			if err == nil {
				ctx = context.WithValue(ctx, playerKey, &p)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserTokenFromContext returns the UserToken stored by EnsureToken, or nil.
func UserTokenFromContext(ctx context.Context) *dbgen.UserToken {
	v, _ := ctx.Value(userTokenKey).(*dbgen.UserToken)
	return v
}

// PlayerFromContext returns the Player stored by EnsureToken, or nil if the
// person hasn't joined a game.
func PlayerFromContext(ctx context.Context) *dbgen.Player {
	v, _ := ctx.Value(playerKey).(*dbgen.Player)
	return v
}

// RawTokenFromRequest extracts the raw cookie value from the request.
// Used by handlers that need the token string itself (e.g. to create a player).
func RawTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("player_token")
	if err != nil {
		return ""
	}
	return cookie.Value
}
