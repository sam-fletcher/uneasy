package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

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
		_, winners, err := DemandWinnersForTargetPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not load demand winners", err)
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
			respondInternalErr(w, r, "could not list dice", err)
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
				respondInternalErr(w, r, "could not leverage asset", err)
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
				respondInternalErr(w, r, "could not add leverage die", err)
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
		_, winners, err := DemandWinnersForTargetPlan(ctx, deps.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not load demand winners", err)
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
			respondInternalErr(w, r, "could not load game", err)
			return
		}
		preparer, err := deps.Q.GetPlayerByID(ctx, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not load target preparer", err)
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
			respondInternalErr(w, r, "could not update plan targets", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, demandEventRetargeted, map[string]any{
			"plan_id":          plan.ID,
			"target_player_id": body.TargetPlayerID,
			"target_asset_id":  body.TargetAssetID,
			"player_id":        player.ID,
		})

		mdLog(ctx, deps, plan, model.SeverityImportant, fmt.Sprintf("%s re-aimed %s's %s under their demand.",
			playerDisplayName(ctx, deps.Q, player.ID),
			playerDisplayName(ctx, deps.Q, plan.PreparerID), planLabel(plan.PlanType)))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":          plan.ID,
			"target_player_id": body.TargetPlayerID,
			"target_asset_id":  body.TargetAssetID,
		})
	}
}
