package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/gametest"
	"uneasy/model"
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

// shakeUpStepFromName maps the JSON shake_up_step name to its int16 constant.
func shakeUpStepFromName(name string) (int16, bool) {
	switch name {
	case "rolling":
		return gamepkg.ShakeUpStepRolling, true
	case "spending":
		return gamepkg.ShakeUpStepSpending, true
	default:
		return 0, false
	}
}

// DevSeed handles POST /api/dev/seed.
//
// Body shape (only phase + players are required):
//
//	{
//	  "phase": "main_event",            // or "shake_up"
//	  "players": ["alice", "bob"],
//	  "current_row": 9,                 // optional
//	  "rankings": {                     // optional; per-category player order,
//	    "power": [1, 0]                 //   value[k] = player index at rank k+1
//	  },
//	  "plans": [                        // optional
//	    {"preparer_idx": 0, "plan_type": "make_war",
//	     "category": "power", "row": 9, "row_order": 0}
//	  ],
//	  "shake_up_tokens": 5,             // optional (shake_up only)
//	  "shake_up_step": "spending"       // optional (shake_up only): rolling|spending
//	}
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
	type planReq struct {
		PreparerIdx int    `json:"preparer_idx"`
		PlanType    string `json:"plan_type"`
		Category    string `json:"category"`
		Row         int16  `json:"row"`
		RowOrder    int16  `json:"row_order"`
	}
	type request struct {
		Phase         string           `json:"phase"`
		Players       []string         `json:"players"`
		CurrentRow    *int16           `json:"current_row"`
		Rankings      map[string][]int `json:"rankings"`
		Plans         []planReq        `json:"plans"`
		ShakeUpTokens *int16           `json:"shake_up_tokens"`
		ShakeUpStep   *string          `json:"shake_up_step"`
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

		opts := make([]gametest.Option, 0)
		if body.CurrentRow != nil {
			opts = append(opts, gametest.WithCurrentRow(*body.CurrentRow))
		}
		for cat, order := range body.Rankings {
			opts = append(opts, gametest.WithRankings(model.RankingCategory(cat), order))
		}
		for _, p := range body.Plans {
			opts = append(opts, gametest.WithPlan(gametest.SeedPlan{
				PreparerIdx: p.PreparerIdx,
				PlanType:    model.PlanType(p.PlanType),
				Category:    model.RankingCategory(p.Category),
				Row:         p.Row,
				RowOrder:    p.RowOrder,
			}))
		}
		if body.ShakeUpTokens != nil {
			opts = append(opts, gametest.WithShakeUpTokens(*body.ShakeUpTokens))
		}
		if body.ShakeUpStep != nil {
			step, ok := shakeUpStepFromName(*body.ShakeUpStep)
			if !ok {
				respondErr(w, http.StatusBadRequest, "unknown shake_up_step: "+*body.ShakeUpStep)
				return
			}
			opts = append(opts, gametest.WithShakeUpStep(step))
		}

		ctx := r.Context()
		var seeded gametest.SeededGame
		var err error
		switch body.Phase {
		case "main_event":
			seeded, err = gametest.SeedMainEvent(ctx, s.Q, body.Players, opts...)
		case "shake_up":
			seeded, err = gametest.SeedShakeUp(ctx, s.Q, body.Players, opts...)
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
