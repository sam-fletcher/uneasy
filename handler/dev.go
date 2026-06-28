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
	"uneasy/hub"
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

// DevDeleteGame handles POST /api/dev/delete-game.
//
// Hard-deletes a single game and all of its data (cascade, migration 039),
// leaving accounts and every other game intact. Replaces the old DevReset,
// which TRUNCATEd everything — too blunt and too easy to fire by accident.
//
// Body: { "game_id": N }. Mounted only when UNEASY_DEV=1.
func DevDeleteGame(s *db.Store) http.HandlerFunc {
	type request struct {
		GameID *int64 `json:"game_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.GameID == nil {
			respondErr(w, http.StatusBadRequest, "game_id required")
			return
		}

		rows, err := s.Q.DeleteGame(r.Context(), *body.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not delete game", err)
			return
		}
		if rows == 0 {
			respondErr(w, http.StatusNotFound, "game not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DevAdvanceRow handles POST /api/dev/advance-row.
//
// Jumps a game's current_row directly, the generic half of "jump to a plan's
// resolution" for manual testing: seed a main_event game, prepare a plan
// through the real UI (so its bespoke prep state is captured faithfully), then
// call this to skip the intervening rows and land on the plan's row. The plan
// stays pending — resolution kickoff lives in the row-advance path, which a
// direct row-set bypasses — so you click "resolve" in the UI, exercising that
// trigger too.
//
// Body (give either plan_id, or game_id + row):
//
//	{ "plan_id": 123 }            // jump that plan's game to the plan's row
//	{ "game_id": 5, "row": 9 }    // jump game 5 to row 9
//
// Mounted only when UNEASY_DEV=1.
func DevAdvanceRow(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	type request struct {
		PlanID *int64 `json:"plan_id"`
		GameID *int64 `json:"game_id"`
		Row    *int16 `json:"row"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		var gameID int64
		var row int16
		switch {
		case body.PlanID != nil:
			plan, err := s.Q.GetPlanByID(ctx, *body.PlanID)
			if errors.Is(err, pgx.ErrNoRows) {
				respondErr(w, http.StatusNotFound, "plan not found")
				return
			}
			if err != nil {
				respondInternalErr(w, r, "could not load plan", err)
				return
			}
			if plan.RowNumber == nil {
				respondErr(
					w,
					http.StatusConflict,
					"plan has no assigned row yet (variable-delay plans assign their row at reveal)",
				)
				return
			}
			gameID, row = plan.GameID, *plan.RowNumber
		case body.GameID != nil && body.Row != nil:
			gameID, row = *body.GameID, *body.Row
		default:
			respondErr(w, http.StatusBadRequest, "provide plan_id, or game_id and row")
			return
		}

		if row < 1 || row > 13 {
			respondErr(w, http.StatusBadRequest, "row out of range 1..13")
			return
		}

		if err := s.Q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
			ID: gameID, CurrentRow: row,
		}); err != nil {
			respondInternalErr(w, r, "could not set current row", err)
			return
		}
		broadcastRowState(ctx, s.Q, manager, gameID)

		respond(w, http.StatusOK, map[string]any{
			"game_id":     gameID,
			"current_row": row,
		})
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
