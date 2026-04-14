package handler

// handler/plan_exchange_courtiers.go — Exchange Courtiers plan handler.
//
// Exchange Courtiers (power, delay 5): The preparer attempts to take a peer
// from the target player. Resolution starts with a fair-trade offer step; if
// declined, a dice roll is created and the normal roll flow proceeds.
//
// Make options: "legal" (standard transfer) or "messy" (transfer + target must
// break a marginalia on one of their assets).
// Mar options: "riposte" (preparer gives their own peer to target) or "forfeit"
// (preparer gives target player an asset of their choice).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanExchangeCourtiers, ecHandler{})
}

type ecHandler struct{}

func (ecHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryPower, Delay: 5}
}

// exchangeCourtiersDifficultyPure returns the difficulty given the target
// player's rank on the power track.
// Difficulty = target's status = 6 - rank (minimum 1).
func exchangeCourtiersDifficultyPure(targetRank int16) int16 {
	return max(int16(diceSides)-targetRank, 1)
}

func (ecHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (int16, string) {
	errMsg := validateExchangeCourtiersPlan(ctx, v.Q, v.Game.ID, v.TargetPlayerID, v.TargetAssetID)
	return 0, errMsg // fixed delay; target row computed from Metadata().Delay
}

func (ecHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	if plan.TargetPlayerID == nil {
		return 0, errors.New("exchange courtiers plan has no target player")
	}
	targetRank, err := playerRankInCategory(ctx, q, plan.GameID, *plan.TargetPlayerID, model.CategoryPower)
	if err != nil {
		return 0, fmt.Errorf("could not determine target player ranking: %w", err)
	}
	return exchangeCourtiersDifficultyPure(targetRank), nil
}

// OnResolve returns nil: Exchange Courtiers starts with the fair-trade step,
// not an immediate dice roll. The fair-trade endpoint (an ExtraRoute) handles
// the optional dice roll after the preparer declines.
func (ecHandler) OnResolve(_ context.Context, _ *PlanDeps, _ *dbgen.Plan) (*dbgen.DiceRoll, error) {
	return nil, nil
}

func (ecHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	if result != makeOutcome {
		return nil // mar effects are narrative / handled via asset endpoints
	}
	return applyExchangeCourtiersMechanic(ctx, deps.Q, deps.Manager, plan, choices, resData)
}

func (ecHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	if resData.MessyBreakRequired && !resData.MessyBreakDone {
		return errors.New("target player must first break a marginalia (POST /plans/{planId}/messy-break)")
	}
	return nil
}

func (ecHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"fair-trade":  fairTradeHandler(deps.Q, deps.Manager),
		"messy-break": messyBreakHandler(deps.Q, deps.Manager),
	}
}

// ── applyExchangeCourtiersMechanic ────────────────────────────────────────────

// applyExchangeCourtiersMechanic transfers the target asset to the preparer and
// sets the MessyBreakRequired flag if the "messy" option was chosen.
func applyExchangeCourtiersMechanic(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	plan *dbgen.Plan,
	choices []string,
	resData *ResolutionData,
) error {
	if plan.TargetAssetID == nil || plan.TargetPlayerID == nil {
		return nil // nothing to do
	}

	// Only transfer if not already done via fair trade accept.
	if resData.FairTradeAccepted == nil || !*resData.FairTradeAccepted {
		if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID:      *plan.TargetAssetID,
			OwnerID: plan.PreparerID,
		}); err != nil {
			return err
		}
		ta, _ := q.GetAssetByID(ctx, *plan.TargetAssetID)
		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
				Asset:      ta,
				OldOwnerID: *plan.TargetPlayerID,
				NewOwnerID: plan.PreparerID,
			})
		}
	}

	// "Messy" requires the target to break a marginalia on any of their assets.
	if slices.Contains(choices, "messy") {
		resData.MessyBreakRequired = true
	}

	return nil
}

// ── FairTrade extra route ─────────────────────────────────────────────────────

// fairTradeHandler handles POST /api/plans/:planId/fair-trade.
//
// Exchange Courtiers only. Three sub-actions via the body:
//
//	{"action": "offer",   "offered_asset_id": 123} — target player offers a peer
//	{"action": "accept"}                           — preparer accepts; assets exchanged
//	{"action": "decline"}                          — preparer declines; dice roll created
func fairTradeHandler(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanExchangeCourtiers {
			respondErr(w, http.StatusBadRequest, "fair trade is only for Exchange Courtiers")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			Action         string `json:"action"`
			OfferedAssetID *int64 `json:"offered_asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		game, err := q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)

		switch body.Action {
		case "offer":
			offerFairTrade(ctx, w, q, &resData, plan, player, body.OfferedAssetID)
		case "accept":
			acceptFairTrade(ctx, w, q, &resData, plan, player, manager, game)
		case "decline":
			declineFairTrade(ctx, w, q, &resData, plan, player, manager, game)
		default:
			respondErr(w, http.StatusBadRequest, "action must be 'offer', 'accept', or 'decline'")
		}
	}
}

func offerFairTrade(
	ctx context.Context, w http.ResponseWriter, q *dbgen.Queries,
	resData *ResolutionData, plan *dbgen.Plan, player *dbgen.Player,
	offeredAssetID *int64,
) {
	if plan.TargetPlayerID == nil || player.ID != *plan.TargetPlayerID {
		respondErr(w, http.StatusForbidden, "only the target player can offer a fair trade")
		return
	}
	if offeredAssetID == nil {
		respondErr(w, http.StatusBadRequest, "offered_asset_id is required")
		return
	}
	asset, err := q.GetAssetByID(ctx, *offeredAssetID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "offered asset not found")
		return
	}
	if asset.OwnerID != player.ID {
		respondErr(w, http.StatusForbidden, "you can only offer your own assets")
		return
	}
	if asset.AssetType != model.AssetPeer {
		respondErr(w, http.StatusBadRequest, "fair trade offer must be a peer asset")
		return
	}
	resData.FairTradeAssetID = offeredAssetID
	if err := saveResolutionData(ctx, q, plan.ID, *resData); err != nil {
		respondErr(w, http.StatusInternalServerError, "could not save offer")
		return
	}
	respond(w, http.StatusOK, map[string]any{
		"plan_id":          plan.ID,
		"offered_asset_id": offeredAssetID,
	})
}

func acceptFairTrade(
	ctx context.Context, w http.ResponseWriter, q *dbgen.Queries,
	resData *ResolutionData, plan *dbgen.Plan, player *dbgen.Player,
	manager *hub.Manager, game dbgen.Game,
) {
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the preparer can accept or decline")
		return
	}
	if resData.FairTradeAssetID == nil {
		respondErr(w, http.StatusConflict, "no fair trade offer has been made yet")
		return
	}
	if plan.TargetAssetID == nil || plan.TargetPlayerID == nil {
		respondErr(w, http.StatusConflict, "plan is missing target asset or player")
		return
	}

	// Transfer targeted asset to preparer.
	if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      *plan.TargetAssetID,
		OwnerID: plan.PreparerID,
	}); err != nil {
		respondErr(w, http.StatusInternalServerError, "could not transfer targeted asset")
		return
	}
	// Transfer offered asset to target player.
	if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      *resData.FairTradeAssetID,
		OwnerID: *plan.TargetPlayerID,
	}); err != nil {
		respondErr(w, http.StatusInternalServerError, "could not transfer offered asset")
		return
	}

	accepted := true
	resData.FairTradeAccepted = &accepted
	resData.Choices = []string{"fair_trade_accepted"}
	if err := saveResolutionData(ctx, q, plan.ID, *resData); err != nil {
		respondErr(w, http.StatusInternalServerError, "could not save decision")
		return
	}

	if err := q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
		ID:     plan.ID,
		Result: new(makeOutcome),
	}); err != nil {
		respondErr(w, http.StatusInternalServerError, "could not resolve plan")
		return
	}

	h, hasHub := manager.Get(game.ID)
	if hasHub {
		ta, _ := q.GetAssetByID(ctx, *plan.TargetAssetID)
		h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
			Asset:      ta,
			OldOwnerID: *plan.TargetPlayerID,
			NewOwnerID: plan.PreparerID,
		})
		oa, _ := q.GetAssetByID(ctx, *resData.FairTradeAssetID)
		h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
			Asset:      oa,
			OldOwnerID: plan.PreparerID,
			NewOwnerID: *plan.TargetPlayerID,
		})
		h.BroadcastEvent(model.EventPlanResolved, model.PlanResolvedPayload{
			PlanID: plan.ID,
			Result: makeOutcome,
		})
	}

	respond(w, http.StatusOK, map[string]any{
		"plan_id": plan.ID,
		"result":  "make",
		"note":    "fair trade accepted; assets exchanged",
	})
}

func declineFairTrade(
	ctx context.Context, w http.ResponseWriter, q *dbgen.Queries,
	resData *ResolutionData, plan *dbgen.Plan, player *dbgen.Player,
	manager *hub.Manager, game dbgen.Game,
) {
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the preparer can accept or decline")
		return
	}
	declined := false
	resData.FairTradeAccepted = &declined
	if err := saveResolutionData(ctx, q, plan.ID, *resData); err != nil {
		respondErr(w, http.StatusInternalServerError, "could not save decision")
		return
	}

	h := ecHandler{}
	difficulty, err := h.ComputeDifficulty(ctx, q, plan, resData)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not compute difficulty")
		return
	}
	roll, err := createPlanRoll(ctx, q, manager, &game, plan, difficulty, player.ID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not create dice roll")
		return
	}
	respond(w, http.StatusOK, map[string]any{
		"plan_id": plan.ID,
		"roll":    roll,
	})
}

// ── MessyBreak extra route ────────────────────────────────────────────────────

// messyBreakHandler handles POST /api/plans/:planId/messy-break.
//
// Exchange Courtiers only. After a make result with the "messy" option, the
// target player must tear one marginalia from any asset in the game.
//
// Request body: {"marginalia_id": 123}
func messyBreakHandler(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanExchangeCourtiers {
			respondErr(w, http.StatusBadRequest, "messy break is only for Exchange Courtiers")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if plan.TargetPlayerID == nil || player.ID != *plan.TargetPlayerID {
			respondErr(w, http.StatusForbidden, "only the target player can perform the messy break")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		if !resData.MessyBreakRequired {
			respondErr(w, http.StatusConflict, "no messy break is required for this plan")
			return
		}
		if resData.MessyBreakDone {
			respondErr(w, http.StatusConflict, "messy break has already been completed")
			return
		}

		var body struct {
			MarginaliaID int64 `json:"marginalia_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MarginaliaID == 0 {
			respondErr(w, http.StatusBadRequest, "marginalia_id is required")
			return
		}

		ctx := r.Context()

		m, err := q.GetMarginaliaByID(ctx, body.MarginaliaID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "marginalia not found")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		asset, err := q.GetAssetByID(ctx, m.AssetID)
		if err != nil || asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "marginalia does not belong to this game")
			return
		}

		if err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
			ID:       m.ID,
			TornByID: &player.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not tear marginalia")
			return
		}

		resData.MessyBreakDone = true
		if err := saveResolutionData(ctx, q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record messy break")
			return
		}

		if h, ok := manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
				AssetID:  asset.ID,
				Position: m.Position,
				TornByID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
			"asset_id":      asset.ID,
		})
	}
}
