package handler

// handler/plan_make_war.go — Make War plan handler (Phase 3d).
//
// Make War (power, delay: variable) declares war against one or more other
// players. All war participants simultaneously reveal a die; the plan's
// delay equals ceil(average) of the revealed faces.
//
// The plan's resolution is purely narrative (no dice roll, no make/mar).
// The focus player posts one scene when the plan reaches its row, then
// completes. The war itself persists across rows in the `wars` table until
// peace is agreed or one side fully surrenders.
//
// Cost of battle — the recurring mechanic — is implemented in two places:
//   - turn.go's advanceRowInner gates row advance on unpaid costs.
//   - /pay-battle-cost, /propose-peace, /vote-peace, /accept-peace are the
//     endpoints players use to resolve those costs and negotiate an ending.
//
// Extra routes:
//
//	POST /api/plans/:planId/join-war           — non-preparer joins a side.
//	POST /api/plans/:planId/post-war-scene     — focus player marks the
//	                                              one-time declaration scene.
//	POST /api/plans/:planId/pay-battle-cost    — pay one opponent's cost
//	                                              (optional surrender modifier).
//	POST /api/plans/:planId/pay-war-entry      — late joiner pays one
//	                                              existing opponent's cost.
//	POST /api/plans/:planId/take-surrender-asset — claim one asset from a
//	                                              surrendered opposing player.
//	POST /api/plans/:planId/propose-peace      — open a peace proposal.
//	POST /api/plans/:planId/vote-peace         — accept/reject open proposal.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanMakeWar, mwHandler{})
}

type mwHandler struct{}

func (mwHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryPower, Delay: -1}
}

func (mwHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (int16, string) {
	if len(v.EnemyPlayerIDs) == 0 {
		return 0, "make_war requires at least one entry in enemy_player_ids"
	}
	seen := map[int64]bool{v.Player.ID: true}
	for _, id := range v.EnemyPlayerIDs {
		if id == v.Player.ID {
			return 0, "cannot declare war on yourself"
		}
		if seen[id] {
			return 0, "enemy_player_ids contains duplicates"
		}
		seen[id] = true
		p, err := v.Q.GetPlayerByID(ctx, id)
		if err != nil || p.GameID != v.Game.ID {
			return 0, fmt.Sprintf("enemy player %d is not in this game", id)
		}
	}
	// Row placeholder; the actual row_number is set after the delay reveal.
	return 0, ""
}

// OnPrepare creates the war row, enrols every participant on the correct
// side, and opens a simultaneous reveal for the delay. Enemy list is read
// from resolution_data (persisted by PreparePlan before this hook fires).
func (mwHandler) OnPrepare(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) error {
	resData := loadResolutionData(plan.ResolutionData)
	if len(resData.WarEnemyPlayerIDs) == 0 {
		return errors.New("make_war: missing enemy list in resolution_data")
	}

	war, err := deps.Q.CreateWar(ctx, dbgen.CreateWarParams{
		GameID:       plan.GameID,
		OriginPlanID: plan.ID,
		StartedAtRow: plan.PreparedAtRow,
	})
	if err != nil {
		return fmt.Errorf("create war: %w", err)
	}

	// Side 1: preparer.
	err = deps.Q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
		WarID:       war.ID,
		PlayerID:    plan.PreparerID,
		Side:        gamepkg.WarSideDeclarer,
		JoinedAtRow: plan.PreparedAtRow,
	})
	if err != nil {
		return fmt.Errorf("enrol preparer: %w", err)
	}
	// Side 2: declared enemies.
	for _, enemyID := range resData.WarEnemyPlayerIDs {
		if err := deps.Q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
			WarID:       war.ID,
			PlayerID:    enemyID,
			Side:        gamepkg.WarSideEnemy,
			JoinedAtRow: plan.PreparedAtRow,
		}); err != nil {
			return fmt.Errorf("enrol enemy %d: %w", enemyID, err)
		}
	}

	// Open the simultaneous reveal (all participants submit a die face).
	reveal, errReveal := deps.Q.CreateSimultaneousReveal(ctx, dbgen.CreateSimultaneousRevealParams{
		GameID:     plan.GameID,
		PlanID:     &plan.ID,
		RevealType: "make_war_delay",
	})
	if errReveal != nil {
		return fmt.Errorf("create delay reveal: %w", errReveal)
	}
	all := append([]int64{plan.PreparerID}, resData.WarEnemyPlayerIDs...)
	for _, pid := range all {
		if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
			RevealID: reveal.ID,
			PlayerID: pid,
		}); err != nil {
			return fmt.Errorf("add reveal entry for %d: %w", pid, err)
		}
	}

	resData.WarID = &war.ID
	resData.DelayRevealID = &reveal.ID
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return fmt.Errorf("save war resolution data: %w", err)
	}
	return nil
}

func (mwHandler) ComputeDifficulty(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	// Make War has no dice roll; difficulty is N/A.
	return 0, nil
}

// OnResolve marks the plan as pre-resolved narratively. No roll is created;
// the focus player posts a declaration scene via /post-war-scene, then
// calls /complete. Setting the plan's result up-front lets CompletePlan
// finalise without a roll outcome.
func (mwHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	res := makeOutcome
	if err := deps.Q.SetPlanResultPreserveStatus(ctx, dbgen.SetPlanResultPreserveStatusParams{
		ID:     plan.ID,
		Result: &res,
	}); err != nil {
		return nil, fmt.Errorf("pre-record war result: %w", err)
	}
	return nil, nil
}

// ApplyChoice is unused — Make War has no make/mar options.
func (mwHandler) ApplyChoice(
	_ context.Context,
	_ *PlanDeps,
	_ *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	_ string,
) error {
	return nil
}

// CanComplete requires the focus player to have posted the war declaration
// scene so the record isn't finalised before the narrative moment.
func (mwHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	if !resData.WarScenePosted {
		return errors.New("post the war's declaration scene before completing")
	}
	return nil
}

func (mwHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"join-war":             mwJoinHandler(deps),
		"post-war-scene":       mwPostSceneHandler(deps),
		"pay-battle-cost":      mwPayBattleCostHandler(deps),
		"pay-war-entry":        mwPayWarEntryHandler(deps),
		"take-surrender-asset": mwTakeSurrenderAssetHandler(deps),
		"propose-peace":        mwProposePeaceHandler(deps),
		"vote-peace":           mwVotePeaceHandler(deps),
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// mwLoadWar resolves the war for a Make War plan. Returns 404 if no war row
// exists for this plan (the plan was cancelled before the delay resolved, or
// called on a malformed plan), 500 on any other DB error.
func mwLoadWar(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	plan *dbgen.Plan,
) (dbgen.War, bool) {
	war, err := q.GetWarByOriginPlan(ctx, plan.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondErr(w, http.StatusNotFound, "no war exists for this plan")
		} else {
			respondErr(w, http.StatusInternalServerError, "could not load war")
		}
		return dbgen.War{}, false
	}
	return war, true
}

// ── join-war ─────────────────────────────────────────────────────────────────
//
// A non-enrolled player joins a side. Allowed free-of-charge while the
// delay reveal is still open (declaration phase). Once the reveal has
// completed (war is active) joiners are still admitted here but must
// complete the full cost-of-battle sequence (one payment per opposing
// opponent) before they are counted as an active participant for peace
// voting — that gating lives in the peace-voting endpoint.
func mwJoinHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		var body struct {
			Side int16 `json:"side"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Side != gamepkg.WarSideDeclarer && body.Side != gamepkg.WarSideEnemy {
			respondErr(w, http.StatusBadRequest, "side must be 1 or 2")
			return
		}

		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		if _, err := deps.Q.GetWarParticipant(ctx, dbgen.GetWarParticipantParams{
			WarID: war.ID, PlayerID: player.ID,
		}); err == nil {
			respondErr(w, http.StatusConflict, "you are already a participant in this war")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		// If the delay reveal is still open, joiners are free — they reveal with
		// everyone else and count as full participants. Once the reveal closes
		// (war becomes active) joiners must pay a full cost of battle against
		// every existing opposing opponent before becoming active themselves.
		resData := loadResolutionData(plan.ResolutionData)
		revealOpen := false
		if resData.DelayRevealID != nil {
			reveal, err := deps.Q.GetSimultaneousReveal(ctx, *resData.DelayRevealID)
			if err == nil && !reveal.IsComplete {
				revealOpen = true
			}
		}

		if revealOpen {
			if err := deps.Q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
				WarID:       war.ID,
				PlayerID:    player.ID,
				Side:        body.Side,
				JoinedAtRow: game.CurrentRow,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not join war")
				return
			}
			_ = deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
				RevealID: *resData.DelayRevealID,
				PlayerID: player.ID,
			})
		} else {
			if err := deps.Q.AddWarParticipantPending(ctx, dbgen.AddWarParticipantPendingParams{
				WarID:       war.ID,
				PlayerID:    player.ID,
				Side:        body.Side,
				JoinedAtRow: game.CurrentRow,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not join war")
				return
			}
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarPlayerJoined, model.WarPlayerJoinedPayload{
				WarID: war.ID, PlayerID: player.ID, Side: body.Side,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"war_id": war.ID, "player_id": player.ID, "side": body.Side,
		})
	}
}

// ── post-war-scene ───────────────────────────────────────────────────────────
//
// Focus player marks the one-time narrative beat complete. Gates /complete.
func mwPostSceneHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, plan, _, ok := requirePlanFocus(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		resData.WarScenePosted = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save scene state")
			return
		}
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "scene_posted": true})
	}
}

// ── applyMakeWarDelayResult ──────────────────────────────────────────────────

// applyMakeWarDelayResult is invoked by reveals.go when the make_war_delay
// simultaneous reveal completes. It sets the plan's row_number to
// current_row + resultDelay (cancelling the plan — and the nascent war — if
// the target exceeds row 13) and broadcasts war.declared.
func applyMakeWarDelayResult(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	planID int64,
	resultDelay int16,
) {
	plan, err := q.GetPlanByID(ctx, planID)
	if err != nil {
		return
	}
	game, err := q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return
	}

	targetRow := game.CurrentRow + resultDelay

	resData := loadResolutionData(plan.ResolutionData)

	if targetRow > publicRecordRowCount {
		_ = q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID:     planID,
			Status: model.PlanCancelled,
		})
		if resData.WarID != nil {
			_ = q.EndWar(ctx, dbgen.EndWarParams{
				ID:         *resData.WarID,
				EndReason:  new(gamepkg.WarEndPeace),
				EndedAtRow: &game.CurrentRow,
			})
		}
		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventPlanResolved, model.PlanResolvedPayload{
				PlanID: planID,
				Result: "cancelled",
			})
		}
		return
	}

	_ = q.SetPlanRowNumber(ctx, dbgen.SetPlanRowNumberParams{
		ID:        planID,
		RowNumber: targetRow,
	})

	if resData.WarID == nil {
		return
	}

	parts, err := q.ListWarParticipants(ctx, *resData.WarID)
	if err == nil {
		infos := make([]model.WarParticipantInfo, 0, len(parts))
		for _, p := range parts {
			infos = append(infos, model.WarParticipantInfo{PlayerID: p.PlayerID, Side: p.Side})
		}
		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarDeclared, model.WarDeclaredPayload{
				PlanID:       planID,
				WarID:        *resData.WarID,
				Participants: infos,
				TargetRow:    targetRow,
			})
		}
	}
}
