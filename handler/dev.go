package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
				Username:     username,
				PasswordHash: string(hash),
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
// Hard-deletes a single game and all of its data (cascade, migrations 039
// and 041), leaving accounts and every other game intact. Replaces the old
// DevReset, which TRUNCATEd everything — too blunt and too easy to fire by
// accident.
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
// Moves a game's current_row for manual testing and the E2E suite, in one of
// two modes.
//
// # Jump (plan_id, or game_id + row)
//
// Sets current_row directly. The generic half of "jump to a plan's resolution":
// seed a main_event game, prepare a plan through the real UI (so its bespoke
// prep state is captured faithfully), then call this to skip the intervening
// rows and land on the plan's row. The plan stays pending — resolution kickoff
// lives in the row-advance path, which a direct row-set bypasses — so you click
// "resolve" in the UI, exercising that trigger too.
//
// # Advance (game_id + advance_to)
//
// Runs the REAL row advance (advanceRowInner) repeatedly until current_row
// reaches advance_to, so everything a jump skips actually happens: the
// row.advanced broadcast and its chat post, the engrailed ranking update at
// rows 4/8/12, and the transition into Shake-Up past row 13. This is the mode
// that replaces the removed POST /tables/{id}/advance-row — see
// adr/FACILITATOR_POWERS_AUDIT.md for why that route is gone from production.
//
// Unlike the route it replaces this does NOT check the play-state gates
// (pending plans, open delay reveals, unpaid battle costs, surrender claims):
// it is a test fast-forward, not a game action, and it is dev-gated. Advancing
// a table that still owes any of those will strand that state exactly as the
// old route did — seed the board the way the test needs it instead.
//
// Body:
//
//	{ "plan_id": 123 }                 // jump that plan's game to the plan's row
//	{ "game_id": 5, "row": 9 }         // jump game 5 to row 9
//	{ "game_id": 5, "advance_to": 9 }  // really advance game 5 up to row 9
//
// Mounted only when UNEASY_DEV=1.
func DevAdvanceRow(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	type request struct {
		PlanID    *int64 `json:"plan_id"`
		GameID    *int64 `json:"game_id"`
		Row       *int16 `json:"row"`
		AdvanceTo *int16 `json:"advance_to"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		if body.AdvanceTo != nil {
			if body.GameID == nil {
				respondErr(w, http.StatusBadRequest, "advance_to requires game_id")
				return
			}
			devAdvanceRowReally(w, r, s, manager, *body.GameID, *body.AdvanceTo)
			return
		}

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
			respondErr(w, http.StatusBadRequest, "provide plan_id, game_id and row, or game_id and advance_to")
			return
		}

		if row < 1 || row > publicRecordRowCount {
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

// devAdvanceRowReally drives advanceRowInner until the game reaches target,
// re-reading the game each pass (advanceRowInner derives the engrailed-line
// crossing from the row it starts on). Stops early once the advance carries the
// game out of main_event — past row 13 that is the Shake-Up, which has no rows
// to advance. Focus is untouched, exactly as advanceRowInner leaves it.
func devAdvanceRowReally(
	w http.ResponseWriter,
	r *http.Request,
	s *db.Store,
	manager *hub.Manager,
	gameID int64,
	target int16,
) {
	ctx := r.Context()
	if target < 1 || target > publicRecordRowCount {
		respondErr(w, http.StatusBadRequest, "advance_to out of range 1..13")
		return
	}

	h, _ := manager.Get(gameID) // nil when no clients are connected — advanceRowInner handles nil

	// Bounded by the record's length: 13 advances can cross the whole sheet
	// from row 1, so this can only spin if advanceRowInner stops making
	// progress, which the phase check below already catches.
	for range publicRecordRowCount {
		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "game not found")
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}
		if game.CurrentRow >= target {
			respond(w, http.StatusOK, map[string]any{
				"game_id": gameID, "current_row": game.CurrentRow, "phase": game.Phase,
			})
			return
		}
		if _, ended, aErr := advanceRowInner(ctx, s.Q, manager, h, &game); aErr != nil {
			respondInternalErr(w, r, "could not advance row", aErr)
			return
		} else if ended {
			respond(w, http.StatusOK, map[string]any{
				"game_id": gameID, "phase": currentGamePhase(ctx, s.Q, gameID),
			})
			return
		}
	}
	respondErr(w, http.StatusInternalServerError, "row advance made no progress")
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
//	  "phase": "main_event",            // or "shake_up" / "prologue_closing"
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
//	  "shake_up_step": "spending",      // optional (shake_up only): rolling|spending
//	  "laws":   ["…"],                  // optional; omit => one sample law, [] => none
//	  "rumors": ["…"]                   // optional; omit => one sample rumor, [] => none
//	}
//
// Creates a game in the requested phase, seating the named accounts as
// players (creating accounts that don't exist). Used by the Playwright
// E2E suite to fast-forward past phases it isn't testing right now.
// Mounted only when UNEASY_DEV=1.
//
// Every seeded game gets one sample law and rumor by default, so the
// laws/rumors UI isn't blank when eyeballing a fresh game; pass an explicit
// (possibly empty) "laws"/"rumors" array to override.
//
// The fixture logic lives in package gametest, shared with the handler
// integration tests so the two suites can't disagree about what a valid
// game in phase X looks like.
// Sample laws/rumors a dev-seeded game gets by default, so the laws/rumors UI
// isn't blank when eyeballing a fresh game. Overridable per request.
const (
	devSeedSampleLaw   = "No noble may raise a private army within the capital without the crown's leave."
	devSeedSampleRumor = "They say the western holdfast was lost at cards, not conquered."
)

// devSeedPlanReq is one plan placed directly onto the seeded board.
type devSeedPlanReq struct {
	PreparerIdx int    `json:"preparer_idx"`
	PlanType    string `json:"plan_type"`
	Category    string `json:"category"`
	Row         int16  `json:"row"`
	RowOrder    int16  `json:"row_order"`
}

// devSeedRequest is the POST /api/dev/seed body.
type devSeedRequest struct {
	Phase         string           `json:"phase"`
	Players       []string         `json:"players"`
	CurrentRow    *int16           `json:"current_row"`
	Rankings      map[string][]int `json:"rankings"`
	Plans         []devSeedPlanReq `json:"plans"`
	ShakeUpTokens *int16           `json:"shake_up_tokens"`
	ShakeUpStep   *string          `json:"shake_up_step"`
	// nil => one sample entry; an explicit (possibly empty) slice overrides.
	Laws   *[]string `json:"laws"`
	Rumors *[]string `json:"rumors"`
	// nil => on for prologue_closing, off for every other phase. Set false to
	// seed blank (marginalia-less) assets at closing — the only way to exercise
	// the closing gate's blank-asset item from a seeded game — or true to give
	// a Main-Event seed notes to tear.
	StartingMarginalia *bool `json:"starting_marginalia"`
}

// devSeedOptions translates the request body into gametest Options. Returns a
// non-empty message when a field is unusable, which the caller turns into a 400.
func devSeedOptions(body *devSeedRequest) ([]gametest.Option, string) {
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
			return nil, "unknown shake_up_step: " + *body.ShakeUpStep
		}
		opts = append(opts, gametest.WithShakeUpStep(step))
	}
	// Populate the laws/rumors UI by default; an explicit array overrides
	// (pass [] for none).
	laws := []string{devSeedSampleLaw}
	if body.Laws != nil {
		laws = *body.Laws
	}
	for _, text := range laws {
		opts = append(opts, gametest.WithLaw(text))
	}
	rumors := []string{devSeedSampleRumor}
	if body.Rumors != nil {
		rumors = *body.Rumors
	}
	for _, text := range rumors {
		opts = append(opts, gametest.WithRumor(text))
	}
	// Seeded assets stay blank except at prologue_closing, where a blank asset
	// can never clear the closing gate and would make the seeded game
	// unplayable out of the box. Defaulting this on for every phase would
	// silently re-calibrate the Main-Event fixtures the e2e specs and
	// integration tests are written against (blank assets are not eligible
	// break targets, so plan sub-flows take different branches).
	startingMarginalia := body.Phase == "prologue_closing"
	if body.StartingMarginalia != nil {
		startingMarginalia = *body.StartingMarginalia
	}
	if startingMarginalia {
		opts = append(opts, gametest.WithStartingMarginalia())
	}
	return opts, ""
}

func DevSeed(s *db.Store) http.HandlerFunc {
	type playerResp struct {
		ID            int64  `json:"id"`
		AccountID     int64  `json:"account_id"`
		DisplayName   string `json:"display_name"`
		IsFacilitator bool   `json:"is_facilitator"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var body devSeedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		opts, optErr := devSeedOptions(&body)
		if optErr != "" {
			respondErr(w, http.StatusBadRequest, optErr)
			return
		}

		ctx := r.Context()
		var seeded gametest.SeededGame
		var err error
		switch body.Phase {
		case "main_event":
			seeded, err = gametest.SeedMainEvent(ctx, s.Q, body.Players, opts...)
		case "shake_up":
			seeded, err = gametest.SeedShakeUp(ctx, s.Q, body.Players, opts...)
		case "prologue_closing":
			seeded, err = gametest.SeedPrologueClosing(ctx, s.Q, body.Players, opts...)
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

// ── Forced dice (POST /api/dev/force-roll) ───────────────────────────────────

// devForceFaces picks die faces that make an open roll resolve to exactly
// `target` distinct uncancelled actor faces.
//
// It works *with* the real resolution path rather than around it:
// rollAndCancelDice keeps any die that already carries a face and only rolls
// the blank ones (the same hook Propose Duel's pre-set bout dice ride on), so
// writing every face up front makes finalizeRoll deterministic while every
// downstream effect — interference cancellation, roll.resolved, the chat post,
// applyAutoChoiceOnRoll, the row-state recompute — still runs exactly as it
// does in production.
//
// Actor die i takes face min(i+1, target), giving faces 1,2,…,target and then
// repeats of target: exactly `target` distinct values. Interference dice all
// take target+1, a face no actor die holds, so cancelInterference pairs
// nothing and the result is the actor count of distinct faces, untouched.
//
// Returns a non-empty message when the target can't be hit on this roll.
func devForceFaces(actorCount, interferenceCount int, target int16) (
	actorFaces []int16, interferenceFace int16, errMsg string,
) {
	switch {
	case actorCount == 0:
		return nil, 0, "roll has no actor dice"
	case target < 1:
		return nil, 0, "target result must be at least 1"
	case int(target) > actorCount:
		return nil, 0, fmt.Sprintf(
			"cannot reach a result of %d: the actor has only %d dice", target, actorCount)
	case target > diceSides:
		return nil, 0, fmt.Sprintf("target result must be at most %d", diceSides)
	case target == diceSides && interferenceCount > 0:
		// All six faces are in play as singletons, so every interference die
		// cancels one and the result can't be held at 6.
		return nil, 0, "cannot force a result of 6 while interference dice are committed"
	}

	actorFaces = make([]int16, actorCount)
	for i := range actorFaces {
		actorFaces[i] = min(int16(i+1), target)
	}
	return actorFaces, target + 1, ""
}

// devForceTarget derives the distinct-face count to aim for from the request's
// outcome and/or explicit result, against the roll's effective difficulty.
// Returns a non-empty message when the pair is unusable.
func devForceTarget(outcome string, result *int16, effDifficulty int16) (int16, string) {
	var target int16
	switch {
	case result != nil:
		target = *result
	case outcome == makeOutcome:
		target = effDifficulty // the minimum make
	case outcome == marOutcome:
		target = effDifficulty - 1 // the maximum mar
		if target < 1 {
			return 0, fmt.Sprintf(
				"cannot force a mar: effective difficulty is %d, so every result makes", effDifficulty)
		}
	default:
		return 0, `outcome must be "make" or "mar" (or pass an explicit result)`
	}

	// When both were given, hold them to each other — a mismatch is a typo in
	// the caller's script, not an instruction.
	if result != nil && outcome != "" {
		got := marOutcome
		if target >= effDifficulty {
			got = makeOutcome
		}
		if got != outcome {
			return 0, fmt.Sprintf(
				"result %d against effective difficulty %d is a %s, not a %s",
				target, effDifficulty, got, outcome)
		}
	}
	return target, ""
}

// devWriteForcedFaces stamps every die on the roll with a face chosen to land
// the result on `target`, so the ordinary resolution path can run unmodified.
// Returns a user-facing message when the target is unreachable on this roll.
func devWriteForcedFaces(
	ctx context.Context, q *dbgen.Queries, rollID int64, target int16,
) (string, error) {
	dice, err := q.ListDiceByRoll(ctx, rollID)
	if err != nil {
		return "", err
	}
	var actor, interference []dbgen.DiceRollDice
	for _, d := range dice {
		if d.IsInterference {
			interference = append(interference, d)
		} else {
			actor = append(actor, d)
		}
	}

	actorFaces, interferenceFace, msg := devForceFaces(len(actor), len(interference), target)
	if msg != "" {
		return msg, nil
	}
	for i, d := range actor {
		face := actorFaces[i]
		if err := q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: d.ID, Face: &face}); err != nil {
			return "", err
		}
	}
	for _, d := range interference {
		face := interferenceFace
		if err := q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: d.ID, Face: &face}); err != nil {
			return "", err
		}
	}
	return "", nil
}

// DevForceRoll handles POST /api/dev/force-roll.
//
// Forces an open dice roll to a chosen outcome, then resolves it through the
// production path. The dice are genuinely random, which leaves the *mar*
// branch of every rolled plan unreachable on demand — half of each plan's
// resolution UI. The integration harness solves this by writing the outcome
// straight to the DB (planLifecycle.forceRoll), but that skips every
// broadcast, so a browser watching the table never updates. This route instead
// pre-sets the die faces and lets finalizeRoll do the rest, so what a tester
// sees is the real thing.
//
// Body (roll_id plus one of outcome / result):
//
//	{ "roll_id": 56, "outcome": "mar" }   // result = difficulty − 1
//	{ "roll_id": 56, "outcome": "make" }  // result = difficulty
//	{ "roll_id": 56, "result": 3 }        // exact distinct-face count
//
// Passing both holds them to each other and 422s on a mismatch. The response
// is the resolved roll.
//
// Caveat: it overwrites every face on the roll, including faces another flow
// pre-set deliberately — Propose Duel's bout dice are tucked under staked
// assets before the roll, so forcing a duel's final roll discards that
// pairing. Force duel outcomes by picking the bouts instead.
//
// Mounted only when UNEASY_DEV=1.
func DevForceRoll(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	type request struct {
		RollID  *int64 `json:"roll_id"`
		Outcome string `json:"outcome"`
		Result  *int16 `json:"result"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.RollID == nil {
			respondErr(w, http.StatusBadRequest, "roll_id required")
			return
		}

		ctx := r.Context()
		roll, err := s.Q.GetDiceRollByID(ctx, *body.RollID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "roll not found")
			return
		}
		if !rollIsOpen(&roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}
		if roll.IsShakeUp {
			respondErr(w, http.StatusConflict,
				"shake-up rolls grant tokens rather than make/mar; force them with an explicit result")
			return
		}

		effDifficulty := roll.Difficulty
		if roll.AdjustedDifficulty != nil {
			effDifficulty = *roll.AdjustedDifficulty
		}
		target, msg := devForceTarget(body.Outcome, body.Result, effDifficulty)
		if msg != "" {
			respondErr(w, http.StatusUnprocessableEntity, msg)
			return
		}

		if msg, err := devWriteForcedFaces(ctx, s.Q, roll.ID, target); err != nil {
			respondInternalErr(w, r, "could not force die faces", err)
			return
		} else if msg != "" {
			respondErr(w, http.StatusUnprocessableEntity, msg)
			return
		}

		// Everyone ready + the real stage machine: advanceToLeverage ends in
		// maybeAutoResolve, which now finds every participant ready and calls
		// finalizeRoll. A roll already in the leverage stage skips straight there.
		if err := s.Q.SetAllParticipantsReady(ctx, roll.ID); err != nil {
			respondInternalErr(w, r, "could not ready participants", err)
			return
		}
		if roll.Stage == stageLeverage {
			err = maybeAutoResolve(ctx, w, r, s.Q, manager, &roll)
		} else {
			err = advanceToLeverage(ctx, w, r, s.Q, manager, &roll)
		}
		if err != nil {
			respondInternalErr(w, r, "could not resolve roll", err)
			return
		}

		resolved, err := s.Q.GetDiceRollByID(ctx, roll.ID)
		if err != nil {
			respondInternalErr(w, r, "could not reload roll", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"roll": resolved})
	}
}
