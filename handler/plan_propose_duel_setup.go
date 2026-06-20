package handler

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── Elect Champion ────────────────────────────────────────────────────────────

// pduelElectChampionHandler: POST /api/plans/:planId/elect-champion
// Body: {"asset_id": N | null}. If asset_id is null or omitted, the player is
// signalling "I'll fight myself." If present, the asset must be a peer owned
// by the caller. The initiative-holder must declare first so the other side's
// UI knows when to unlock.
//
//nolint:gocognit // champion election with eligibility + auto-advance
func pduelElectChampionHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may elect a champion")
			return
		}

		var body struct {
			AssetID *int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()

		if state.Phase != duelPhaseSetup {
			respondErr(w, http.StatusConflict, "champions can only be elected during setup")
			return
		}
		alreadyDeclared := state.PreparerChampionDeclared
		if player.ID != plan.PreparerID {
			alreadyDeclared = state.TargetChampionDeclared
		}
		if alreadyDeclared {
			respondErr(w, http.StatusConflict, "you have already declared your champion choice")
			return
		}
		// Initiative-holder declares first.
		if state.InitiativePlayerID != nil && *state.InitiativePlayerID != player.ID {
			initHas := state.PreparerChampionDeclared
			if *state.InitiativePlayerID != plan.PreparerID {
				initHas = state.TargetChampionDeclared
			}
			if !initHas {
				respondErr(w, http.StatusConflict,
					"the player with initiative must declare their champion choice first")
				return
			}
		}

		if body.AssetID != nil {
			asset, err := deps.Q.GetAssetByID(ctx, *body.AssetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, "asset not found")
				return
			}
			if asset.GameID != plan.GameID || asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you do not own this asset")
				return
			}
			if asset.AssetType != model.AssetPeer {
				respondErr(w, http.StatusBadRequest, "champion must be a peer asset")
				return
			}
		}

		if player.ID == plan.PreparerID {
			state.PreparerChampionID = body.AssetID
			state.PreparerChampionDeclared = true
		} else {
			state.TargetChampionID = body.AssetID
			state.TargetChampionDeclared = true
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save champion", err)
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			var aid int64
			if body.AssetID != nil {
				aid = *body.AssetID
			}
			h.BroadcastEvent(model.EventDuelChampionElected, model.DuelChampionElectedPayload{
				PlanID: plan.ID, PlayerID: player.ID, AssetID: aid,
			})
		}

		who := playerDisplayName(ctx, deps.Q, player.ID)
		if body.AssetID != nil {
			pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s sends %s into the ring to fight as their champion.",
				who, assetDisplayName(ctx, deps.Q, *body.AssetID)))
		} else {
			pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s steps onto the duelling ground to answer for themselves.", who))
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "player_id": player.ID, "asset_id": body.AssetID,
		})
	}
}

// ── Stake Reveal ──────────────────────────────────────────────────────────────

// pduelStakeRevealHandler: POST /api/plans/:planId/stake-reveal
// Body: {"count": N}. Min 1; max 1+esteem status.
// Counts are held until both players submit, then revealed.
func pduelStakeRevealHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may reveal stakes")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseSetup {
			respondErr(w, http.StatusConflict, "stake reveal is only allowed in 'setup' phase")
			return
		}

		var body struct {
			Count int16 `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Count < 1 {
			respondErr(w, http.StatusBadRequest, "count must be ≥ 1")
			return
		}

		ctx := r.Context()
		rank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, player.ID, model.CategoryEsteem)
		if err != nil {
			respondInternalErr(w, r, "could not load esteem rank", err)
			return
		}
		if body.Count > gamepkg.MaxStakes(rank) {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("count %d exceeds maximum %d for your esteem status",
					body.Count, gamepkg.MaxStakes(rank)))
			return
		}

		// Accumulate per-player stake counts until both have submitted.
		if state.StakeCounts == nil {
			state.StakeCounts = map[int64]int16{}
		}
		state.StakeCounts[player.ID] = body.Count

		if len(state.StakeCounts) >= 2 {
			// Both submitted — reveal and advance to staking.
			state.PreparerStakeCount = state.StakeCounts[plan.PreparerID]
			if plan.TargetPlayerID != nil {
				state.TargetStakeCount = state.StakeCounts[*plan.TargetPlayerID]
			}
			state.Phase = duelPhaseStaking

			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save stake counts", err)
				return
			}

			broadcastEvent(deps.Manager, plan.GameID, model.EventDuelStakesRevealed, model.DuelStakesRevealedPayload{
				PlanID:             plan.ID,
				PreparerStakeCount: state.PreparerStakeCount,
				TargetStakeCount:   state.TargetStakeCount,
			})
			prepName := playerDisplayName(ctx, deps.Q, plan.PreparerID)
			targName := prepName
			if plan.TargetPlayerID != nil {
				targName = playerDisplayName(ctx, deps.Q, *plan.TargetPlayerID)
			}
			pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"The terms are set: %s wagers %d, %s wagers %d. Now they choose what to stake.",
				prepName, state.PreparerStakeCount, targName, state.TargetStakeCount))
		} else {
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save stake reveal", err)
				return
			}
		}

		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "submitted": len(state.StakeCounts)})
	}
}

// ── Select Stakes ─────────────────────────────────────────────────────────────

// pduelSelectStakesHandler: POST /api/plans/:planId/select-stakes
// Body: {"asset_ids": [N, ...]}
// The count must match the player's revealed stake count. Server rolls a
// hidden d6 for each asset and stores it in duel_staked_assets. The hidden
// die is visible only to the asset owner; the opponent sees only that a
// stake has been placed.
//
//nolint:gocognit,funlen // stake-selection lifecycle including target-claim path
func pduelSelectStakesHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may stake assets")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseStaking {
			respondErr(w, http.StatusConflict, "select-stakes is only allowed in 'staking' phase")
			return
		}

		var expected int16
		if player.ID == plan.PreparerID {
			expected = state.PreparerStakeCount
		} else {
			expected = state.TargetStakeCount
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if int16(len(body.AssetIDs)) != expected {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("expected %d asset_ids to match your stake count", expected))
			return
		}

		ctx := r.Context()

		// Check whether this player has already staked.
		existing, err := deps.Q.ListDuelStakesByPlanPlayer(ctx, dbgen.ListDuelStakesByPlanPlayerParams{
			PlanID: plan.ID, PlayerID: player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not load existing stakes", err)
			return
		}
		if len(existing) > 0 {
			respondErr(w, http.StatusConflict, "you have already selected your stakes")
			return
		}

		// Validate each asset: owned, non-destroyed, not already leveraged.
		for _, aid := range body.AssetIDs {
			asset, errAsset := deps.Q.GetAssetByID(ctx, aid)
			if errAsset != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", aid))
				return
			}
			if asset.GameID != plan.GameID || asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, fmt.Sprintf("you do not own asset %d", aid))
				return
			}
			if asset.IsDestroyed {
				respondErr(w, http.StatusBadRequest, fmt.Sprintf("asset %d is destroyed", aid))
				return
			}
			if asset.IsLeveraged {
				respondErr(w, http.StatusBadRequest, fmt.Sprintf("asset %d is already leveraged", aid))
				return
			}
		}

		// Create stakes with a hidden d6 per asset. Collect them so the caller
		// can see their own hidden dice in the response without polling.
		createdStakes := make([]dbgen.DuelStakedAsset, 0, len(body.AssetIDs))
		for _, aid := range body.AssetIDs {
			face := int16(rand.IntN(gamepkg.DiceSides) + 1)
			stake, errStake := deps.Q.CreateDuelStake(ctx, dbgen.CreateDuelStakeParams{
				PlanID:    plan.ID,
				PlayerID:  player.ID,
				AssetID:   aid,
				HiddenDie: face,
			})
			if errStake != nil {
				respondInternalErr(w, r, "could not create stake", errStake)
				return
			}
			createdStakes = append(createdStakes, stake)
		}

		stakeNames := make([]string, 0, len(body.AssetIDs))
		for _, aid := range body.AssetIDs {
			stakeNames = append(stakeNames, assetDisplayName(ctx, deps.Q, aid))
		}
		pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s lays their stakes on the line: %s — each guarding a hidden die.",
			playerDisplayName(ctx, deps.Q, player.ID), strings.Join(stakeNames, ", ")))

		// If both players have staked, advance to bouts.
		allStakes, err := deps.Q.ListDuelStakesByPlan(ctx, plan.ID)
		if err != nil {
			respondInternalErr(w, r, "could not load stakes", err)
			return
		}
		total := int16(len(allStakes))
		if total == state.PreparerStakeCount+state.TargetStakeCount {
			state.Phase = duelPhaseBouts
			state.CurrentBout = 0
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not advance phase", err)
				return
			}
			// Mirror the stake-reveal broadcast: the waiting duellist needs a
			// duel event to refetch the plan and pick up phase=bouts. Without
			// it, broadcastRowState alone leaves them soft-locked on the
			// staking panel even though it's their turn to declare.
			broadcastEvent(deps.Manager, plan.GameID, model.EventDuelStakesSelected, model.DuelStakesSelectedPayload{
				PlanID: plan.ID,
			})
		}

		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "staked": len(body.AssetIDs), "stakes": createdStakes,
		})
	}
}
