// handler/rolls.go — Dice roll endpoints (Phase 2e).
//
// Roll lifecycle:
//
//  1. Actor calls POST /api/tables/:id/rolls to create a roll.
//     Two base dice are created for the actor (no asset, not interference).
//  2. Any player calls POST /api/rolls/:rollId/leverage to commit a leveraged
//     asset, adding one die. Actor dice are is_interference=false; all others
//     are is_interference=true.
//  3. (Optional) Actor calls POST /api/rolls/:rollId/call-vote to broadcast
//     a difficulty vote to all players.
//  4. Players call POST /api/rolls/:rollId/vote with yea or nay. When all
//     players have voted, the server computes adjusted_difficulty.
//  5. Actor (or facilitator) calls POST /api/rolls/:rollId/close-leverage to
//     roll all dice and resolve the roll.
//  6. GET /api/rolls/:rollId returns the current roll state.
package handler

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// ── Shared helpers ────────────────────────────────────────────────────────────

// requireRollAccess parses rollId from the URL, fetches the roll, and verifies
// the caller is a member of the roll's game. Returns roll and player.
func requireRollAccess(w http.ResponseWriter, r *http.Request, q *dbgen.Queries) (*dbgen.DiceRoll, *dbgen.Player, bool) {
	rollID, err := strconv.ParseInt(chi.URLParam(r, "rollId"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid roll id")
		return nil, nil, false
	}
	roll, err := q.GetDiceRollByID(r.Context(), rollID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "roll not found")
		return nil, nil, false
	}
	player := appMiddleware.PlayerFromContext(r.Context())
	if player == nil || player.GameID != roll.GameID {
		respondErr(w, http.StatusForbidden, "not a member of this table")
		return nil, nil, false
	}
	return &roll, player, true
}

// rollIsOpen returns true when the roll has not yet been resolved (no result).
func rollIsOpen(roll *dbgen.DiceRoll) bool {
	return roll.Result == nil
}

// ── GetActiveRollForGame ──────────────────────────────────────────────────────

// GetActiveRollForGame handles GET /api/tables/:id/rolls/active.
//
// Returns the most recently created unresolved roll for the game, plus its
// dice and votes. If no roll is active, returns {"roll": null, "dice": [], "votes": []}.
func GetActiveRollForGame(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}

		ctx := r.Context()
		rolls, err := q.ListDiceRollsByGame(ctx, gameID)
		if err != nil {
			respond(w, http.StatusOK, map[string]any{
				"roll":  nil,
				"dice":  []any{},
				"votes": []any{},
			})
			return
		}

		// Find the most recent open roll.
		var active *dbgen.DiceRoll
		for i := len(rolls) - 1; i >= 0; i-- {
			if rolls[i].Result == nil {
				r := rolls[i]
				active = &r
				break
			}
		}
		if active == nil {
			respond(w, http.StatusOK, map[string]any{
				"roll":  nil,
				"dice":  []dbgen.DiceRollDice{},
				"votes": []dbgen.DifficultyVote{},
			})
			return
		}

		dice, err := q.ListDiceByRoll(ctx, active.ID)
		if err != nil {
			dice = []dbgen.DiceRollDice{}
		}
		votes, err := q.ListVotesByRoll(ctx, active.ID)
		if err != nil {
			votes = []dbgen.DifficultyVote{}
		}

		respond(w, http.StatusOK, map[string]any{
			"roll":  active,
			"dice":  dice,
			"votes": votes,
		})
	}
}

// ── CreateRoll ────────────────────────────────────────────────────────────────

// CreateRoll handles POST /api/tables/:id/rolls.
//
// Request body:
//
//	{"difficulty": 1..6}
//
// Creates a dice roll for the current row. The caller becomes the actor and
// receives 2 base dice (no leveraged asset). The roll is broadcast via
// roll.created so all clients can display the panel.
func CreateRoll(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}
		game, err := q.GetGameByID(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "dice rolls require the main event phase")
			return
		}

		var body struct {
			Difficulty int16 `json:"difficulty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Difficulty < 1 || body.Difficulty > 6 {
			respondErr(w, http.StatusBadRequest, "difficulty must be between 1 and 6")
			return
		}

		ctx := r.Context()
		row := game.CurrentRow

		roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
			GameID:     gameID,
			PlanID:     nil,
			RowNumber:  &row,
			ActorID:    player.ID,
			Difficulty: body.Difficulty,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create roll")
			return
		}

		// Create 2 base dice for the actor (no leveraged asset).
		for range 2 {
			if _, err := q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
				RollID:           roll.ID,
				PlayerID:         player.ID,
				IsInterference:   false,
				LeveragedAssetID: nil,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not create base dice")
				return
			}
		}

		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventRollCreated, model.RollCreatedPayload{Roll: roll})
		}

		respond(w, http.StatusCreated, map[string]any{"roll": roll})
	}
}

// ── GetRoll ───────────────────────────────────────────────────────────────────

// GetRoll handles GET /api/rolls/:rollId.
//
// Returns the roll, its dice, and the current vote counts.
func GetRoll(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, _, ok := requireRollAccess(w, r, q)
		if !ok {
			return
		}

		ctx := r.Context()
		dice, err := q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			dice = []dbgen.DiceRollDice{}
		}
		votes, err := q.ListVotesByRoll(ctx, roll.ID)
		if err != nil {
			votes = []dbgen.DifficultyVote{}
		}

		respond(w, http.StatusOK, map[string]any{
			"roll":  roll,
			"dice":  dice,
			"votes": votes,
		})
	}
}

// ── LeverageRoll ──────────────────────────────────────────────────────────────

// LeverageRoll handles POST /api/rolls/:rollId/leverage.
//
// Request body:
//
//	{"asset_id": 123, "is_interference": true}
//
// Commits a player's asset to the roll, adding one die. The caller must own
// the asset. The die is marked as interference when the caller is not the
// actor. The asset must not already be committed to this roll.
func LeverageRoll(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, q)
		if !ok {
			return
		}
		if !rollIsOpen(roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}

		var body struct {
			AssetID int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		// Validate the asset.
		asset, err := q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.OwnerID != player.ID {
			respondErr(w, http.StatusForbidden, "you can only leverage your own assets")
			return
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "asset is destroyed")
			return
		}

		// Check the asset hasn't already been committed to this roll.
		existingDice, err := q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check dice")
			return
		}
		for _, d := range existingDice {
			if d.LeveragedAssetID != nil && *d.LeveragedAssetID == body.AssetID {
				respondErr(w, http.StatusConflict, "asset is already committed to this roll")
				return
			}
		}

		// Mark the asset as leveraged.
		if err := q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID:          body.AssetID,
			IsLeveraged: true,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not leverage asset")
			return
		}

		// Determine interference: actor's own dice are not interference.
		isInterference := player.ID != roll.ActorID

		die, err := q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID:           roll.ID,
			PlayerID:         player.ID,
			IsInterference:   isInterference,
			LeveragedAssetID: &body.AssetID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not add die")
			return
		}

		if h, ok := manager.Get(roll.GameID); ok {
			h.BroadcastEvent(model.EventAssetLeveraged, model.AssetIDPayload{
				AssetID:  body.AssetID,
				PlayerID: player.ID,
			})
			h.BroadcastEvent(model.EventRollLeverageAdded, model.RollLeverageAddedPayload{
				RollID:         roll.ID,
				PlayerID:       player.ID,
				AssetID:        body.AssetID,
				IsInterference: isInterference,
			})
		}

		respond(w, http.StatusOK, map[string]any{"die": die})
	}
}

// ── CallVote ──────────────────────────────────────────────────────────────────

// CallVote handles POST /api/rolls/:rollId/call-vote.
//
// Actor-only. Broadcasts roll.vote_called to all players to open the
// difficulty vote UI. No DB change — the vote state is tracked by the
// presence of rows in difficulty_votes.
func CallVote(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, q)
		if !ok {
			return
		}
		if !rollIsOpen(roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}
		if player.ID != roll.ActorID {
			respondErr(w, http.StatusForbidden, "only the actor can call a difficulty vote")
			return
		}

		if h, ok := manager.Get(roll.GameID); ok {
			h.BroadcastEvent(model.EventRollVoteCalled, model.RollVoteCalledPayload{RollID: roll.ID})
		}

		respond(w, http.StatusOK, map[string]any{"roll_id": roll.ID})
	}
}

// ── Vote ──────────────────────────────────────────────────────────────────────

// Vote handles POST /api/rolls/:rollId/vote.
//
// Request body:
//
//	{"vote": "yea"} or {"vote": "nay"}
//
// Submits a difficulty vote. When all players in the game have voted, the
// server computes adjusted_difficulty = difficulty + nay_count - yea_count
// (clamped to 1..6) and broadcasts roll.vote_resolved.
func Vote(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, q)
		if !ok {
			return
		}
		if !rollIsOpen(roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}

		var body struct {
			Vote string `json:"vote"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Vote != "yea" && body.Vote != "nay" {
			respondErr(w, http.StatusBadRequest, "vote must be 'yea' or 'nay'")
			return
		}

		ctx := r.Context()

		if err := q.CreateDifficultyVote(ctx, dbgen.CreateDifficultyVoteParams{
			RollID:   roll.ID,
			PlayerID: player.ID,
			Vote:     body.Vote,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record vote")
			return
		}

		h, hasHub := manager.Get(roll.GameID)
		if hasHub {
			h.BroadcastEvent(model.EventRollVoteCast, model.RollVoteCastPayload{
				RollID:   roll.ID,
				PlayerID: player.ID,
				Vote:     body.Vote,
			})
		}

		// Check if all players have voted.
		allPlayers, err := q.GetPlayersByGame(ctx, roll.GameID)
		if err != nil {
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}
		counts, err := q.CountVotesByRoll(ctx, roll.ID)
		if err != nil {
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}
		totalVotes := counts.YeaCount + counts.NayCount
		if totalVotes < int64(len(allPlayers)) {
			// Not everyone has voted yet.
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}

		// All votes in — compute and store adjusted_difficulty.
		adj := max(roll.Difficulty+int16(counts.NayCount)-int16(counts.YeaCount), 1)
		if adj > 6 {
			adj = 6
		}
		if err := q.SetDiceRollAdjustedDifficulty(ctx, dbgen.SetDiceRollAdjustedDifficultyParams{
			ID:                 roll.ID,
			AdjustedDifficulty: &adj,
		}); err != nil {
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}

		if hasHub {
			h.BroadcastEvent(model.EventRollVoteResolved, model.RollVoteResolvedPayload{
				RollID:             roll.ID,
				AdjustedDifficulty: adj,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"vote":                body.Vote,
			"adjusted_difficulty": adj,
		})
	}
}

// ── CloseLeverage ─────────────────────────────────────────────────────────────

// CloseLeverage handles POST /api/rolls/:rollId/close-leverage.
//
// Actor or facilitator only. Closes the leverage window, assigns random faces
// (1–6) to every die, applies interference cancellation, computes the result,
// and broadcasts roll.resolved.
//
// Interference cancellation algorithm:
//
//	For each distinct face value:
//	  cancel min(actorCount, interferenceCount) actor dice showing that face.
//
// Result = count of distinct face values in the actor's uncancelled dice.
// Outcome = "make" if result >= effective_difficulty, else "mar".
// Effective difficulty = adjusted_difficulty if set, otherwise difficulty.
func CloseLeverage(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, q)
		if !ok {
			return
		}
		if !rollIsOpen(roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}

		// Only the actor or the facilitator may close the leverage window.
		isActor := player.ID == roll.ActorID
		isFac := player.IsFacilitator
		if !isActor && !isFac {
			respondErr(w, http.StatusForbidden, "only the actor or facilitator can close the leverage window")
			return
		}

		ctx := r.Context()

		dice, err := q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load dice")
			return
		}

		// ── Roll all dice ────────────────────────────────────────────────────────

		// Assign random faces and bucket into actor vs. interference slices.
		type rolledDie struct {
			die  dbgen.DiceRollDice
			face int16
		}
		var actorDice, intDice []rolledDie

		for _, d := range dice {
			f := int16(rand.IntN(6) + 1) //nolint:gosec // game randomness, not security
			face := f
			if err := q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: d.ID, Face: &face}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set die face")
				return
			}
			d.Face = &f
			rd := rolledDie{die: d, face: f}
			if d.IsInterference {
				intDice = append(intDice, rd)
			} else {
				actorDice = append(actorDice, rd)
			}
		}

		// ── Interference cancellation ────────────────────────────────────────────

		// Group actor dice by face.
		actorByFace := map[int16][]rolledDie{}
		for _, rd := range actorDice {
			actorByFace[rd.face] = append(actorByFace[rd.face], rd)
		}
		// Group interference dice by face.
		intByFace := map[int16][]rolledDie{}
		for _, rd := range intDice {
			intByFace[rd.face] = append(intByFace[rd.face], rd)
		}

		// For each interference face, cancel min(actorCount, intCount) actor dice.
		cancelledIDs := map[int64]bool{}
		for face, intGroup := range intByFace {
			actorGroup := actorByFace[face]
			cancelCount := min(len(intGroup), len(actorGroup))
			for i := range cancelCount {
				id := actorGroup[i].die.ID
				if err := q.SetDieCancelled(ctx, id); err != nil {
					respondErr(w, http.StatusInternalServerError, "could not cancel die")
					return
				}
				cancelledIDs[id] = true
			}
		}

		// ── Result calculation ───────────────────────────────────────────────────

		distinctFaces := map[int16]bool{}
		for _, rd := range actorDice {
			if !cancelledIDs[rd.die.ID] {
				distinctFaces[rd.face] = true
			}
		}
		result := int16(len(distinctFaces))

		effectiveDifficulty := roll.Difficulty
		if roll.AdjustedDifficulty != nil {
			effectiveDifficulty = *roll.AdjustedDifficulty
		}
		outcome := "mar"
		if result >= effectiveDifficulty {
			outcome = "make"
		}

		if err := q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
			ID:      roll.ID,
			Result:  &result,
			Outcome: &outcome,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not resolve roll")
			return
		}

		// Re-fetch the resolved roll and all dice (with faces + cancelled flags).
		resolvedRoll, err := q.GetDiceRollByID(ctx, roll.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not reload roll")
			return
		}
		finalDice, err := q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			finalDice = []dbgen.DiceRollDice{}
		}

		// Build cancelled slice for the payload.
		cancelledDice := []dbgen.DiceRollDice{}
		for _, d := range finalDice {
			if d.IsCancelled {
				cancelledDice = append(cancelledDice, d)
			}
		}

		if h, ok := manager.Get(roll.GameID); ok {
			h.BroadcastEvent(model.EventRollResolved, model.RollResolvedPayload{
				Roll:          resolvedRoll,
				Dice:          finalDice,
				CancelledDice: cancelledDice,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"roll":           resolvedRoll,
			"dice":           finalDice,
			"cancelled_dice": cancelledDice,
		})
	}
}
