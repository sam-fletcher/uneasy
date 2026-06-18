package handler

// handler/rolls.go — HTTP endpoints for the dice-roll stage machine.
//
// Server-driven flow:
//
//  1. POST /api/tables/{id}/rolls (CreateRoll): caller passes
//     {actor_id, difficulty, scene_id?, plan_id?}. Server validates the
//     actor against the optional context, creates the roll at
//     stage='decide_vote', seeds one participant row per game player
//     (actor with intent='aid', others with intent=NULL), and adds the
//     actor's 2 base dice.
//
//  2. Actor calls /call-vote (→ stage='voting') or /skip-vote
//     (→ stage='leverage'). Skip path runs the leverage-entry
//     short-circuit immediately.
//
//  3. During voting: each player casts +1 or -1 via /vote. Other players
//     see only "voted: true" (server redacts). When the last vote lands,
//     the server computes adjusted_difficulty, broadcasts the full
//     ballot, and advances to leverage (running the short-circuit).
//
//  4. During leverage: non-actors pick an intent (aid/interfere) via
//     /intent. Any player commits dice via /leverage (asset) or
//     /use-banked-die. Each commit auto-unreadies opponents who can
//     still commit, auto-readies the committer if they have nothing
//     left, and writes a Minor chat log entry. Players toggle /ready;
//     when the last unready participant readies, the server rolls and
//     resolves automatically.
//
// Stage machine internals (sweeps, seed, advance) live in rolls_stage.go.
// Pure dice math + resolution (cancellation, faces, finalize) live in
// rolls_dice.go. Chat-log entry emitters (EmitRollCommit,
// EmitRollSkipLeverage) live in system_posts.go alongside the other
// system-post helpers.
//
// CloseLeverage remains on the backend (unsurfaced in the frontend) as a
// future-proofing hook for table-wide decision timers.

import (
	"context"
	"encoding/json"
	"errors"
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

	stageDecideVote = "decide_vote"
	stageVoting     = "voting"
	stageLeverage   = "leverage"
	stageResolved   = "resolved"

	intentAid       = "aid"
	intentInterfere = "interfere"
)

// ── Shared request helpers ───────────────────────────────────────────────────

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

// rollIsOpen returns true when the roll has not yet been resolved.
func rollIsOpen(roll *dbgen.DiceRoll) bool {
	return roll.Result == nil
}

// requireLeverageStage writes a 409 and returns false if the roll isn't in
// the leverage stage.
func requireLeverageStage(w http.ResponseWriter, roll *dbgen.DiceRoll) bool {
	if roll.Stage != stageLeverage {
		respondErr(w, http.StatusConflict, "action only allowed in leverage stage")
		return false
	}
	return true
}

// ── Vote redaction ───────────────────────────────────────────────────────────

// voteView is the redacted/full vote shape returned to a viewer. The Vote
// pointer is nil when redacted.
type voteView struct {
	RollID   int64  `json:"roll_id"`
	PlayerID int64  `json:"player_id"`
	Voted    bool   `json:"voted"`
	Vote     *int16 `json:"vote,omitempty"`
}

// redactVotesForViewer returns the votes array tailored to viewerID. During
// stage='voting', other players' vote values are hidden; the viewer's own
// vote is always visible.
func redactVotesForViewer(
	votes []dbgen.DifficultyVote,
	stage string,
	viewerID int64,
) []voteView {
	out := make([]voteView, 0, len(votes))
	hide := stage == stageVoting
	for _, v := range votes {
		view := voteView{RollID: v.RollID, PlayerID: v.PlayerID, Voted: true}
		if !hide || v.PlayerID == viewerID {
			vv := v.Vote
			view.Vote = &vv
		}
		out = append(out, view)
	}
	return out
}

// ── GetActiveRollForGame ─────────────────────────────────────────────────────

// GetActiveRollForGame handles GET /api/tables/:id/rolls/active.
func GetActiveRollForGame(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		ctx := r.Context()
		rolls, err := s.Q.ListDiceRollsByGame(ctx, gameID)
		if err != nil {
			respond(w, http.StatusOK, map[string]any{
				"roll": nil, "dice": []any{}, "votes": []any{}, "participants": []any{},
			})
			return
		}

		var active *dbgen.DiceRoll
		// 1) The latest still-open roll, if any.
		for i := len(rolls) - 1; i >= 0; i-- {
			if rolls[i].Result == nil {
				active = new(rolls[i])
				break
			}
		}
		// 2) Otherwise, a resolved roll whose plan is still being resolved. Its
		//    make/mar outcome drives the plan's option picker, and once the roll
		//    has resolved this endpoint is the only place a reloading client can
		//    recover that outcome — without it the resolution UI goes blank
		//    (every branch keys off the roll outcome) until choices are applied.
		if active == nil {
			for i := len(rolls) - 1; i >= 0; i-- {
				if rolls[i].PlanID == nil {
					continue
				}
				plan, err := s.Q.GetPlanByID(ctx, *rolls[i].PlanID)
				if err == nil && plan.Status == model.PlanResolving {
					active = new(rolls[i])
					break
				}
			}
		}
		if active == nil {
			respond(w, http.StatusOK, map[string]any{
				"roll":         nil,
				"dice":         []dbgen.DiceRollDice{},
				"votes":        []voteView{},
				"participants": []dbgen.DiceRollParticipant{},
			})
			return
		}

		dice, _ := s.Q.ListDiceByRoll(ctx, active.ID)
		votes, _ := s.Q.ListVotesByRoll(ctx, active.ID)
		parts, _ := s.Q.ListParticipantsByRoll(ctx, active.ID)

		respond(w, http.StatusOK, map[string]any{
			"roll":         active,
			"dice":         dice,
			"votes":        redactVotesForViewer(votes, active.Stage, player.ID),
			"participants": parts,
		})
	}
}

// ── CreateRoll ───────────────────────────────────────────────────────────────

// validateActorContext checks that actorID is a player in gameID, and that
// any provided sceneID / planID matches the actor (focus_player_id /
// preparer_id respectively). Writes an error response and returns false on
// any mismatch.
func validateActorContext(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	gameID, actorID int64,
	sceneID, planID *int64,
) bool {
	actor, err := q.GetPlayerByID(ctx, actorID)
	if err != nil || actor.GameID != gameID {
		respondErr(w, http.StatusBadRequest, "actor is not a member of this table")
		return false
	}
	if sceneID != nil {
		scene, err := q.GetSceneByID(ctx, *sceneID)
		if err != nil || scene.GameID != gameID {
			respondErr(w, http.StatusBadRequest, "scene not found")
			return false
		}
		if scene.FocusPlayerID != actorID {
			respondErr(w, http.StatusConflict, "actor must be the scene's focus player")
			return false
		}
	}
	if planID != nil {
		plan, err := q.GetPlanByID(ctx, *planID)
		if err != nil || plan.GameID != gameID {
			respondErr(w, http.StatusBadRequest, "plan not found")
			return false
		}
		if plan.PreparerID != actorID {
			respondErr(w, http.StatusConflict, "actor must be the plan's preparer")
			return false
		}
	}
	return true
}

// CreateRoll handles POST /api/tables/:id/rolls.
//
// Request body:
//
//	{"actor_id": N, "difficulty": 1..6, "scene_id": N?, "plan_id": N?}
//
// The caller specifies the actor explicitly. If scene_id is given, actor_id
// must equal scene.focus_player_id; if plan_id is given, must equal
// plan.preparer_id. Roll starts at stage='decide_vote' with one participant
// row per game player (actor with intent='aid').
func CreateRoll(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
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
			ActorID    int64  `json:"actor_id"`
			Difficulty int16  `json:"difficulty"`
			SceneID    *int64 `json:"scene_id"`
			PlanID     *int64 `json:"plan_id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Difficulty < 1 || body.Difficulty > 6 {
			respondErr(w, http.StatusBadRequest, "difficulty must be between 1 and 6")
			return
		}
		if body.ActorID == 0 {
			respondErr(w, http.StatusBadRequest, "actor_id is required")
			return
		}

		ctx := r.Context()
		if !validateActorContext(ctx, w, s.Q, gameID, body.ActorID, body.SceneID, body.PlanID) {
			return
		}
		// One in-flight interactive roll per game (friendly pre-check; the
		// uq_one_open_roll_per_game index is the atomic backstop below).
		if blockIfOpenRoll(ctx, w, r, s.Q, gameID) {
			return
		}

		var roll dbgen.DiceRoll
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			r2, cErr := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
				GameID:     gameID,
				PlanID:     body.PlanID,
				RowNumber:  new(game.CurrentRow),
				ActorID:    body.ActorID,
				Difficulty: body.Difficulty,
				Stage:      stageDecideVote,
			})
			if cErr != nil {
				return cErr // preserved so a lost race maps to 409 below
			}
			roll = r2

			for range 2 {
				if _, dErr := q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
					RollID:           roll.ID,
					PlayerID:         body.ActorID,
					IsInterference:   false,
					LeveragedAssetID: nil,
				}); dErr != nil {
					return errors.New("could not create base dice")
				}
			}
			return seedRollParticipants(ctx, q, gameID, roll.ID, body.ActorID)
		})
		if err != nil {
			if isUniqueViolation(err, openRollConstraint) {
				respondErr(w, http.StatusConflict, openRollBusyMsg)
				return
			}
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		broadcastEvent(manager, gameID, model.EventRollCreated, model.RollCreatedPayload{Roll: roll})
		respond(w, http.StatusCreated, map[string]any{"roll": roll})
	}
}

// ── GetRoll ──────────────────────────────────────────────────────────────────

// GetRoll handles GET /api/rolls/:rollId.
func GetRoll(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roll, player, ok := requireRollAccess(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		dice, _ := s.Q.ListDiceByRoll(ctx, roll.ID)
		votes, _ := s.Q.ListVotesByRoll(ctx, roll.ID)
		parts, _ := s.Q.ListParticipantsByRoll(ctx, roll.ID)
		respond(w, http.StatusOK, map[string]any{
			"roll":         roll,
			"dice":         dice,
			"votes":        redactVotesForViewer(votes, roll.Stage, player.ID),
			"participants": parts,
		})
	}
}

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
