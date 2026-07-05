package handler

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	appMiddleware "uneasy/middleware"
)

// CreateSession handles POST /api/sessions (log in).
//
// Body: {"username": "...", "password": "..."}
// On success: opens a session, sets the cookie, returns the account.
func CreateSession(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Username == "" || body.Password == "" {
			respondErr(w, http.StatusBadRequest, "username and password are required")
			return
		}

		ctx := r.Context()
		account, err := s.Q.GetAccountByUsername(ctx, body.Username)
		if err != nil {
			respondErr(w, http.StatusUnauthorized, "incorrect username or password")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(body.Password)) != nil {
			respondErr(w, http.StatusUnauthorized, "incorrect username or password")
			return
		}

		if err := openSession(ctx, w, s.Q, account.ID); err != nil {
			respondInternalErr(w, r, "could not open session", err)
			return
		}
		respond(w, http.StatusOK, accountResponse(&account))
	}
}

// DeleteSession handles DELETE /api/sessions (log out current device).
func DeleteSession(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := appMiddleware.RawTokenFromRequest(r)
		if token != "" {
			_ = s.Q.DeleteSession(r.Context(), token)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "player_token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   secureCookies,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	}
}
