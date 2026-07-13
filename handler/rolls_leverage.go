package handler

// handler/rolls_leverage.go — Roll voting, intent, leverage, and banked-dice
// HTTP endpoints (CallVote/SkipVote/Vote, SetIntent/SetReady, LeverageRoll,
// ListBankedDice/UseBankedDie, CloseLeverage). See rolls.go for the roll
// object endpoints (Create/Get) and the stage machine's full lifecycle doc.

import (
	"context"
	"encoding/json"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── CallVote / SkipVote / Vote ───────────────────────────────────────────────

// CallVote handles POST /api/rolls/:rollId/call-vote. Actor-only;
// decide_vote → voting.
func CallVote(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok {
			return
		}
		if roll.Stage != stageDecideVote {
			respondErr(w, http.StatusConflict, "vote can only be called from decide_vote stage")
			return
		}
		if player.ID != roll.ActorID {
			respondErr(w, http.StatusForbidden, "only the actor can call a difficulty vote")
			return
		}
		ctx := r.Context()
		if err := s.Q.SetDiceRollStage(ctx, dbgen.SetDiceRollStageParams{
			ID: roll.ID, Stage: stageVoting,
		}); err != nil {
			respondInternalErr(w, r, "could not set stage", err)
			return
		}
		broadcastEvent(manager, roll.GameID, model.EventRollStageChanged, model.RollStageChangedPayload{
			RollID: roll.ID, Stage: stageVoting,
		})
		// The acting set changes from [actor] to "players minus voters".
		broadcastRowState(ctx, s.Q, manager, roll.GameID)
		respond(w, http.StatusOK, map[string]any{"roll_id": roll.ID})
	}
}

// SkipVote handles POST /api/rolls/:rollId/skip-vote. Actor-only;
// decide_vote → leverage. Runs the skip-leverage short-circuit and
// auto-resolution via advanceToLeverage.
func SkipVote(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok {
			return
		}
		if roll.Stage != stageDecideVote {
			respondErr(w, http.StatusConflict, "vote can only be skipped from decide_vote stage")
			return
		}
		if player.ID != roll.ActorID {
			respondErr(w, http.StatusForbidden, "only the actor can skip the difficulty vote")
			return
		}
		if err := advanceToLeverage(r.Context(), w, r, s.Q, manager, roll); err != nil {
			respondInternalErr(w, r, "could not advance to leverage", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"roll_id": roll.ID})
	}
}

// Vote handles POST /api/rolls/:rollId/vote.
//
// Request body: {"vote": 1} or {"vote": -1}.
//
// Hidden ballot: other players see only that the voter has voted (not the
// value). When the last vote lands, the server computes adjusted_difficulty,
// broadcasts the full ballot, and advances to leverage.
func Vote(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok {
			return
		}
		if roll.Stage != stageVoting {
			respondErr(w, http.StatusConflict, "votes only accepted in voting stage")
			return
		}

		var body struct {
			Vote int16 `json:"vote"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Vote != 1 && body.Vote != -1 {
			respondErr(w, http.StatusBadRequest, "vote must be 1 or -1")
			return
		}

		ctx := r.Context()
		if err := s.Q.CreateDifficultyVote(ctx, dbgen.CreateDifficultyVoteParams{
			RollID: roll.ID, PlayerID: player.ID, Vote: body.Vote,
		}); err != nil {
			respondInternalErr(w, r, "could not record vote", err)
			return
		}

		// Hidden-ballot broadcast: no vote value.
		broadcastEvent(manager, roll.GameID, model.EventRollVoteCast, model.RollVoteCastPayload{
			RollID: roll.ID, PlayerID: player.ID,
		})

		allPlayers, err := s.Q.GetPlayersByGame(ctx, roll.GameID)
		if err != nil {
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}
		summary, err := s.Q.SumVotesByRoll(ctx, roll.ID)
		if err != nil {
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}
		if summary.VoteCount < int64(len(allPlayers)) {
			// This voter drops out of the "players minus voters" acting set.
			broadcastRowState(ctx, s.Q, manager, roll.GameID)
			respond(w, http.StatusOK, map[string]any{"vote": body.Vote})
			return
		}

		// All votes in — compute, reveal, advance.
		adj := int16(min(max(int64(roll.Difficulty)+summary.VoteSum, 1), diceSides))
		if err = s.Q.SetDiceRollAdjustedDifficulty(ctx, dbgen.SetDiceRollAdjustedDifficultyParams{
			ID: roll.ID, AdjustedDifficulty: &adj,
		}); err != nil {
			respondInternalErr(w, r, "could not set adjusted difficulty", err)
			return
		}
		roll.AdjustedDifficulty = &adj

		allVotes, _ := s.Q.ListVotesByRoll(ctx, roll.ID)
		ballot := make([]voteView, 0, len(allVotes))
		for _, v := range allVotes {
			vv := v.Vote
			ballot = append(ballot, voteView{
				RollID: v.RollID, PlayerID: v.PlayerID, Voted: true, Vote: &vv,
			})
		}
		broadcastEvent(manager, roll.GameID, model.EventRollVoteResolved, model.RollVoteResolvedPayload{
			RollID:             roll.ID,
			AdjustedDifficulty: adj,
			Ballot:             ballot,
		})
		EmitDifficultyVoteResolved(ctx, s.Q, manager, roll, allVotes, adj)

		if err := advanceToLeverage(ctx, w, r, s.Q, manager, roll); err != nil {
			respondInternalErr(w, r, "could not advance to leverage", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{
			"vote":                body.Vote,
			"adjusted_difficulty": adj,
		})
	}
}

// ── SetIntent ────────────────────────────────────────────────────────────────

// SetIntent handles POST /api/rolls/:rollId/intent.
//
// Body: {"intent": "aid"|"interfere"}. Non-actor only; leverage stage only;
// rejected once the player has committed any die on this roll.
func SetIntent(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok || !requireLeverageStage(w, roll) {
			return
		}
		if player.ID == roll.ActorID {
			respondErr(w, http.StatusForbidden, "the actor's intent is always aid")
			return
		}

		var body struct {
			Intent string `json:"intent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Intent != intentAid && body.Intent != intentInterfere {
			respondErr(w, http.StatusBadRequest, "intent must be 'aid' or 'interfere'")
			return
		}

		ctx := r.Context()
		// Lock: any committed die freezes intent.
		dice, err := s.Q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			respondInternalErr(w, r, "could not load dice", err)
			return
		}
		for _, d := range dice {
			if d.PlayerID == player.ID {
				respondErr(w, http.StatusConflict, "intent is locked once a die is committed")
				return
			}
		}

		if err := s.Q.SetParticipantIntent(ctx, dbgen.SetParticipantIntentParams{
			RollID: roll.ID, PlayerID: player.ID, Intent: &body.Intent,
		}); err != nil {
			respondInternalErr(w, r, "could not set intent", err)
			return
		}
		broadcastEvent(manager, roll.GameID, model.EventRollIntentSet, model.RollIntentSetPayload{
			RollID: roll.ID, PlayerID: player.ID, Intent: body.Intent,
		})
		respond(w, http.StatusOK, map[string]any{"intent": body.Intent})
	}
}

// ── SetReady ─────────────────────────────────────────────────────────────────

// SetReady handles POST /api/rolls/:rollId/ready.
//
// Body: {"is_ready": true|false}. Leverage stage only. Players cannot
// unready when they have no dice left to commit. Setting ready=true as the
// last unready participant triggers auto-resolution.
func SetReady(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok || !requireLeverageStage(w, roll) {
			return
		}

		var body struct {
			IsReady bool `json:"is_ready"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		if !body.IsReady {
			can, err := playerCanCommit(ctx, s.Q, roll.GameID, player.ID)
			if err != nil {
				respondInternalErr(w, r, "could not check commit ability", err)
				return
			}
			if !can {
				respondErr(w, http.StatusConflict, "you have no dice left to commit")
				return
			}
		}

		if err := s.Q.SetParticipantReady(ctx, dbgen.SetParticipantReadyParams{
			RollID: roll.ID, PlayerID: player.ID, IsReady: body.IsReady,
		}); err != nil {
			respondInternalErr(w, r, "could not set ready", err)
			return
		}
		broadcastEvent(manager, roll.GameID, model.EventRollReadyChanged, model.RollReadyChangedPayload{
			RollID: roll.ID, PlayerID: player.ID, IsReady: body.IsReady,
		})
		// This player joins or leaves the leverage-stage unready acting set.
		// Redundant with finalizeRoll's own broadcast if this ready flip also
		// triggers auto-resolution below — harmless, the client no-ops on a
		// repeated row_state.changed with the same content.
		broadcastRowState(ctx, s.Q, manager, roll.GameID)

		if body.IsReady {
			if err := maybeAutoResolve(ctx, w, r, s.Q, manager, roll); err != nil {
				respondInternalErr(w, r, "could not auto-resolve", err)
				return
			}
		}
		respond(w, http.StatusOK, map[string]any{"is_ready": body.IsReady})
	}
}

// ── LeverageRoll ─────────────────────────────────────────────────────────────

// LeverageRoll handles POST /api/rolls/:rollId/leverage.
//
// Body: {"asset_id": N}.
//
// Leverage stage only; caller must not be currently ready; non-actors must
// have set an intent. Validates the asset, marks it leveraged, then delegates
// to commitDie for the shared create/broadcast/log/sweep flow.
func LeverageRoll(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok || !requireLeverageStage(w, roll) {
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
		isInterference, ok := commitGate(w, r, s.Q, roll, player)
		if !ok {
			return
		}
		asset, ok := validateLeverageAsset(ctx, w, r, s.Q, roll, player, body.AssetID)
		if !ok {
			return
		}

		if err := s.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID: body.AssetID, IsLeveraged: true,
		}); err != nil {
			respondInternalErr(w, r, "could not leverage asset", err)
			return
		}
		broadcastEvent(manager, roll.GameID, model.EventAssetLeveraged, model.AssetIDPayload{
			AssetID: body.AssetID, PlayerID: player.ID,
		})

		die, ok := commitDie(ctx, w, r, s.Q, manager, roll, player, isInterference, &body.AssetID, &asset.Name)
		if !ok {
			return
		}
		respond(w, http.StatusOK, map[string]any{"die": die})
	}
}

// validateLeverageAsset loads and validates an asset for leverage: owned by
// the caller, not destroyed, not already committed to this roll, not blocked
// by a Make Demands control_leverage winner.
func validateLeverageAsset(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	roll *dbgen.DiceRoll,
	player *dbgen.Player,
	assetID int64,
) (dbgen.Asset, bool) {
	asset, err := q.GetAssetByID(ctx, assetID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "asset not found")
		return dbgen.Asset{}, false
	}
	if asset.OwnerID != player.ID {
		respondErr(w, http.StatusForbidden, "you can only leverage your own assets")
		return dbgen.Asset{}, false
	}
	if asset.IsDestroyed {
		respondErr(w, http.StatusConflict, "asset is destroyed")
		return dbgen.Asset{}, false
	}
	if asset.IsLeveraged {
		// Mirrors LeverageAsset's own check (handler/assets.go): an asset already
		// leveraged — whether from an earlier roll this session or the standalone
		// /assets/{id}/leverage action — can't be committed again until refreshed.
		// This is what makes Shake-Up ruling 4 ("leverage is real and persistent
		// across categories — nothing refreshes assets between them") actually
		// hold: without it, an asset spent on the esteem roll could be leveraged
		// again for knowledge.
		respondErr(w, http.StatusConflict, "asset is already leveraged")
		return dbgen.Asset{}, false
	}
	existingDice, err := q.ListDiceByRoll(ctx, roll.ID)
	if err != nil {
		respondInternalErr(w, r, "could not check dice", err)
		return dbgen.Asset{}, false
	}
	for _, d := range existingDice {
		if d.LeveragedAssetID != nil && *d.LeveragedAssetID == assetID {
			respondErr(w, http.StatusConflict, "asset is already committed to this roll")
			return dbgen.Asset{}, false
		}
	}
	if leverageBlockedByDemandWinner(ctx, q, roll, player, asset) {
		respondErr(w, http.StatusForbidden,
			"a demand's control_leverage winner has taken over leverage of your assets on this roll")
		return dbgen.Asset{}, false
	}
	return asset, true
}

// commitGate enforces the shared preconditions for leveraging an asset or
// spending a banked die: leverage stage (assumed already checked by the
// caller), not currently ready, intent set for non-actors. Returns
// (isInterference, ok).
func commitGate(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	roll *dbgen.DiceRoll,
	player *dbgen.Player,
) (bool, bool) {
	part, err := q.GetParticipant(r.Context(), dbgen.GetParticipantParams{
		RollID: roll.ID, PlayerID: player.ID,
	})
	if err != nil {
		respondErr(w, http.StatusForbidden, "not a participant in this roll")
		return false, false
	}
	if part.IsReady {
		respondErr(w, http.StatusConflict, "unready yourself before committing more dice")
		return false, false
	}
	if player.ID == roll.ActorID {
		return false, true
	}
	if part.Intent == nil {
		respondErr(w, http.StatusConflict, "set intent (aid or interfere) before committing")
		return false, false
	}
	return *part.Intent == intentInterfere, true
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
	_, winners, err := DemandWinnersForTargetPlan(ctx, q, &plan)
	if err != nil {
		return false
	}
	winnerID, hasWinner := winners[gamepkg.DemandOptionControlLeverage]
	return hasWinner && winnerID != 0 && winnerID != plan.PreparerID
}

// ── ListBankedDice / UseBankedDie ────────────────────────────────────────────

// ListBankedDice handles GET /api/tables/:id/banked-dice. Returns the
// calling player's unspent banked dice in this game.
func ListBankedDice(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		dice, err := s.Q.ListBankedDiceByPlayer(r.Context(), dbgen.ListBankedDiceByPlayerParams{
			GameID: gameID, PlayerID: player.ID,
		})
		if err != nil {
			respond(w, http.StatusOK, map[string]any{"dice": []any{}})
			return
		}
		respond(w, http.StatusOK, map[string]any{"dice": dice})
	}
}

// UseBankedDie handles POST /api/rolls/:rollId/use-banked-die.
//
// Spends one of the calling player's banked dice on this roll. Owner-only
// (no actor restriction). The die rolls a random face at resolution like
// any other die — banked dice no longer carry a pre-determined face.
// Same gating as LeverageRoll: leverage stage, not ready, intent set for
// non-actors; runs the same post-commit sweeps and chat log.
//
// Request body: {"banked_die_id": N}.
func UseBankedDie(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok || !requireLeverageStage(w, roll) {
			return
		}
		if roll.IsShakeUp {
			respondErr(w, http.StatusConflict, "banked dice cannot be spent during the Shake-Up")
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
		isInterference, ok := commitGate(w, r, s.Q, roll, player)
		if !ok {
			return
		}
		if !validateBankedDie(ctx, w, s.Q, roll, player, body.BankedDieID) {
			return
		}

		if err := s.Q.MarkBankedDieUsed(ctx, dbgen.MarkBankedDieUsedParams{
			ID: body.BankedDieID, UsedRollID: &roll.ID,
		}); err != nil {
			respondInternalErr(w, r, "could not mark banked die as used", err)
			return
		}

		die, ok := commitDie(ctx, w, r, s.Q, manager, roll, player, isInterference, nil, nil)
		if !ok {
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"die":           die,
			"banked_die_id": body.BankedDieID,
		})
	}
}

// validateBankedDie loads and validates a banked die for spending: it exists,
// belongs to the caller, is in this game, and hasn't already been spent.
func validateBankedDie(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	roll *dbgen.DiceRoll,
	player *dbgen.Player,
	bankedDieID int64,
) bool {
	banked, err := q.GetBankedDie(ctx, bankedDieID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "banked die not found")
		return false
	}
	if banked.PlayerID != player.ID {
		respondErr(w, http.StatusForbidden, "this banked die does not belong to you")
		return false
	}
	if banked.GameID != roll.GameID {
		respondErr(w, http.StatusBadRequest, "banked die does not belong to this game")
		return false
	}
	if banked.UsedAt.Valid {
		respondErr(w, http.StatusConflict, "this banked die has already been spent")
		return false
	}
	return true
}

// ── CloseLeverage (legacy, unsurfaced) ───────────────────────────────────────

// CloseLeverage handles POST /api/rolls/:rollId/close-leverage. Actor or
// facilitator only; remains on the backend as a future-proof hook (e.g. a
// table-wide decision timer). Not surfaced in the frontend; auto-resolution
// is the default path.
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
		if player.ID != roll.ActorID && !player.IsFacilitator {
			respondErr(w, http.StatusForbidden, "only the actor or facilitator can close leverage")
			return
		}
		if err := finalizeRoll(r.Context(), w, r, s.Q, manager, roll); err != nil {
			respondInternalErr(w, r, "could not finalize roll", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"roll_id": roll.ID})
	}
}
