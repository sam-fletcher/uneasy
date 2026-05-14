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

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

const (
	demandEventPrepared       = "demand.prepared"
	demandEventResolved       = "demand.resolved"
	demandEventDraftPick      = "demand.draft_pick"
	demandEventCounterPending = "demand.counter_pending"
	demandEventLeverageSet    = "demand.leverage_set"
	demandEventRetargeted     = "demand.retargeted"
	demandEventCounterPlaced  = "demand.counter_placed"
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
	return gamepkg.DemandRowPlacement(target.RowNumber, v.Game.CurrentRow), ""
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
	return gamepkg.MakeDemandsDifficulty(targetDiff, demanderRank, targetRank), nil
}

// OnPrepare is a no-op beyond the broadcast: the targeted_plan_id column is
// populated by PreparePlan after the row is created.
func (mdHandler) OnPrepare(_ context.Context, deps *PlanDeps, plan *dbgen.Plan) error {
	broadcastEvent(deps.Manager, plan.GameID, demandEventPrepared, model.PlanPayload{Plan: *plan})
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
		"draft-choice":    mdDraftChoiceHandler(deps),
		"counter-demand":  mdCounterDemandHandler(deps),
		"demand-leverage": mdDemandLeverageHandler(deps),
		"demand-retarget": mdDemandRetargetHandler(deps),
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

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
	first, second := gamepkg.DemandDraftPickers(demanderID, targetPreparerID, demanderRank, targetRank)
	return first, second, nil
}

// ── draft-choice ─────────────────────────────────────────────────────────────
//
// Demander and target-plan preparer alternate draft picks. Body:
//
//	{"option": "control_leverage" | "keep_or_change_target" |
//	           "keep_assets"      | "perform_steps"}
//
//nolint:gocognit // demand draft state machine (alternating-pick + recursive demand)
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

		broadcastEvent(deps.Manager, plan.GameID, demandEventDraftPick, map[string]any{
			"plan_id":    plan.ID,
			"player_id":  player.ID,
			"option":     body.Option,
			"pick_index": len(resData.DraftChoices),
		})

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
		if plan.Result == nil || *plan.Result != marOutcome {
			respondErr(w, http.StatusConflict, "counter-demand is only open after a marred demand")
			return
		}
		if plan.TargetedPlanID == nil {
			respondErr(w, http.StatusInternalServerError, "demand has no targeted plan")
			return
		}

		ctx := r.Context()
		targetOfDemand, err := deps.Q.GetPlanByID(ctx, *plan.TargetedPlanID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load target plan")
			return
		}
		if player.ID != targetOfDemand.PreparerID {
			respondErr(w, http.StatusForbidden, "only the target of the demand may counter-demand")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		if resData.CounterDemandPlaced {
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
			respondErr(w, http.StatusInternalServerError, "could not load game")
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
				respondErr(w, http.StatusInternalServerError, "could not record pending counter-demand")
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, demandEventCounterPending, map[string]any{
				"plan_id":          plan.ID,
				"pending_id":       pending.ID,
				"demanding_player": plan.PreparerID,
				"target_player":    player.ID,
			})
		}

		resData.CounterDemandPlaced = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save counter-demand state")
			return
		}

		if counterPlanID != nil {
			broadcastEvent(deps.Manager, plan.GameID, demandEventCounterPlaced, map[string]any{
				"plan_id":         plan.ID,
				"counter_plan_id": *counterPlanID,
			})
		}

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
		resData.CounterDemandPlaced = true
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

	row := gamepkg.DemandRowPlacement(target.RowNumber, game.CurrentRow)
	if row > publicRecordRowCount {
		return nil, "counter-demand would be placed past row 13", http.StatusConflict
	}

	count, err := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    game.ID,
		RowNumber: row,
	})
	if err != nil {
		count = 0
	}

	plan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        game.ID,
		PlanType:      model.PlanMakeDemands,
		Category:      model.CategoryPower,
		PreparerID:    preparerID,
		RowNumber:     row,
		RowOrder:      int16(count),
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

// ── demand-leverage (Stage 4) ────────────────────────────────────────────────
//
// Mounted on the *target* plan: POST /api/plans/:planId/demand-leverage with
// body {"asset_ids": [int64]}. Callable only by the control_leverage winner of
// a resolved, made demand against this plan, while the target plan is still
// in its leverage window (status = resolving, roll open). Leverages the chosen
// subset of the target preparer's own assets onto the target plan's roll.
// The target preparer's own leverage of their own assets is separately blocked
// while a control_leverage winner exists (see handler/rolls.go LeverageRoll).
//
//nolint:gocognit // leverage rights handoff to demand winner
func mdDemandLeverageHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType == model.PlanMakeDemands {
			respondErr(w, http.StatusBadRequest, "demand-leverage is mounted on the target plan, not the demand plan")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "target plan is not in resolving status")
			return
		}

		ctx := r.Context()
		_, winners, err := gamepkg.DemandWinnersForTargetPlan(ctx, deps.Q, plan)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load demand winners")
			return
		}
		winnerID, ok := winners[gamepkg.DemandOptionControlLeverage]
		if !ok || winnerID == 0 {
			respondErr(w, http.StatusConflict, "no control_leverage winner on this plan")
			return
		}
		if player.ID != winnerID {
			respondErr(w, http.StatusForbidden, "only the control_leverage winner may set leverage here")
			return
		}

		roll, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if err != nil {
			respondErr(w, http.StatusConflict, "target plan has no open roll")
			return
		}
		if !rollIsOpen(&roll) {
			respondErr(w, http.StatusConflict, "roll is already resolved")
			return
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		existingDice, err := deps.Q.ListDiceByRoll(ctx, roll.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not list dice")
			return
		}
		committed := map[int64]struct{}{}
		for _, d := range existingDice {
			if d.LeveragedAssetID != nil {
				committed[*d.LeveragedAssetID] = struct{}{}
			}
		}

		for _, assetID := range body.AssetIDs {
			asset, err := deps.Q.GetAssetByID(ctx, assetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", assetID))
				return
			}
			if asset.OwnerID != plan.PreparerID {
				respondErr(w, http.StatusForbidden,
					fmt.Sprintf("asset %d does not belong to the target preparer", assetID))
				return
			}
			if asset.IsDestroyed {
				respondErr(w, http.StatusConflict, fmt.Sprintf("asset %d is destroyed", assetID))
				return
			}
			if _, dup := committed[assetID]; dup {
				continue
			}
			if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
				ID: assetID, IsLeveraged: true,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not leverage asset")
				return
			}
			// Target preparer's own dice would not be interference; these are
			// added on their behalf by the demand winner, so keep non-interference.
			if _, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
				RollID:           roll.ID,
				PlayerID:         plan.PreparerID,
				IsInterference:   plan.PreparerID != roll.ActorID,
				LeveragedAssetID: &assetID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not add leverage die")
				return
			}
			committed[assetID] = struct{}{}
		}

		broadcastEvent(deps.Manager, plan.GameID, demandEventLeverageSet, map[string]any{
			"plan_id":   plan.ID,
			"roll_id":   roll.ID,
			"asset_ids": body.AssetIDs,
			"player_id": player.ID,
		})

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"roll_id":   roll.ID,
			"asset_ids": body.AssetIDs,
		})
	}
}

// ── demand-retarget (Stage 4) ────────────────────────────────────────────────
//
// Mounted on the *target* plan: POST /api/plans/:planId/demand-retarget with
// body {"target_player_id"?, "target_asset_id"?}. Callable only by the
// keep_or_change_target winner. Re-validates the target plan's preparation
// rules with the proposed new target values (treating the target plan's own
// preparer as the nominal player) before persisting. Only valid while the
// target plan has not yet resolved its roll.
func mdDemandRetargetHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType == model.PlanMakeDemands {
			respondErr(w, http.StatusBadRequest, "demand-retarget is mounted on the target plan, not the demand plan")
			return
		}
		if plan.Status != model.PlanPending && plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "target plan has already resolved")
			return
		}

		ctx := r.Context()
		_, winners, err := gamepkg.DemandWinnersForTargetPlan(ctx, deps.Q, plan)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load demand winners")
			return
		}
		winnerID, ok := winners[gamepkg.DemandOptionKeepOrChangeTarget]
		if !ok || winnerID == 0 {
			respondErr(w, http.StatusConflict, "no keep_or_change_target winner on this plan")
			return
		}
		if player.ID != winnerID {
			respondErr(w, http.StatusForbidden, "only the keep_or_change_target winner may retarget")
			return
		}

		// Block retarget once the roll has been resolved — stakes have been locked.
		roll, errRoll := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if errRoll == nil && !rollIsOpen(&roll) {
			respondErr(w, http.StatusConflict, "target plan's roll has already resolved")
			return
		}
		if errRoll != nil {
			respondErr(w, http.StatusInternalServerError, fmt.Sprintf("could not load target plan's roll: %v", errRoll))
			return
		}

		var body struct {
			TargetPlayerID *int64 `json:"target_player_id"`
			TargetAssetID  *int64 `json:"target_asset_id"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		targetHandler, ok := GetHandler(plan.PlanType)
		if !ok {
			respondErr(w, http.StatusInternalServerError, "no handler for target plan type")
			return
		}
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}
		preparer, err := deps.Q.GetPlayerByID(ctx, plan.PreparerID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load target preparer")
			return
		}
		vc := &ValidationContext{
			Q:              deps.Q,
			Game:           &game,
			Player:         &preparer,
			TargetPlayerID: body.TargetPlayerID,
			TargetAssetID:  body.TargetAssetID,
		}
		if _, errMsg := targetHandler.ValidatePreparation(ctx, vc); errMsg != "" {
			respondErr(w, http.StatusBadRequest, "retarget invalid: "+errMsg)
			return
		}

		err = deps.Q.SetPlanTargets(ctx, dbgen.SetPlanTargetsParams{
			ID:             plan.ID,
			TargetPlayerID: body.TargetPlayerID,
			TargetAssetID:  body.TargetAssetID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update plan targets")
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, demandEventRetargeted, map[string]any{
			"plan_id":          plan.ID,
			"target_player_id": body.TargetPlayerID,
			"target_asset_id":  body.TargetAssetID,
			"player_id":        player.ID,
		})

		respond(w, http.StatusOK, map[string]any{
			"plan_id":          plan.ID,
			"target_player_id": body.TargetPlayerID,
			"target_asset_id":  body.TargetAssetID,
		})
	}
}
