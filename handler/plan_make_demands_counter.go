package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── draft-choice ─────────────────────────────────────────────────────────────
//
// Demander and target-plan preparer alternate draft picks. Body:
//
//	{"option": "control_leverage" | "keep_or_change_target" |
//	           "keep_assets"      | "perform_steps"}
//

func mdDraftChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeDemands) {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		ctx := r.Context()
		if outcome := mdRollOutcome(ctx, deps.Q, plan.ID); outcome != makeOutcome {
			respondErr(w, http.StatusConflict, "draft is only open after a made demand")
			return
		}
		if plan.TargetedPlanID == nil {
			respondErr(w, http.StatusInternalServerError, "demand has no targeted plan")
			return
		}

		var body struct {
			Option string `json:"option"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !validDemandOption(body.Option) {
			respondErr(w, http.StatusBadRequest, "unknown draft option")
			return
		}

		target, err := deps.Q.GetPlanByID(ctx, *plan.TargetedPlanID)
		if err != nil {
			respondInternalErr(w, r, "could not load target plan", err)
			return
		}
		if player.ID != plan.PreparerID && player.ID != target.PreparerID {
			respondErr(w, http.StatusForbidden, "only the demander or target preparer may draft")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		md := resData.EnsureMakeDemands()
		if len(md.DraftChoices) >= 4 {
			respondErr(w, http.StatusConflict, "all four options have already been drafted")
			return
		}
		for _, c := range md.DraftChoices {
			if c.Option == body.Option {
				respondErr(w, http.StatusConflict, "that option has already been picked")
				return
			}
		}

		first, second, err := mdDraftPickers(ctx, deps.Q, plan.GameID, plan.PreparerID, target.PreparerID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		expected := first
		if len(md.DraftChoices)%2 == 1 {
			expected = second
		}
		if player.ID != expected {
			respondErr(w, http.StatusConflict, "it is not your turn to pick")
			return
		}

		md.DraftChoices = append(md.DraftChoices, gamepkg.DraftChoice{
			PlayerID: player.ID,
			Option:   body.Option,
		})
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save draft pick", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, demandEventDraftPick, map[string]any{
			"plan_id":    plan.ID,
			"player_id":  player.ID,
			"option":     body.Option,
			"pick_index": len(md.DraftChoices),
		})

		// On the final pick, persist the winners map on the demand plan so
		// the target plan's resolution path can consult it cheaply.
		if len(md.DraftChoices) == 4 {
			if err := mdPersistDraftWinners(ctx, deps, plan, md.DraftChoices); err != nil {
				respondInternalErr(w, r, "could not save option winners", err)
				return
			}
		}

		// Each pick alternates the acting player; the final pick clears
		// the override entirely. Broadcast so the WaitingOnBar updates.
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":        plan.ID,
			"option":         body.Option,
			"picks_done":     len(md.DraftChoices),
			"draft_complete": len(md.DraftChoices) == 4,
		})
	}
}

// ── counter-demand (Stage 5) ─────────────────────────────────────────────────
//
// Mounted on the *demand* plan: POST /api/plans/:demandPlanId/counter-demand
// with body {"target_plan_id": int64 | null}. Callable only by the target of a
// marred demand (= the preparer of the plan the demand targeted).
//
//   - If target_plan_id is set, synthesizes a free Make Demands plan targeting
//     that plan immediately, bypassing token / eligibility / peer checks.
//     Row = max(targetPlan.row - 1, game.current_row).
//   - If target_plan_id is null, records a pending_counter_demands row. The
//     original demander's next PreparePlan will consume it and synthesize the
//     counter then.
//
// Either path marks the demand's CounterDemandPlaced flag so the demand plan
// can be completed.
func mdCounterDemandHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeDemands) {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "demand is not in resolving status")
			return
		}
		ctx := r.Context()
		if outcome := mdRollOutcome(ctx, deps.Q, plan.ID); outcome != marOutcome {
			respondErr(w, http.StatusConflict, "counter-demand is only open after a marred demand")
			return
		}
		if plan.TargetedPlanID == nil {
			respondErr(w, http.StatusInternalServerError, "demand has no targeted plan")
			return
		}

		targetOfDemand, err := deps.Q.GetPlanByID(ctx, *plan.TargetedPlanID)
		if err != nil {
			respondInternalErr(w, r, "could not load target plan", err)
			return
		}
		if player.ID != targetOfDemand.PreparerID {
			respondErr(w, http.StatusForbidden, "only the target of the demand may counter-demand")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		md := resData.EnsureMakeDemands()
		if md.CounterDemandPlaced {
			respondErr(w, http.StatusConflict, "counter-demand has already been placed or deferred")
			return
		}

		var body struct {
			TargetPlanID *int64 `json:"target_plan_id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}

		var counterPlanID *int64
		if body.TargetPlanID != nil {
			counter, errMsg, status := synthesizeCounterDemand(ctx, deps, &game, player.ID, *body.TargetPlanID)
			if errMsg != "" {
				respondErr(w, status, errMsg)
				return
			}
			counterPlanID = &counter.ID
		} else {
			pending, err := deps.Q.CreatePendingCounterDemand(ctx, dbgen.CreatePendingCounterDemandParams{
				GameID:            plan.GameID,
				DemandingPlayerID: plan.PreparerID,
				TargetPlayerID:    player.ID,
				OriginPlanID:      plan.ID,
			})
			if err != nil {
				respondInternalErr(w, r, "could not record pending counter-demand", err)
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, demandEventCounterPending, map[string]any{
				"plan_id":          plan.ID,
				"pending_id":       pending.ID,
				"demanding_player": plan.PreparerID,
				"target_player":    player.ID,
			})
		}

		md.CounterDemandPlaced = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save counter-demand state", err)
			return
		}

		counterer := playerDisplayName(ctx, deps.Q, player.ID)
		if counterPlanID != nil {
			broadcastEvent(deps.Manager, plan.GameID, demandEventCounterPlaced, map[string]any{
				"plan_id":         plan.ID,
				"counter_plan_id": *counterPlanID,
			})
			mdLog(ctx, deps, plan, model.SeverityImportant,
				fmt.Sprintf("%s answered the marred demand with a counter-demand.", counterer))
		} else {
			mdLog(ctx, deps, plan, model.SeverityDefault,
				fmt.Sprintf("%s deferred their counter-demand to their next plan.", counterer))
		}
		// CounterDemandPlaced just flipped, so the row transitions out of
		// await_demand_counter back to plan_resolving (until the demand
		// is completed).
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":         plan.ID,
			"counter_plan_id": counterPlanID,
			"deferred":        body.TargetPlanID == nil,
		})
	}
}

// consumePendingCounterDemandFor checks whether a pending counter-demand is
// waiting on newPlan.PreparerID. If so, synthesizes a Make Demands plan owned
// by the deferred target and targeting the newly-created plan, then marks the
// pending row resolved. Returns the new counter-demand plan's ID, or nil if
// no pending row existed (or if synthesis failed — errors are swallowed so a
// pending-row glitch never breaks the preparer's own successful plan
// creation).
func consumePendingCounterDemandFor(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	game *dbgen.Game,
	newPlan *dbgen.Plan,
) *int64 {
	pending, err := q.ConsumePendingCounterDemand(ctx, newPlan.PreparerID)
	if err != nil {
		return nil
	}
	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: manager}
	counter, errMsg, _ := synthesizeCounterDemand(ctx, deps, game, pending.TargetPlayerID, newPlan.ID)
	if errMsg != "" {
		return nil
	}
	if err := q.ResolvePendingCounterDemand(ctx, dbgen.ResolvePendingCounterDemandParams{
		ID:             pending.ID,
		ResolvedPlanID: &counter.ID,
	}); err != nil {
		return nil
	}
	broadcastEvent(manager, game.ID, demandEventCounterPlaced, map[string]any{
		"plan_id":         pending.OriginPlanID,
		"counter_plan_id": counter.ID,
		"triggered_by":    newPlan.ID,
	})

	// Mark the origin demand's CounterDemandPlaced so it can be completed.
	if origin, err := q.GetPlanByID(ctx, pending.OriginPlanID); err == nil {
		resData := loadResolutionData(origin.ResolutionData)
		resData.EnsureMakeDemands().CounterDemandPlaced = true
		_ = saveResolutionData(ctx, q, origin.ID, resData)
	}

	return &counter.ID
}

// synthesizeCounterDemand creates a Make Demands plan owned by preparerID
// targeting targetPlanID, bypassing token / eligibility / peer checks.
// Returns (plan, "", 0) on success, or (_, errMsg, httpStatus) on failure.
func synthesizeCounterDemand(
	ctx context.Context,
	deps *PlanDeps,
	game *dbgen.Game,
	preparerID int64,
	targetPlanID int64,
) (*dbgen.Plan, string, int) {
	target, err := deps.Q.GetPlanByID(ctx, targetPlanID)
	if err != nil {
		return nil, "target plan not found", http.StatusBadRequest
	}
	if target.GameID != game.ID {
		return nil, "target plan is not in this game", http.StatusBadRequest
	}
	if target.Status == model.PlanResolved || target.Status == model.PlanCancelled {
		return nil, "target plan is already resolved or cancelled", http.StatusConflict
	}
	if target.PlanType == model.PlanMakeWar {
		return nil, "Make War cannot be the target of a demand", http.StatusBadRequest
	}
	if target.PreparerID == preparerID {
		return nil, "you cannot demand against your own plan", http.StatusBadRequest
	}
	existing, err := deps.Q.GetPlansTargeting(ctx, &target.ID)
	if err != nil {
		return nil, "could not check existing demands", http.StatusInternalServerError
	}
	for _, d := range existing {
		if d.Status != model.PlanResolved && d.Status != model.PlanCancelled {
			return nil, "another demand already targets that plan", http.StatusConflict
		}
	}

	if target.RowNumber == nil {
		return nil, "target plan has not been assigned a row yet (its delay reveal is still open)", http.StatusConflict
	}
	if target.Status == model.PlanResolving {
		return nil, "target plan is already resolving — a counter-demand cannot slot in before it", http.StatusConflict
	}
	row, rowOrder := gamepkg.DemandPlacement(*target.RowNumber, target.RowOrder)
	if row > publicRecordRowCount {
		return nil, "counter-demand would be placed past row 13", http.StatusConflict
	}

	if err := deps.Q.ShiftRowOrderAtOrAfter(ctx, dbgen.ShiftRowOrderAtOrAfterParams{
		GameID:    game.ID,
		RowNumber: new(row),
		RowOrder:  rowOrder,
	}); err != nil {
		return nil, "could not shift row order: " + err.Error(), http.StatusInternalServerError
	}

	plan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        game.ID,
		PlanType:      model.PlanMakeDemands,
		Category:      model.CategoryPower,
		PreparerID:    preparerID,
		RowNumber:     new(row),
		RowOrder:      rowOrder,
		PreparedAtRow: game.CurrentRow,
	})
	if err != nil {
		return nil, "could not create counter-demand plan: " + err.Error(), http.StatusInternalServerError
	}
	err = deps.Q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID:             plan.ID,
		TargetedPlanID: &target.ID,
	})
	if err != nil {
		return nil, "could not persist counter-demand target: " + err.Error(), http.StatusInternalServerError
	}
	refreshed, err := deps.Q.GetPlanByID(ctx, plan.ID)
	if err == nil {
		plan = refreshed
	}

	if h, ok := deps.Manager.Get(game.ID); ok {
		h.BroadcastEvent(model.EventPlanPrepared, model.PlanPayload{Plan: plan})
		h.BroadcastEvent(demandEventPrepared, model.PlanPayload{Plan: plan})
	}
	EmitPlanPrepared(ctx, deps.Q, deps.Manager, plan)
	return &plan, "", 0
}
