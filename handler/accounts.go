package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

const sessionCookieMaxAge = int(365 * 24 * time.Hour / time.Second)

// maxPasswordBytes is bcrypt's hard limit: bcrypt.GenerateFromPassword
// errors on anything longer, which without this guard surfaces as a
// confusing 500. Not a password-strength policy — there is no minimum.
const maxPasswordBytes = 72

// validNotifyCadenceHours mirrors the accounts.notify_cadence_hours CHECK
// constraint (migration 048) — the five cadence options a player can pick in
// Profile → Notifications. NULL (not in this set) means off.
var validNotifyCadenceHours = map[int16]bool{1: true, 3: true, 8: true, 24: true, 72: true}

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
		username, ok := textField(w, "username", body.Username, maxUsernameLen)
		if !ok {
			return
		}
		body.Username = username
		if body.Username == "" {
			respondErr(w, http.StatusBadRequest, "username is required")
			return
		}
		if body.Password == "" {
			respondErr(w, http.StatusBadRequest, "password is required")
			return
		}
		if len(body.Password) > maxPasswordBytes {
			respondErr(w, http.StatusBadRequest, "password too long (max 72 characters)")
			return
		}
		if body.Email != nil {
			email, ok := textField(w, "email", *body.Email, maxEmailLen)
			if !ok {
				return
			}
			body.Email = &email
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
			"id":                   acct.ID,
			"username":             acct.Username,
			"email":                acct.Email,
			"notify_cadence_hours": acct.NotifyCadenceHours,
			"vapid_public_key":     vapidPublicKey,
		})
	}
}

// UpdateMe handles PATCH /api/accounts/me.
//
// Body fields are all optional: {"username": ..., "email": ..., "password": ...,
// "notify_cadence_hours": ...}. notify_cadence_hours is presence-aware: a
// caller-supplied JSON null ({"notify_cadence_hours": null}) explicitly turns
// notifications off, distinct from omitting the field entirely (which leaves
// the existing cadence untouched) — reading into a typed struct alone can't
// tell those apart, since both decode to a nil pointer, so the raw body is
// also decoded into a map to check key presence.
func UpdateMe(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "could not read body")
			return
		}

		var body struct {
			Username           *string `json:"username"`
			Email              *string `json:"email"`
			Password           *string `json:"password"`
			NotifyCadenceHours *int16  `json:"notify_cadence_hours"`
		}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		var rawFields map[string]json.RawMessage
		_ = json.Unmarshal(bodyBytes, &rawFields)
		_, cadencePresent := rawFields["notify_cadence_hours"]

		ctx := r.Context()

		// Pre-validate inputs outside the transaction so we can return clean
		// 4xx errors without opening a connection. The actual writes (which
		// can partially succeed if any one fails) run atomically below.
		var newUsername *string
		if body.Username != nil {
			name, ok := textField(w, "username", *body.Username, maxUsernameLen)
			if !ok {
				return
			}
			if name == "" {
				respondErr(w, http.StatusBadRequest, "username cannot be empty")
				return
			}
			newUsername = &name
		}
		var newEmail *string
		if body.Email != nil {
			email, ok := textField(w, "email", *body.Email, maxEmailLen)
			if !ok {
				return
			}
			newEmail = &email
		}
		var newPasswordHash *string
		if body.Password != nil {
			hash, ok := hashPasswordField(w, r, *body.Password)
			if !ok {
				return
			}
			newPasswordHash = hash
		}
		if cadencePresent && body.NotifyCadenceHours != nil && !validNotifyCadenceHours[*body.NotifyCadenceHours] {
			respondErr(w, http.StatusBadRequest, "notify_cadence_hours must be one of 1, 3, 8, 24, 72, or null")
			return
		}

		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			return updateAccountFields(ctx, q, acct, newUsername, newEmail, newPasswordHash,
				cadencePresent, body.NotifyCadenceHours)
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

// hashPasswordField validates and bcrypt-hashes a non-empty password update
// for UpdateMe, writing the appropriate 4xx (or 500, on a hash failure) and
// returning ok=false on any error. Split out to keep UpdateMe's branching
// flat — the caller only needs to check ok and return.
func hashPasswordField(w http.ResponseWriter, r *http.Request, password string) (*string, bool) {
	if password == "" {
		respondErr(w, http.StatusBadRequest, "password cannot be empty")
		return nil, false
	}
	if len(password) > maxPasswordBytes {
		respondErr(w, http.StatusBadRequest, "password too long (max 72 characters)")
		return nil, false
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		respondInternalErr(w, r, "could not hash password", err)
		return nil, false
	}
	h := string(hash)
	return &h, true
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
		"id":                   a.ID,
		"username":             a.Username,
		"email":                a.Email,
		"notify_cadence_hours": a.NotifyCadenceHours,
	}
}

// updateAccountFields applies the given account field updates within a
// transaction. cadenceSet distinguishes "notify_cadence_hours omitted" (false
// — leave untouched) from "notify_cadence_hours present in the request body"
// (true — write newCadence, which may itself be nil to turn notifications
// off); see UpdateMe's doc comment for why presence can't be read off
// newCadence alone.
func updateAccountFields(ctx context.Context, q *dbgen.Queries, acct *appMiddleware.Account,
	newUsername, newEmail *string, newPasswordHash *string,
	cadenceSet bool, newCadence *int16,
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
	if cadenceSet {
		if _, err := q.UpdateAccountNotifyCadence(ctx, dbgen.UpdateAccountNotifyCadenceParams{
			ID:                 acct.ID,
			NotifyCadenceHours: newCadence,
		}); err != nil {
			return httpErr(http.StatusInternalServerError, "could not update notification cadence")
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
		Secure:   secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionCookieMaxAge,
	})
	return nil
}
