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
// Body: {"username": "...", "code": "...", "email": "..."?}
// Creates the account, opens a session, and sets the cookie.
func CreateAccount(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string  `json:"username"`
			Code     string  `json:"code"`
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
		if body.Code == "" {
			respondErr(w, http.StatusBadRequest, "code is required")
			return
		}

		ctx := r.Context()

		if _, err := q.GetAccountByUsername(ctx, body.Username); err == nil {
			respondErr(w, http.StatusConflict, "username taken")
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			respondErr(w, http.StatusInternalServerError, "could not check username")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(body.Code), bcrypt.DefaultCost)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not hash code")
			return
		}

		account, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: body.Username,
			CodeHash: string(hash),
			Email:    body.Email,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create account")
			return
		}

		if err = openSession(ctx, w, q, account.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not open session")
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
// Body fields are all optional: {"username": ..., "email": ..., "code": ...}.
func UpdateMe(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		var body struct {
			Username *string `json:"username"`
			Email    *string `json:"email"`
			Code     *string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		if body.Username != nil {
			name := strings.TrimSpace(*body.Username)
			if name == "" {
				respondErr(w, http.StatusBadRequest, "username cannot be empty")
				return
			}
			if existing, err := q.GetAccountByUsername(ctx, name); err == nil && existing.ID != acct.ID {
				respondErr(w, http.StatusConflict, "username taken")
				return
			}
			_, err := q.UpdateAccountUsername(ctx, dbgen.UpdateAccountUsernameParams{
				ID:       acct.ID,
				Username: name,
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not update username")
				return
			}
		}

		if body.Email != nil {
			email := strings.TrimSpace(*body.Email)
			var emailPtr *string
			if email != "" {
				emailPtr = &email
			}
			_, err := q.UpdateAccountEmail(ctx, dbgen.UpdateAccountEmailParams{
				ID:    acct.ID,
				Email: emailPtr,
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not update email")
				return
			}
		}

		if body.Code != nil {
			if *body.Code == "" {
				respondErr(w, http.StatusBadRequest, "code cannot be empty")
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(*body.Code), bcrypt.DefaultCost)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not hash code")
				return
			}
			_, err = q.UpdateAccountCode(ctx, dbgen.UpdateAccountCodeParams{
				ID:       acct.ID,
				CodeHash: string(hash),
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not update code")
				return
			}
		}

		updated, err := q.GetAccountByID(ctx, acct.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not reload account")
			return
		}
		respond(w, http.StatusOK, accountResponse(&updated))
	}
}

// ListMyTables handles GET /api/accounts/me/tables.
func ListMyTables(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}
		rows, err := q.ListPlayersByAccount(r.Context(), acct.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not list tables")
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

// openSession creates a sessions row and sets the cookie.
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
