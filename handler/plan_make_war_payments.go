package handler

// handler/plan_make_war_payments.go — Cost-of-battle payment routes for Make
// War: /pay-battle-cost, /pay-war-entry, /take-surrender-asset, plus the
// surrender handling those routes trigger. Outstanding-cost queries live in
// plan_make_war_costs.go; peace negotiation lives in plan_make_war_peace.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── pay-battle-cost ──────────────────────────────────────────────────────────
//
// Cost of battle is paid in reverse power order, one (payer, opponent) pair
// at a time. The caller's turn is determined by asking MissingBattleCosts
// for the first outstanding pair — if its PayerID matches the caller, they
// are up next.

// mwPayBattleCostHandler handles POST /api/plans/:planId/pay-battle-cost.
//
// Body:
//
//	{
//	  "opponent_id":    int64,
//	  "choice":         "break_asset" | "leverage_two",
//	  "marginalia_id":  int64,   // break_asset only — tear one marginalia
//	  "asset_id_1":     int64,   // leverage_two only
//	  "asset_id_2":     int64    // leverage_two only
//	}
//
// Setting `surrender: true` marks the payer surrendered after the chosen
// payment is applied; each opposing non-surrendered opponent then has one
// open surrender claim, redeemable via /take-surrender-asset.
//
//nolint:funlen,gocognit // orchestrates many sequential war-cost validation branches
func mwPayBattleCostHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		var body struct {
			OpponentID   int64  `json:"opponent_id"`
			Choice       string `json:"choice"`
			MarginaliaID int64  `json:"marginalia_id"`
			AssetID1     int64  `json:"asset_id_1"`
			AssetID2     int64  `json:"asset_id_2"`
			Surrender    bool   `json:"surrender"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidBattleCostChoice(body.Choice) {
			respondErr(w, http.StatusBadRequest, "choice must be break_asset or leverage_two")
			return
		}

		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		if war.Status != warStatusActive {
			respondErr(w, http.StatusConflict, "war is no longer active")
			return
		}
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		snap, err := mwSnapshotWar(ctx, deps.Q, war)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load war participants")
			return
		}
		ranks, err := mwPowerRanks(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}
		missing, err := mwOutstandingCostsForWar(ctx, deps.Q, snap, ranks, game.CurrentRow)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not compute outstanding costs")
			return
		}
		if len(missing) == 0 {
			respondErr(w, http.StatusConflict, "no battle costs are outstanding this row")
			return
		}
		if missing[0].PayerID != player.ID {
			respondErr(w, http.StatusConflict,
				"another participant must pay their battle cost first (reverse power order)")
			return
		}
		owesOpponent := false
		for _, k := range missing {
			if k.PayerID == player.ID && k.OpponentID == body.OpponentID {
				owesOpponent = true
				break
			}
		}
		if !owesOpponent {
			respondErr(w, http.StatusConflict, "you do not owe a cost to that opponent this row")
			return
		}

		assetID1, assetID2, ok := mwApplyCostChoice(
			ctx, deps, plan.GameID, player,
			body.Choice, body.MarginaliaID, body.AssetID1, body.AssetID2, w,
		)
		if !ok {
			return
		}

		if _, err := deps.Q.CreateBattleCost(ctx, dbgen.CreateBattleCostParams{
			WarID:       war.ID,
			RowNumber:   game.CurrentRow,
			PayerID:     player.ID,
			OpponentID:  body.OpponentID,
			Choice:      body.Choice,
			AssetID1:    assetID1,
			AssetID2:    assetID2,
			Surrendered: body.Surrender,
			IsEntry:     false,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record battle cost")
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventWarBattleCostPaid, model.WarBattleCostPaidPayload{
			WarID: war.ID, RowNumber: game.CurrentRow,
			PayerID: player.ID, OpponentID: body.OpponentID,
			Choice: body.Choice, Surrendered: body.Surrender,
		})

		if body.Surrender {
			if err := mwApplySurrender(ctx, deps, war, snap, player.ID, game.CurrentRow); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not apply surrender")
				return
			}
			respond(w, http.StatusOK, map[string]any{
				"war_id":      war.ID,
				"row_number":  game.CurrentRow,
				"opponent_id": body.OpponentID,
				"choice":      body.Choice,
				"surrendered": true,
			})
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"war_id":      war.ID,
			"row_number":  game.CurrentRow,
			"opponent_id": body.OpponentID,
			"choice":      body.Choice,
		})
	}
}

// mwApplySurrender marks payer surrendered, opens a surrender claim for each
// opposing non-surrendered full participant, broadcasts events, and ends the
// war if a side is now empty.
func mwApplySurrender(
	ctx context.Context,
	deps *PlanDeps,
	war dbgen.War,
	snap mwWarSnapshot,
	payerID int64,
	row int16,
) error {
	if err := deps.Q.SetWarParticipantSurrendered(ctx, dbgen.SetWarParticipantSurrenderedParams{
		WarID: war.ID, PlayerID: payerID, SurrenderedAtRow: &row,
	}); err != nil {
		return err
	}
	for _, opp := range gamepkg.ActiveOpponents(payerID, snap.Sides, snap.Surrendered) {
		if opp == payerID {
			continue
		}
		if err := deps.Q.CreateSurrenderClaim(ctx, dbgen.CreateSurrenderClaimParams{
			WarID: war.ID, SurrenderedID: payerID, ClaimantID: opp,
		}); err != nil {
			return err
		}
	}

	h, hasHub := deps.Manager.Get(war.GameID)
	if hasHub {
		h.BroadcastEvent(model.EventWarPlayerSurrender, model.WarPlayerSurrenderPayload{
			WarID: war.ID, PlayerID: payerID, RowNumber: row,
		})
	}

	ended, reason := gamepkg.SurrenderOutcome(snap.Sides, snap.Surrendered, payerID)
	if !ended {
		return nil
	}
	if err := deps.Q.EndWar(ctx, dbgen.EndWarParams{
		ID: war.ID, EndReason: new(reason), EndedAtRow: &row,
	}); err != nil {
		return err
	}
	if hasHub {
		h.BroadcastEvent(model.EventWarEnded, model.WarEndedPayload{
			WarID: war.ID, Reason: reason, RowNumber: row,
		})
	}
	return nil
}

// mwApplyCostChoice validates and applies one break_asset or leverage_two
// payment against the caller's assets. Shared by /pay-battle-cost and
// /pay-war-entry. Returns (assetID1, assetID2) for battle_cost record.
func mwApplyCostChoice(
	ctx context.Context,
	deps *PlanDeps,
	gameID int64,
	player *dbgen.Player,
	choice string,
	marginaliaID, assetID1In, assetID2In int64,
	w http.ResponseWriter,
) (a1, a2 *int64, ok bool) {
	switch choice {
	case gamepkg.WarCostBreakAsset:
		return mwApplyBreakAsset(ctx, deps, gameID, player, marginaliaID, w)
	case gamepkg.WarCostLeverageTwo:
		return mwApplyLeverageTwo(ctx, deps, player, assetID1In, assetID2In, w)
	}
	respondErr(w, http.StatusBadRequest, "choice must be break_asset or leverage_two")
	return nil, nil, false
}

func mwApplyBreakAsset(
	ctx context.Context,
	deps *PlanDeps,
	gameID int64,
	player *dbgen.Player,
	marginaliaID int64,
	w http.ResponseWriter,
) (a1, a2 *int64, ok bool) {
	m, err := deps.Q.GetMarginaliaByID(ctx, marginaliaID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "marginalia not found")
		return nil, nil, false
	}
	asset, err := deps.Q.GetAssetByID(ctx, m.AssetID)
	if err != nil || asset.OwnerID != player.ID {
		respondErr(w, http.StatusForbidden, "marginalia must belong to an asset you own")
		return nil, nil, false
	}
	if asset.IsDestroyed {
		respondErr(w, http.StatusConflict, "asset is already destroyed")
		return nil, nil, false
	}
	if m.IsTorn {
		respondErr(w, http.StatusConflict, "marginalia is already torn")
		return nil, nil, false
	}
	_, err = deps.Q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID: m.ID, TornByID: &player.ID,
	})
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not tear marginalia")
		return nil, nil, false
	}
	broadcastEvent(deps.Manager, gameID, model.EventMarginaliaTorn, model.MarginaliaTornPayload{
		AssetID: asset.ID, Position: m.Position, TornByID: player.ID,
	})
	intact, _ := deps.Q.CountIntactMarginalia(ctx, asset.ID)
	if intact == 0 {
		total, _ := deps.Q.CountMarginalia(ctx, asset.ID)
		if total > 0 {
			_ = deps.Q.DestroyAsset(ctx, asset.ID)
			broadcastEvent(deps.Manager, gameID, model.EventAssetDestroyed, model.AssetIDPayload{AssetID: asset.ID})
		}
	}
	return &asset.ID, nil, true
}

func mwApplyLeverageTwo(
	ctx context.Context,
	deps *PlanDeps,
	player *dbgen.Player,
	assetID1In, assetID2In int64,
	w http.ResponseWriter,
) (a1, a2 *int64, ok bool) {
	if assetID1In == 0 || assetID2In == 0 || assetID1In == assetID2In {
		respondErr(w, http.StatusBadRequest, "must specify two distinct assets to leverage")
		return nil, nil, false
	}
	for _, id := range []int64{assetID1In, assetID2In} {
		a, err := deps.Q.GetAssetByID(ctx, id)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return nil, nil, false
		}
		if a.OwnerID != player.ID {
			respondErr(w, http.StatusForbidden, "you can only leverage your own assets")
			return nil, nil, false
		}
		if a.IsDestroyed {
			respondErr(w, http.StatusConflict, "asset is destroyed")
			return nil, nil, false
		}
		if a.IsLeveraged {
			respondErr(w, http.StatusConflict, "asset is already leveraged")
			return nil, nil, false
		}
	}
	for _, id := range []int64{assetID1In, assetID2In} {
		err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID: id, IsLeveraged: true,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not leverage asset")
			return nil, nil, false
		}
	}
	return &assetID1In, &assetID2In, true
}

// ── pay-war-entry ────────────────────────────────────────────────────────────
//
// Late joiners (entry_payment_complete=FALSE) pay a full cost of battle
// against each existing opposing opponent before becoming full participants.
// Body mirrors pay-battle-cost (minus surrender).
//
//nolint:funlen,gocognit // orchestrates late-joiner entry payment flow
func mwPayWarEntryHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		var body struct {
			OpponentID   int64  `json:"opponent_id"`
			Choice       string `json:"choice"`
			MarginaliaID int64  `json:"marginalia_id"`
			AssetID1     int64  `json:"asset_id_1"`
			AssetID2     int64  `json:"asset_id_2"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidBattleCostChoice(body.Choice) {
			respondErr(w, http.StatusBadRequest, "choice must be break_asset or leverage_two")
			return
		}

		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		if war.Status != warStatusActive {
			respondErr(w, http.StatusConflict, "war is no longer active")
			return
		}
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}

		part, err := deps.Q.GetWarParticipant(ctx, dbgen.GetWarParticipantParams{
			WarID: war.ID, PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusForbidden, "you are not a participant in this war")
			return
		}
		if part.EntryPaymentComplete {
			respondErr(w, http.StatusConflict, "you have already paid your war entry")
			return
		}

		snap, err := mwSnapshotWar(ctx, deps.Q, war)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load war participants")
			return
		}
		targets := gamepkg.ActiveOpponents(
			player.ID,
			gamepkg.MergeSides(snap.Sides, map[int64]int16{player.ID: part.Side}),
			snap.Surrendered,
		)
		validTarget := slices.Contains(targets, body.OpponentID)
		if !validTarget {
			respondErr(w, http.StatusConflict, "that opponent is not an active opposing participant")
			return
		}
		existing, err := deps.Q.ListBattleCostsByPayerForRow(ctx, dbgen.ListBattleCostsByPayerForRowParams{
			WarID: war.ID, RowNumber: game.CurrentRow, PayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load existing payments")
			return
		}
		for _, bc := range existing {
			if bc.IsEntry && bc.OpponentID == body.OpponentID {
				respondErr(w, http.StatusConflict, "you have already paid entry against that opponent")
				return
			}
		}

		a1, a2, ok := mwApplyCostChoice(ctx, deps, plan.GameID, player,
			body.Choice, body.MarginaliaID, body.AssetID1, body.AssetID2, w)
		if !ok {
			return
		}

		if _, err := deps.Q.CreateBattleCost(ctx, dbgen.CreateBattleCostParams{
			WarID:       war.ID,
			RowNumber:   game.CurrentRow,
			PayerID:     player.ID,
			OpponentID:  body.OpponentID,
			Choice:      body.Choice,
			AssetID1:    a1,
			AssetID2:    a2,
			Surrendered: false,
			IsEntry:     true,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record entry payment")
			return
		}

		remaining := 0
		paidSet := map[int64]bool{body.OpponentID: true}
		for _, bc := range existing {
			if bc.IsEntry {
				paidSet[bc.OpponentID] = true
			}
		}
		for _, t := range targets {
			if !paidSet[t] {
				remaining++
			}
		}
		complete := remaining == 0
		if complete {
			if err := deps.Q.SetWarParticipantEntryComplete(ctx, dbgen.SetWarParticipantEntryCompleteParams{
				WarID: war.ID, PlayerID: player.ID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not mark entry complete")
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, model.EventWarEntryCompleted, model.WarEntryCompletedPayload{
				WarID: war.ID, PlayerID: player.ID, Side: part.Side,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"war_id":         war.ID,
			"opponent_id":    body.OpponentID,
			"entry_complete": complete,
			"remaining":      remaining,
		})
	}
}

// ── take-surrender-asset ─────────────────────────────────────────────────────
//
// After a player surrenders, each opposing non-surrendered full participant
// holds an unfulfilled claim to take one of the surrendered player's assets.
// Body: {"surrendered_id": int64, "asset_id": int64}
func mwTakeSurrenderAssetHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		var body struct {
			SurrenderedID int64 `json:"surrendered_id"`
			AssetID       int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		claim, err := deps.Q.GetSurrenderClaim(ctx, dbgen.GetSurrenderClaimParams{
			WarID: war.ID, SurrenderedID: body.SurrenderedID, ClaimantID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusNotFound, "no open surrender claim for you against that player")
			return
		}
		if claim.FulfilledAt.Valid {
			respondErr(w, http.StatusConflict, "claim already fulfilled")
			return
		}

		asset, err := deps.Q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.OwnerID != body.SurrenderedID {
			respondErr(w, http.StatusForbidden, "asset does not belong to the surrendered player")
			return
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "asset is destroyed")
			return
		}

		if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID: asset.ID, OwnerID: player.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not transfer asset")
			return
		}
		if err := deps.Q.FulfillSurrenderClaim(ctx, dbgen.FulfillSurrenderClaimParams{
			ID: claim.ID, AssetID: &asset.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not fulfill claim")
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventWarAssetSeized, model.WarAssetSeizedPayload{
			WarID: war.ID, SurrenderedID: body.SurrenderedID,
			ClaimantID: player.ID, AssetID: asset.ID,
		})
		respond(w, http.StatusOK, map[string]any{
			"war_id":   war.ID,
			"asset_id": asset.ID,
		})
	}
}
