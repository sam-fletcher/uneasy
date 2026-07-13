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

		// During the Shake-Up, "the active roll" is the open shake-up roll (if
		// any) — a direct, indexed lookup, rather than the interactive-roll scan
		// below (which only ever tracks is_shake_up=FALSE rolls and would need a
		// plan lookup per candidate that's meaningless once plans no longer exist).
		if game, gerr := s.Q.GetGameByID(ctx, gameID); gerr == nil && game.Phase == model.PhaseShakeUp {
			active, aerr := s.Q.GetOpenShakeUpRollByGame(ctx, gameID)
			if aerr != nil {
				respond(w, http.StatusOK, map[string]any{
					"roll":         nil,
					"dice":         []dbgen.DiceRollDice{},
					"votes":        []voteView{},
					"participants": []dbgen.DiceRollParticipant{},
				})
				return
			}
			dice, _ := s.Q.ListDiceByRoll(ctx, active.ID)
			parts, _ := s.Q.ListParticipantsByRoll(ctx, active.ID)
			respond(w, http.StatusOK, map[string]any{
				"roll":         active,
				"dice":         dice,
				"votes":        []voteView{},
				"participants": parts,
			})
			return
		}

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
			return seedRollParticipants(ctx, q, gameID, roll.ID, body.ActorID, body.PlanID)
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
		// A new roll immediately becomes the top-of-chain row-state gate
		// (await_dice_roll), overriding whatever the row was showing before.
		broadcastRowState(ctx, s.Q, manager, gameID)
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
