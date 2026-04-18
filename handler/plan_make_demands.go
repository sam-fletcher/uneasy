package handler

// handler/plan_make_demands.go — Make Demands plan handler (Phase 3d).
//
// Make Demands (power, variable delay) targets another unresolved plan. The
// demand lands on the row just before the target plan's row (or immediately,
// if the target is on the current row). Difficulty is the target plan's
// baseline plus the demander's power-rank deficit vs. the target's preparer.
//
// On a made roll, the demander and target's preparer alternate drafting the
// four demand options — control_leverage, keep_or_change_target, keep_assets,
// perform_steps — in power-rank order (higher-ranked = lower rank number
// picks first). Winners are persisted on the demand plan's
// demand_option_winners column so the target plan's resolution can consult
// them without re-walking the demand.
//
// On a marred roll, the target of the demand may prepare a free counter-
// demand (Stage 5). Until that counter lands (or the target waives it) the
// demand plan is not marked complete.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

const (
	demandEventPrepared       = "demand.prepared"
	demandEventResolved       = "demand.resolved"
	demandEventDraftPick      = "demand.draft_pick"
	demandEventCounterPending = "demand.counter_pending"
)

func init() {
	RegisterPlan(model.PlanMakeDemands, mdHandler{})
}

type mdHandler struct{}

func (mdHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryPower, Delay: -1}
}

func (mdHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (int16, string) {
	if v.TargetPlanID == nil {
		return 0, "make_demands requires target_plan_id"
	}
	target, err := v.Q.GetPlanByID(ctx, *v.TargetPlanID)
	if err != nil {
		return 0, "target plan not found"
	}
	if target.GameID != v.Game.ID {
		return 0, "target plan is not in this game"
	}
	if target.Status == model.PlanResolved || target.Status == model.PlanCancelled {
		return 0, "target plan is already resolved or cancelled"
	}
	if target.PlanType == model.PlanMakeWar {
		return 0, "Make War cannot be the target of a demand"
	}
	if target.PreparerID == v.Player.ID {
		return 0, "you cannot demand against your own plan"
	}
	existing, err := v.Q.GetPlansTargeting(ctx, &target.ID)
	if err != nil {
		return 0, "could not check existing demands"
	}
	for _, d := range existing {
		if d.Status != model.PlanResolved && d.Status != model.PlanCancelled {
			return 0, "another demand already targets that plan"
		}
	}
	row := target.RowNumber - 1
	if row < v.Game.CurrentRow {
		row = v.Game.CurrentRow
	}
	return row, ""
}

func (mdHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	if plan.TargetedPlanID == nil {
		return 0, errors.New("make_demands plan has no targeted plan")
	}
	target, err := q.GetPlanByID(ctx, *plan.TargetedPlanID)
	if err != nil {
		return 0, fmt.Errorf("load target plan: %w", err)
	}
	targetHandler, ok := GetHandler(target.PlanType)
	if !ok {
		return 0, fmt.Errorf("no handler for target plan type %s", target.PlanType)
	}
	targetRes := loadResolutionData(target.ResolutionData)
	targetDiff, err := targetHandler.ComputeDifficulty(ctx, q, &target, &targetRes)
	if err != nil {
		return 0, fmt.Errorf("compute target difficulty: %w", err)
	}
	demanderRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryPower)
	if err != nil {
		return 0, fmt.Errorf("load demander power rank: %w", err)
	}
	targetRank, err := playerRankInCategory(ctx, q, plan.GameID, target.PreparerID, model.CategoryPower)
	if err != nil {
		return 0, fmt.Errorf("load target power rank: %w", err)
	}
	if targetRank < demanderRank {
		return targetDiff + (demanderRank - targetRank), nil
	}
	return targetDiff, nil
}

// OnPrepare is a no-op beyond the broadcast: the targeted_plan_id column is
// populated by PreparePlan after the row is created.
func (mdHandler) OnPrepare(_ context.Context, deps *PlanDeps, plan *dbgen.Plan) error {
	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(demandEventPrepared, model.PlanPayload{Plan: *plan})
	}
	return nil
}

func (mdHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	diff, err := (mdHandler{}).ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, fmt.Errorf("load game: %w", err)
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, diff, plan.PreparerID)
}

// ApplyChoice records the result via the standard MakeChoice endpoint; the
// draft itself flows through /draft-choice. On a marred demand, the counter-
// demand window opens and is consumed via /counter-demand (Stage 5).
func (mdHandler) ApplyChoice(
	_ context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	result string,
) error {
	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(demandEventResolved, map[string]any{
			"plan_id": plan.ID,
			"result":  result,
		})
		if result == marOutcome {
			h.BroadcastEvent(demandEventCounterPending, map[string]any{
				"plan_id": plan.ID,
			})
		}
	}
	return nil
}

func (mdHandler) CanComplete(plan *dbgen.Plan, resData *ResolutionData) error {
	if plan.Result == nil {
		return errors.New("demand has no result yet")
	}
	switch *plan.Result {
	case makeOutcome:
		if len(resData.DraftChoices) < 4 {
			return fmt.Errorf("draft incomplete: %d of 4 options picked", len(resData.DraftChoices))
		}
	case marOutcome:
		if !resData.CounterDemandPlaced {
			return errors.New("target must place or waive the counter-demand before completing")
		}
	}
	return nil
}

func (mdHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"draft-choice":   mdDraftChoiceHandler(deps),
		"counter-demand": mdCounterDemandHandler(deps),
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func mdCheckPlan(w http.ResponseWriter, plan *dbgen.Plan) bool {
	if plan.PlanType != model.PlanMakeDemands {
		respondErr(w, http.StatusBadRequest, "route is only for Make Demands plans")
		return false
	}
	return true
}

func validDemandOption(s string) bool {
	switch s {
	case gamepkg.DemandOptionControlLeverage,
		gamepkg.DemandOptionKeepOrChangeTarget,
		gamepkg.DemandOptionKeepAssets,
		gamepkg.DemandOptionPerformSteps:
		return true
	}
	return false
}

// mdDraftPickers returns (firstPicker, secondPicker) by power rank. The
// higher-ranked (lower rank number) player picks first. Power ranks are
// unique per (game, category) via DB constraint, so no tiebreaker is needed.
func mdDraftPickers(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, demanderID, targetPreparerID int64,
) (int64, int64, error) {
	demanderRank, err := playerRankInCategory(ctx, q, gameID, demanderID, model.CategoryPower)
	if err != nil {
		return 0, 0, fmt.Errorf("load demander power rank: %w", err)
	}
	targetRank, err := playerRankInCategory(ctx, q, gameID, targetPreparerID, model.CategoryPower)
	if err != nil {
		return 0, 0, fmt.Errorf("load target power rank: %w", err)
	}
	if demanderRank < targetRank {
		return demanderID, targetPreparerID, nil
	}
	return targetPreparerID, demanderID, nil
}

// ── draft-choice ─────────────────────────────────────────────────────────────
//
// Demander and target-plan preparer alternate draft picks. Body:
//
//	{"option": "control_leverage" | "keep_or_change_target" |
//	           "keep_assets"      | "perform_steps"}
func mdDraftChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !mdCheckPlan(w, plan) {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if plan.Result == nil || *plan.Result != makeOutcome {
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

		ctx := r.Context()
		target, err := deps.Q.GetPlanByID(ctx, *plan.TargetedPlanID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load target plan")
			return
		}
		if player.ID != plan.PreparerID && player.ID != target.PreparerID {
			respondErr(w, http.StatusForbidden, "only the demander or target preparer may draft")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		if len(resData.DraftChoices) >= 4 {
			respondErr(w, http.StatusConflict, "all four options have already been drafted")
			return
		}
		for _, c := range resData.DraftChoices {
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
		if len(resData.DraftChoices)%2 == 1 {
			expected = second
		}
		if player.ID != expected {
			respondErr(w, http.StatusConflict, "it is not your turn to pick")
			return
		}

		resData.DraftChoices = append(resData.DraftChoices, gamepkg.DraftChoice{
			PlayerID: player.ID,
			Option:   body.Option,
		})
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save draft pick")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(demandEventDraftPick, map[string]any{
				"plan_id":    plan.ID,
				"player_id":  player.ID,
				"option":     body.Option,
				"pick_index": len(resData.DraftChoices),
			})
		}

		// On the final pick, persist the winners map on the demand plan so
		// the target plan's resolution path can consult it cheaply.
		if len(resData.DraftChoices) == 4 {
			winners := gamepkg.DemandOptionWinners{}
			for _, c := range resData.DraftChoices {
				winners[c.Option] = c.PlayerID
			}
			raw, err := json.Marshal(winners)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not encode option winners")
				return
			}
			if err := deps.Q.SetDemandOptionWinners(ctx, dbgen.SetDemandOptionWinnersParams{
				ID:                  plan.ID,
				DemandOptionWinners: raw,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save option winners")
				return
			}
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":        plan.ID,
			"option":         body.Option,
			"picks_done":     len(resData.DraftChoices),
			"draft_complete": len(resData.DraftChoices) == 4,
		})
	}
}

// mdCounterDemandHandler is fully wired in Stage 5. Stage 3 mounts the route
// so the registry's ExtraRoutes surface matches the final API shape.
func mdCounterDemandHandler(_ *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondErr(w, http.StatusNotImplemented, "counter-demand is implemented in Stage 5")
	}
}
