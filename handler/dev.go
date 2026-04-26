package handler

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	dbgen "uneasy/db/gen"
)

// DevLogin handles POST /api/dev/login?username=foo.
//
// Skips code verification: looks up (or creates) the account by username
// and opens a session. Mounted only when UNEASY_DEV=1.
func DevLogin(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		if username == "" {
			respondErr(w, http.StatusBadRequest, "username query param required")
			return
		}

		ctx := r.Context()
		account, err := q.GetAccountByUsername(ctx, username)
		if errors.Is(err, pgx.ErrNoRows) {
			hash, _ := bcrypt.GenerateFromPassword([]byte("dev"), bcrypt.MinCost)
			account, err = q.CreateAccount(ctx, dbgen.CreateAccountParams{
				Username: username,
				CodeHash: string(hash),
			})
		}
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not get/create account")
			return
		}

		if err = openSession(ctx, w, q, account.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not open session")
			return
		}
		respond(w, http.StatusOK, accountResponse(&account))
	}
}

// DevReset handles POST /api/dev/reset. Wipes all game and account data.
// Schema (migrations) is left untouched.
func DevReset(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := q.DevWipe(r.Context()); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
