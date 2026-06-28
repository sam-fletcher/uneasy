package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

const sessionCookieMaxAge = int(365 * 24 * time.Hour / time.Second)

// CreateAccount handles POST /api/accounts.
//
// Body: {"username": "...", "password": "...", "email": "..."?}
// Creates the account, opens a session, and sets the cookie.
func CreateAccount(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string  `json:"username"`
			Password string  `json:"password"`
			Email    *string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Username = strings.TrimSpace(body.Username)
		if body.Username == "" {
			respondErr(w, http.StatusBadRequest, "username is required")
			return
		}
		if body.Password == "" {
			respondErr(w, http.StatusBadRequest, "password is required")
			return
		}

		ctx := r.Context()

		if _, err := s.Q.GetAccountByUsername(ctx, body.Username); err == nil {
			respondErr(w, http.StatusConflict, "username taken")
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			respondErr(w, http.StatusInternalServerError, "could not check username")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			respondInternalErr(w, r, "could not hash password", err)
			return
		}

		account, err := s.Q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username:     body.Username,
			PasswordHash: string(hash),
			Email:        body.Email,
		})
		if err != nil {
			respondInternalErr(w, r, "could not create account", err)
			return
		}

		if err = openSession(ctx, w, s.Q, account.ID); err != nil {
			respondInternalErr(w, r, "could not open session", err)
			return
		}

		respond(w, http.StatusCreated, accountResponse(&account))
	}
}

// GetMe handles GET /api/accounts/me.
func GetMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}
		respond(w, http.StatusOK, map[string]any{
			"id":       acct.ID,
			"username": acct.Username,
			"email":    acct.Email,
		})
	}
}

// UpdateMe handles PATCH /api/accounts/me.
//
// Body fields are all optional: {"username": ..., "email": ..., "password": ...}.
func UpdateMe(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		var body struct {
			Username *string `json:"username"`
			Email    *string `json:"email"`
			Password *string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		// Pre-validate inputs outside the transaction so we can return clean
		// 4xx errors without opening a connection. The actual writes (which
		// can partially succeed if any one fails) run atomically below.
		var newUsername *string
		if body.Username != nil {
			name := strings.TrimSpace(*body.Username)
			if name == "" {
				respondErr(w, http.StatusBadRequest, "username cannot be empty")
				return
			}
			newUsername = &name
		}
		var newEmail *string
		if body.Email != nil {
			email := strings.TrimSpace(*body.Email)
			if email != "" {
				newEmail = &email
			} else {
				empty := ""
				newEmail = &empty
			}
		}
		var newPasswordHash *string
		if body.Password != nil {
			if *body.Password == "" {
				respondErr(w, http.StatusBadRequest, "password cannot be empty")
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), bcrypt.DefaultCost)
			if err != nil {
				respondInternalErr(w, r, "could not hash password", err)
				return
			}
			h := string(hash)
			newPasswordHash = &h
		}

		err := s.InTx(ctx, func(q *dbgen.Queries) error {
			return updateAccountFields(ctx, q, acct, newUsername, newEmail, newPasswordHash)
		})
		if err != nil {
			respondHTTPErr(w, r, err)
			return
		}

		updated, err := s.Q.GetAccountByID(ctx, acct.ID)
		if err != nil {
			respondInternalErr(w, r, "could not reload account", err)
			return
		}
		respond(w, http.StatusOK, accountResponse(&updated))
	}
}

// ListMyTables handles GET /api/accounts/me/tables.
func ListMyTables(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}
		rows, err := s.Q.ListPlayersByAccount(r.Context(), acct.ID)
		if err != nil {
			respondInternalErr(w, r, "could not list tables", err)
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, map[string]any{
				"game_id":        row.GameID,
				"join_code":      row.JoinCode,
				"is_facilitator": row.IsFacilitator,
				"joined_at":      row.JoinedAt,
			})
		}
		respond(w, http.StatusOK, map[string]any{"tables": out})
	}
}

func accountResponse(a *dbgen.Account) map[string]any {
	return map[string]any{
		"id":       a.ID,
		"username": a.Username,
		"email":    a.Email,
	}
}

// updateAccountFields applies the given account field updates within a transaction.
// It returns the appropriate HTTP status code and any error encountered.
func updateAccountFields(ctx context.Context, q *dbgen.Queries, acct *appMiddleware.Account,
	newUsername, newEmail *string, newPasswordHash *string,
) error {
	if newUsername != nil {
		if existing, err := q.GetAccountByUsername(ctx, *newUsername); err == nil && existing.ID != acct.ID {
			return httpErr(http.StatusConflict, "username taken")
		}
		if _, err := q.UpdateAccountUsername(ctx, dbgen.UpdateAccountUsernameParams{
			ID:       acct.ID,
			Username: *newUsername,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update username")
		}
		// players.display_name is a snapshot taken at join time, so propagate
		// the rename to every seat this account holds across in-progress games.
		if err := q.UpdateDisplayNameByAccount(ctx, dbgen.UpdateDisplayNameByAccountParams{
			AccountID:   acct.ID,
			DisplayName: *newUsername,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update player names")
		}
	}
	if newEmail != nil {
		var emailPtr *string
		if *newEmail != "" {
			emailPtr = newEmail
		}
		if _, err := q.UpdateAccountEmail(ctx, dbgen.UpdateAccountEmailParams{
			ID:    acct.ID,
			Email: emailPtr,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update email")
		}
	}
	if newPasswordHash != nil {
		if _, err := q.UpdateAccountPassword(ctx, dbgen.UpdateAccountPasswordParams{
			ID:           acct.ID,
			PasswordHash: *newPasswordHash,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update password")
		}
	}
	return nil
}

// openSession creates a sessions row and sets the cookie. Internal helper
// shared by CreateAccount, sessions.go, and dev.go; takes *dbgen.Queries
// directly so callers inside a transaction can pass their transactional
// handle if needed.
func openSession(ctx context.Context, w http.ResponseWriter, q *dbgen.Queries, accountID int64) error {
	token, err := db.NewCookieToken()
	if err != nil {
		return err
	}
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{
		Token:     token,
		AccountID: accountID,
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "player_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionCookieMaxAge,
	})
	return nil
}
