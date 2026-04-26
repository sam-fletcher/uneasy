package handler

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

// CreateSession handles POST /api/sessions (log in).
//
// Body: {"username": "...", "code": "..."}
// On success: opens a session, sets the cookie, returns the account.
func CreateSession(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Code     string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Username == "" || body.Code == "" {
			respondErr(w, http.StatusBadRequest, "username and code are required")
			return
		}

		ctx := r.Context()
		account, err := q.GetAccountByUsername(ctx, body.Username)
		if err != nil {
			respondErr(w, http.StatusUnauthorized, "incorrect username or code")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(account.CodeHash), []byte(body.Code)) != nil {
			respondErr(w, http.StatusUnauthorized, "incorrect username or code")
			return
		}

		if err := openSession(ctx, w, q, account.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not open session")
			return
		}
		respond(w, http.StatusOK, accountResponse(&account))
	}
}

// DeleteSession handles DELETE /api/sessions (log out current device).
func DeleteSession(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := appMiddleware.RawTokenFromRequest(r)
		if token != "" {
			_ = q.DeleteSession(r.Context(), token)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "player_token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}
