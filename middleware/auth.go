// Package middleware contains HTTP middleware for the Uneasy server.
package middleware

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	"uneasy/model"
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
func EnsureToken(pool *pgxpool.Pool) func(http.Handler) http.Handler {
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
			ut, err := db.GetUserToken(ctx, pool, cookie.Value)
			if err == nil {
				ctx = context.WithValue(ctx, userTokenKey, &ut)
			}

			// Look up the game seat if they've joined one.
			p, err := db.GetPlayerByToken(ctx, pool, cookie.Value)
			if err == nil {
				ctx = context.WithValue(ctx, playerKey, &p)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserTokenFromContext returns the UserToken stored by EnsureToken, or nil.
func UserTokenFromContext(ctx context.Context) *model.UserToken {
	v, _ := ctx.Value(userTokenKey).(*model.UserToken)
	return v
}

// PlayerFromContext returns the Player stored by EnsureToken, or nil if the
// person hasn't joined a game.
func PlayerFromContext(ctx context.Context) *model.Player {
	v, _ := ctx.Value(playerKey).(*model.Player)
	return v
}

// RawTokenFromContext extracts the raw cookie value from the request context.
// Used by handlers that need the token string itself (e.g. to create a player).
func RawTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie("player_token")
	if err != nil {
		return ""
	}
	return cookie.Value
}
