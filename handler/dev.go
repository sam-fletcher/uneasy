package handler

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
)

// DevLogin handles POST /api/dev/login?username=foo.
//
// Skips code verification: looks up (or creates) the account by username
// and opens a session. Mounted only when UNEASY_DEV=1.
func DevLogin(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		if username == "" {
			respondErr(w, http.StatusBadRequest, "username query param required")
			return
		}

		ctx := r.Context()
		account, err := s.Q.GetAccountByUsername(ctx, username)
		if errors.Is(err, pgx.ErrNoRows) {
			hash, _ := bcrypt.GenerateFromPassword([]byte("dev"), bcrypt.MinCost)
			account, err = s.Q.CreateAccount(ctx, dbgen.CreateAccountParams{
				Username: username,
				CodeHash: string(hash),
			})
		}
		if err != nil {
			respondInternalErr(w, r, "could not get/create account", err)
			return
		}

		if err = openSession(ctx, w, s.Q, account.ID); err != nil {
			respondInternalErr(w, r, "could not open session", err)
			return
		}
		respond(w, http.StatusOK, accountResponse(&account))
	}
}

// DevReset handles POST /api/dev/reset. Wipes all game and account data.
// Schema (migrations) is left untouched.
func DevReset(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.Q.DevWipe(r.Context()); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
