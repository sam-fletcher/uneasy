package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

// SetIdentity handles POST /api/identity.
//
// If the request has no player_token cookie, a new one is generated and set.
// The display_name in the request body is upserted against the token.
// Returns the user token object (without the raw token value).
func SetIdentity(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.DisplayName == "" {
			respondErr(w, http.StatusBadRequest, "display_name is required")
			return
		}

		// Get or generate the cookie token.
		token := appMiddleware.RawTokenFromRequest(r)
		if token == "" {
			var err error
			token, err = db.NewCookieToken()
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not generate token")
				return
			}
			// Set a 1-year cookie.
			http.SetCookie(w, &http.Cookie{
				Name:     "player_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   int(365 * 24 * time.Hour / time.Second),
			})
		}

		ut, err := q.UpsertUserToken(r.Context(), dbgen.UpsertUserTokenParams{
			Token:       token,
			DisplayName: body.DisplayName,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save identity")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"display_name": ut.DisplayName,
			"created_at":   ut.CreatedAt,
		})
	}
}

// GetIdentity handles GET /api/identity.
//
// Returns the current player's identity from the cookie. 401 if no cookie or
// unrecognised token.
func GetIdentity(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ut := appMiddleware.UserTokenFromContext(r.Context())
		if ut == nil {
			respondErr(w, http.StatusUnauthorized, "no identity — POST /api/identity first")
			return
		}

		// Also include game membership if they've joined one.
		player := appMiddleware.PlayerFromContext(r.Context())

		respond(w, http.StatusOK, map[string]any{
			"display_name": ut.DisplayName,
			"created_at":   ut.CreatedAt,
			"player":       player, // null if not in a game
		})
	}
}
