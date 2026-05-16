package handler

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

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

const (
	diceSides   = 6
	makeOutcome = "make"
	marOutcome  = "mar"
)

// ── Shared helpers ────────────────────────────────────────────────────────────

// requireRollAccess parses rollId from the URL, fetches the roll, and verifies
// the caller is a member of the roll's game. Returns roll and player.
func requireRollAccess(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
) (*dbgen.DiceRoll, *dbgen.Player, bool) {
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
	player, ok := requirePlayerInGame(w, r, q, roll.GameID)
	if !ok {
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
func GetActiveRollForGame(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		ctx := r.Context()
		rolls, err := s.Q.ListDiceRollsByGame(ctx, gameID)
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
				active = new(rolls[i])
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

		dice, err := s.Q.ListDiceByRoll(ctx, active.ID)
		if err != nil {
			dice = []dbgen.DiceRollDice{}
		}
		votes, err := s.Q.ListVotesByRoll(ctx, active.ID)
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
func CreateRoll(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		game, err := s.Q.GetGameByID(r.Context(), gameID)
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
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Difficulty < 1 || body.Difficulty > 6 {
			respondErr(w, http.StatusBadRequest, "difficulty must be between 1 and 6")
			return
		}

		ctx := r.Context()

		var roll dbgen.DiceRoll
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			r2, cErr := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
				GameID:     gameID,
				PlanID:     nil,
				RowNumber:  new(game.CurrentRow),
				ActorID:    player.ID,
				Difficulty: body.Difficulty,
			})
			if cErr != nil {
				return errors.New("could not create roll")
			}
			roll = r2

			for range 2 {
				if _, dErr := q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
					RollID:           roll.ID,
					PlayerID:         player.ID,
					IsInterference:   false,
					LeveragedAssetID: nil,
				}); dErr != nil {
					return errors.New("could not create base dice")
				}
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		broadcastEvent(manager, gameID, model.EventRollCreated, model.RollCreatedPayload{Roll: roll})

		respond(w, http.StatusCreated, map[string]any{"roll": roll})
	}
}

// ── GetRoll ───────────────────────────────────────────────────────────────────

// GetRoll handles GET /api/rolls/:rollId.
//
// Returns the roll, its dice, and the current vote counts.
func GetRoll(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, _, ok := requireRollAccess(w, r, s.Q)
		if !ok {
			return
		}

		ctx := r.Context()
		dice, err := s.Q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			dice = []dbgen.DiceRollDice{}
		}
		votes, err := s.Q.ListVotesByRoll(ctx, roll.ID)
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
func LeverageRoll(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
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
		asset, err := s.Q.GetAssetByID(ctx, body.AssetID)
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
		existingDice, err := s.Q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			respondInternalErr(w, "could not check dice", err)
			return
		}
		for _, d := range existingDice {
			if d.LeveragedAssetID != nil && *d.LeveragedAssetID == body.AssetID {
				respondErr(w, http.StatusConflict, "asset is already committed to this roll")
				return
			}
		}

		// If this roll is tied to a plan that has an active control_leverage
		// winner on a resolved demand, the target preparer may not leverage
		// their own assets on this roll — that right belongs to the demand
		// winner via /demand-leverage. Other participants leverage normally.
		if leverageBlockedByDemandWinner(ctx, s.Q, roll, player, asset) {
			respondErr(w, http.StatusForbidden,
				"a demand's control_leverage winner has taken over leverage of your assets on this roll")
			return
		}

		// Mark the asset as leveraged.
		if err = s.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID:          body.AssetID,
			IsLeveraged: true,
		}); err != nil {
			respondInternalErr(w, "could not leverage asset", err)
			return
		}

		// Determine interference: actor's own dice are not interference.
		isInterference := player.ID != roll.ActorID

		die, err := s.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID:           roll.ID,
			PlayerID:         player.ID,
			IsInterference:   isInterference,
			LeveragedAssetID: &body.AssetID,
		})
		if err != nil {
			respondInternalErr(w, "could not add die", err)
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

// leverageBlockedByDemandWinner returns true if the roll is tied to a plan
// whose target preparer (== player) is leveraging their own asset, but a
// resolved Make Demands has handed control_leverage rights to someone else.
func leverageBlockedByDemandWinner(
	ctx context.Context,
	q *dbgen.Queries,
	roll *dbgen.DiceRoll,
	player *dbgen.Player,
	asset dbgen.Asset,
) bool {
	if roll.PlanID == nil {
		return false
	}
	plan, err := q.GetPlanByID(ctx, *roll.PlanID)
	if err != nil {
		return false
	}
	if player.ID != plan.PreparerID || asset.OwnerID != plan.PreparerID {
		return false
	}
	_, winners, err := gamepkg.DemandWinnersForTargetPlan(ctx, q, &plan)
	if err != nil {
		return false
	}
	winnerID, hasWinner := winners[gamepkg.DemandOptionControlLeverage]
	return hasWinner && winnerID != 0 && winnerID != plan.PreparerID
}

// ── CallVote ──────────────────────────────────────────────────────────────────

// CallVote handles POST /api/rolls/:rollId/call-vote.
//
// Actor-only. Broadcasts roll.vote_called to all players to open the
// difficulty vote UI. No DB change — the vote state is tracked by the
// presence of rows in difficulty_votes.
func CallVote(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
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

		broadcastEvent(manager, roll.GameID, model.EventRollVoteCalled, model.RollVoteCalledPayload{RollID: roll.ID})

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
func Vote(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
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

		if err := s.Q.CreateDifficultyVote(ctx, dbgen.CreateDifficultyVoteParams{
			RollID:   roll.ID,
			PlayerID: player.ID,
			Vote:     body.Vote,
		}); err != nil {
			respondInternalErr(w, "could not record vote", err)
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
		allPlayers, err := s.Q.GetPlayersByGame(ctx, roll.GameID)
		if err != nil {
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}
		counts, err := s.Q.CountVotesByRoll(ctx, roll.ID)
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

		// All votes in — compute and store adjusted_difficulty (clamped to 1–6).
		adj := int16(min(
			max(int64(roll.Difficulty)+counts.NayCount-counts.YeaCount, 1),
			diceSides))
		if err = s.Q.SetDiceRollAdjustedDifficulty(ctx, dbgen.SetDiceRollAdjustedDifficultyParams{
			ID:                 roll.ID,
			AdjustedDifficulty: new(adj),
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
func CloseLeverage(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
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

		dice, err := s.Q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			respondInternalErr(w, "could not load dice", err)
			return
		}

		actorDice, cancelledIDs, err := rollAndCancelDice(ctx, w, s.Q, dice)
		if err != nil {
			return
		}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		err = s.Q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
			ID:      roll.ID,
			Result:  new(result),
			Outcome: new(outcome),
		})
		if err != nil {
			respondInternalErr(w, "could not resolve roll", err)
			return
		}

		// Re-fetch the resolved roll and all dice (with faces + cancelled flags).
		resolvedRoll, err := s.Q.GetDiceRollByID(ctx, roll.ID)
		if err != nil {
			respondInternalErr(w, "could not reload roll", err)
			return
		}
		finalDice, err := s.Q.ListDiceByRoll(ctx, roll.ID)
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

		broadcastEvent(manager, roll.GameID, model.EventRollResolved, model.RollResolvedPayload{
			Roll:          resolvedRoll,
			Dice:          finalDice,
			CancelledDice: cancelledDice,
		})

		respond(w, http.StatusOK, map[string]any{
			"roll":           resolvedRoll,
			"dice":           finalDice,
			"cancelled_dice": cancelledDice,
		})
	}
}

// dieEntry is a lightweight representation of a die used in roll processing.
type dieEntry struct {
	id   int64
	face int16
}

// cancelInterference groups dice by face and returns the set of cancelled
// actor die IDs. Each interference die on a given face cancels one matching
// actor die on that face (up to the count of actor dice on that face).
func cancelInterference(actorDice, interfereDice []dieEntry) map[int64]struct{} {
	// Group actor and interference dice by face value.
	actorByFace := make(map[int16][]dieEntry)
	for _, e := range actorDice {
		actorByFace[e.face] = append(actorByFace[e.face], e)
	}
	intByFace := make(map[int16][]dieEntry)
	for _, e := range interfereDice {
		intByFace[e.face] = append(intByFace[e.face], e)
	}

	// For each interference face, cancel min(actorCount, intCount) actor dice.
	cancelledIDs := make(map[int64]struct{})
	for face, intGroup := range intByFace {
		actorGroup := actorByFace[face]
		for i := range min(len(intGroup), len(actorGroup)) {
			cancelledIDs[actorGroup[i].id] = struct{}{}
		}
	}
	return cancelledIDs
}

// rollAndCancelDice rolls all dice and applies interference cancellation.
func rollAndCancelDice(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	dice []dbgen.DiceRollDice,
) ([]dieEntry, map[int64]struct{}, error) {
	var actorDice, interfereDice []dieEntry

	// Assign random faces (honouring pre-set faces, e.g. from duel accumulated
	// dice and banked dice); bucket die IDs by actor vs. interference.
	for _, d := range dice {
		var f int16
		if d.Face != nil && *d.Face >= 1 && *d.Face <= diceSides {
			f = *d.Face
		} else {
			f = int16(rand.IntN(diceSides) + 1)
			if err := q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: d.ID, Face: new(f)}); err != nil {
				respondInternalErr(w, "could not set die face", err)
				return nil, nil, err
			}
		}
		e := dieEntry{id: d.ID, face: f}
		if d.IsInterference {
			interfereDice = append(interfereDice, e)
		} else {
			actorDice = append(actorDice, e)
		}
	}

	// Apply cancellation using the pure algorithm.
	cancelledIDs := cancelInterference(actorDice, interfereDice)

	// Mark cancelled dice in the database.
	for cancelledID := range cancelledIDs {
		if err := q.SetDieCancelled(ctx, cancelledID); err != nil {
			respondInternalErr(w, "could not cancel die", err)
			return nil, nil, err
		}
	}
	return actorDice, cancelledIDs, nil
}

// ── UseBankedDie ──────────────────────────────────────────────────────────────

// UseBankedDie handles POST /api/rolls/:rollId/use-banked-die.
//
// Spends one of the actor's banked dice (from Clandestinely Liaise
// leverage_partner) on this roll. The banked die contributes to the actor's
// pool with its pre-set face value; it cannot be used as interference.
//
// Request body: {"banked_die_id": N}
func UseBankedDie(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok {
			return
		}
		if !rollIsOpen(roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}
		// Only the actor can spend banked dice (they always go on the actor's side).
		if player.ID != roll.ActorID {
			respondErr(w, http.StatusForbidden, "only the actor can spend banked dice")
			return
		}

		var body struct {
			BankedDieID int64 `json:"banked_die_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BankedDieID == 0 {
			respondErr(w, http.StatusBadRequest, "banked_die_id is required")
			return
		}

		ctx := r.Context()

		banked, err := s.Q.GetBankedDie(ctx, body.BankedDieID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "banked die not found")
			return
		}
		if banked.PlayerID != player.ID {
			respondErr(w, http.StatusForbidden, "this banked die does not belong to you")
			return
		}
		if banked.GameID != roll.GameID {
			respondErr(w, http.StatusBadRequest, "banked die does not belong to this game")
			return
		}
		if banked.UsedAt.Valid {
			respondErr(w, http.StatusConflict, "this banked die has already been spent")
			return
		}

		// Create a die entry with the pre-set face.
		die, err := s.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID:           roll.ID,
			PlayerID:         player.ID,
			IsInterference:   false,
			LeveragedAssetID: nil,
		})
		if err != nil {
			respondInternalErr(w, "could not add banked die to roll", err)
			return
		}

		// Set the face immediately (banked dice have a pre-determined face).
		if err := s.Q.SetDieFace(ctx, dbgen.SetDieFaceParams{
			ID:   die.ID,
			Face: &banked.Face,
		}); err != nil {
			respondInternalErr(w, "could not set banked die face", err)
			return
		}

		// Mark the banked die as used.
		if err := s.Q.MarkBankedDieUsed(ctx, dbgen.MarkBankedDieUsedParams{
			ID:         body.BankedDieID,
			UsedRollID: &roll.ID,
		}); err != nil {
			respondInternalErr(w, "could not mark banked die as used", err)
			return
		}

		broadcastEvent(manager, roll.GameID, model.EventRollLeverageAdded, model.RollLeverageAddedPayload{
			RollID:         roll.ID,
			PlayerID:       player.ID,
			AssetID:        0, // no asset — banked die
			IsInterference: false,
		})

		respond(w, http.StatusOK, map[string]any{
			"die":           die,
			"banked_die_id": body.BankedDieID,
			"face":          banked.Face,
		})
	}
}

// ── calculateRollResult ───────────────────────────────────────────────────────

// calculateRollResult computes the result and outcome of a resolved roll.
func calculateRollResult(actorDice []dieEntry, cancelledIDs map[int64]struct{}, roll *dbgen.DiceRoll) (int16, string) {
	distinctFaces := make(map[int16]struct{})
	for _, e := range actorDice {
		if _, exists := cancelledIDs[e.id]; !exists {
			distinctFaces[e.face] = struct{}{}
		}
	}
	result := int16(len(distinctFaces))

	effectiveDifficulty := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		effectiveDifficulty = *roll.AdjustedDifficulty
	}
	outcome := marOutcome
	if result >= effectiveDifficulty {
		outcome = makeOutcome
	}
	return result, outcome
}
