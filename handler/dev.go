package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/gametest"
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

// DevSeed handles POST /api/dev/seed.
//
// Body shape:
//
//	{ "phase": "main_event", "players": ["alice", "bob"] }
//
// Creates a game in the requested phase, seating the named accounts as
// players (creating accounts that don't exist). Used by the Playwright
// E2E suite to fast-forward past phases it isn't testing right now.
// Mounted only when UNEASY_DEV=1.
//
// The fixture logic lives in package gametest, shared with the handler
// integration tests so the two suites can't disagree about what a valid
// game in phase X looks like.
func DevSeed(s *db.Store) http.HandlerFunc {
	type request struct {
		Phase   string   `json:"phase"`
		Players []string `json:"players"`
	}
	type playerResp struct {
		ID            int64  `json:"id"`
		AccountID     int64  `json:"account_id"`
		DisplayName   string `json:"display_name"`
		IsFacilitator bool   `json:"is_facilitator"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		var seeded gametest.SeededGame
		var err error
		switch body.Phase {
		case "main_event":
			seeded, err = gametest.SeedMainEvent(ctx, s.Q, body.Players)
		default:
			respondErr(w, http.StatusBadRequest, "unknown phase: "+body.Phase)
			return
		}
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}

		players := make([]playerResp, len(seeded.Players))
		for i, p := range seeded.Players {
			players[i] = playerResp{
				ID:            p.ID,
				AccountID:     p.AccountID,
				DisplayName:   p.DisplayName,
				IsFacilitator: p.IsFacilitator,
			}
		}
		respond(w, http.StatusOK, map[string]any{
			"game_id":   seeded.Game.ID,
			"join_code": seeded.Game.JoinCode,
			"phase":     string(seeded.Game.Phase),
			"players":   players,
		})
	}
}
