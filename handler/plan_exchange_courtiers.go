package handler

// handler/plan_exchange_courtiers.go — Exchange Courtiers plan handler.
//
// Exchange Courtiers (power, delay 5): The preparer attempts to take a peer
// from the target player. Resolution starts with a fair-trade offer step; if
// declined, a dice roll is created and the normal roll flow proceeds.
//
// Make options (preparer): "legal" (standard transfer), "messy" (transfer +
// target breaks one of the preparer's assets), "conspiracy" (narrative).
// Mar options (target-driven): "fair_trade" (the trade goes through anyway),
// "riposte"/"forfeit" (the target claims one of the preparer's peers; riposte
// also lets the preparer break it first).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
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

// PreparedDescriptor names the targeted peer in the plan.prepared log — the
// generic line drops which courtier the preparer is angling for.
func (ecHandler) PreparedDescriptor(
	ctx context.Context,
	q *dbgen.Queries,
	plan dbgen.Plan,
	_ *ResolutionData,
) (string, bool) {
	if plan.TargetAssetID == nil {
		return "", false
	}
	return fmt.Sprintf("prepared Exchange Courtiers, angling for the peer %s%s",
		assetDisplayName(ctx, q, *plan.TargetAssetID), notesSuffix(plan)), true
}

func (ecHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (*int16, string) {
	errMsg := validateExchangeCourtiersPlan(ctx, v.Q, v.Game.ID, v.TargetPlayerID, v.TargetAssetID)
	return nil, errMsg // fixed delay; target row computed from Metadata().Delay
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
	return gamepkg.ExchangeCourtiersDifficulty(targetRank), nil
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
	if result == makeOutcome {
		return applyExchangeCourtiersMake(ctx, deps, plan, choices, resData)
	}
	return applyExchangeCourtiersMar(ctx, deps, plan, choices, resData)
}

func (ecHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	ec := resData.ExchangeCourtiers
	if ec == nil {
		return nil
	}
	if ec.MessyBreakRequired && !ec.MessyBreakDone {
		return errors.New("target player must first break a marginalia (POST /plans/{planId}/messy-break)")
	}
	if ec.PeerClaimsDone < ec.PeerClaimsRequired {
		return fmt.Errorf("target must still claim %d peer(s) (POST /plans/{planId}/claim-peer)",
			ec.PeerClaimsRequired-ec.PeerClaimsDone)
	}
	return nil
}

func (ecHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"fair-trade":    fairTradeHandler(deps.Q, deps.Manager),
		"messy-break":   messyBreakHandler(deps.Q, deps.Manager),
		"claim-peer":    ecClaimPeerHandler(deps),
		"riposte-break": ecRiposteBreakHandler(deps),
	}
}

// MaxChoices: up to (result − difficulty) make options; up to (difficulty −
// result) mar options.
func (ecHandler) MaxChoices(result string, rollResult, difficulty int16) int {
	if result == makeOutcome {
		return int(rollResult - difficulty)
	}
	return int(difficulty - rollResult)
}

// ecLog emits an Exchange Courtiers action-log entry anchored to the plan row.
func ecLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.exchange_courtiers",
		severity, body, plan.RowNumber, &plan.ID, nil,
		map[string]any{"plan_id": plan.ID})
}

// ── Make ──────────────────────────────────────────────────────────────────────

// applyExchangeCourtiersMake transfers the targeted peer to the preparer and
// applies the chosen make options:
//   - "messy"      → the target must break one of the preparer's assets later.
//   - "legal"      → everything went to plan (log only).
//   - "conspiracy" → the peer was in on it (narrative; log only).
func applyExchangeCourtiersMake(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choices []string,
	resData *ResolutionData,
) error {
	if plan.TargetAssetID == nil || plan.TargetPlayerID == nil {
		return nil // nothing to do
	}

	// Only transfer if not already done via fair trade accept.
	ec := resData.ExchangeCourtiers
	if ec == nil || ec.FairTradeAccepted == nil || !*ec.FairTradeAccepted {
		// Incoming side may be redirected by a resolved Make Demands
		// (keep_assets). Preparer's outgoing asset (handled via messy/
		// fair trade paths) is not redirected.
		recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
		if err != nil {
			return err
		}
		if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID:      *plan.TargetAssetID,
			OwnerID: recipient,
		}); err != nil {
			return err
		}
		ta, _ := deps.Q.GetAssetByID(ctx, *plan.TargetAssetID)
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
			Asset:      ta,
			OldOwnerID: *plan.TargetPlayerID,
			NewOwnerID: recipient,
		})
		ecLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("%s took the peer %q from %s.",
				playerDisplayName(ctx, deps.Q, recipient), ta.Name,
				playerDisplayName(ctx, deps.Q, *plan.TargetPlayerID)))
	}

	// "Messy" requires the target to break a marginalia on one of the preparer's
	// assets before completion; otherwise the made resolution is done.
	ec = resData.EnsureExchangeCourtiers()
	if slices.Contains(choices, "messy") {
		ec.MessyBreakRequired = true
		ec.Phase = gamepkg.ECPhaseMessyBreak
		ecLog(ctx, deps, plan, model.SeverityDefault,
			"It got messy — the target may break one of the preparer's assets.")
	} else {
		ec.Phase = gamepkg.ECPhaseDone
	}
	if slices.Contains(choices, "conspiracy") {
		ecLog(ctx, deps, plan, model.SeverityDefault,
			"The peer was in on it all along.")
	}

	return nil
}

// ── Mar (target-driven) ─────────────────────────────────────────────────────

// applyExchangeCourtiersMar records the target player's mar choices:
//   - "fair_trade" → the trade goes through anyway: the targeted peer still
//     passes to the preparer (inline).
//   - "forfeit"    → the target claims one of the preparer's peers.
//   - "riposte"    → the target claims one of the preparer's peers; the
//     preparer may break it first (riposte-break route).
//
// Each riposte/forfeit adds one required peer-claim, performed via claim-peer.
func applyExchangeCourtiersMar(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	choices []string,
	resData *ResolutionData,
) error {
	ec := resData.EnsureExchangeCourtiers()
	for _, c := range choices {
		switch c {
		case "fair_trade":
			if plan.TargetAssetID == nil || plan.TargetPlayerID == nil {
				continue
			}
			recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
			if err != nil {
				return err
			}
			if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
				ID:      *plan.TargetAssetID,
				OwnerID: recipient,
			}); err != nil {
				return err
			}
			ta, _ := deps.Q.GetAssetByID(ctx, *plan.TargetAssetID)
			broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
				Asset:      ta,
				OldOwnerID: *plan.TargetPlayerID,
				NewOwnerID: recipient,
			})
			ecLog(ctx, deps, plan, model.SeverityImportant,
				fmt.Sprintf("A fair trade: %q passed to %s after all.",
					ta.Name, playerDisplayName(ctx, deps.Q, recipient)))
		case "forfeit":
			ec.PeerClaimsRequired++
		case "riposte":
			ec.PeerClaimsRequired++
			ec.RiposteAllowed = true
		}
	}
	// The target has now made their mar choices. Advance the cursor: a
	// forfeit/riposte leaves peer claims outstanding (still target-side); a
	// fair_trade-only mar leaves no trace, so resolution is done. This phase
	// advance is what lets the WaitingOnBar stop blocking on the target without
	// a separate "mar choices submitted" marker.
	if ec.PeerClaimsRequired > 0 {
		ec.Phase = gamepkg.ECPhasePeerClaims
	} else {
		ec.Phase = gamepkg.ECPhaseDone
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
			respondInternalErr(w, r, "could not load game", err)
			return
		}

		resData := loadResolutionData(plan.ResolutionData)

		switch body.Action {
		case "offer":
			offerFairTrade(r, ctx, w, q, &resData, plan, player, body.OfferedAssetID)
		case "accept":
			acceptFairTrade(r, ctx, w, q, &resData, plan, player, manager, game)
		case "decline":
			declineFairTrade(r, ctx, w, q, &resData, plan, player, manager, game)
		default:
			respondErr(w, http.StatusBadRequest, "action must be 'offer', 'accept', or 'decline'")
		}
	}
}

func offerFairTrade(
	r *http.Request,
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
	resData.EnsureExchangeCourtiers().FairTradeAssetID = offeredAssetID
	if err := saveResolutionData(ctx, q, plan.ID, *resData); err != nil {
		respondInternalErr(w, r, "could not save offer", err)
		return
	}
	respond(w, http.StatusOK, map[string]any{
		"plan_id":          plan.ID,
		"offered_asset_id": offeredAssetID,
	})
}

func acceptFairTrade(
	r *http.Request,
	ctx context.Context, w http.ResponseWriter, q *dbgen.Queries,
	resData *ResolutionData, plan *dbgen.Plan, player *dbgen.Player,
	manager *hub.Manager, game dbgen.Game,
) {
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the preparer can accept or decline")
		return
	}
	ec := resData.ExchangeCourtiers
	if ec == nil || ec.FairTradeAssetID == nil {
		respondErr(w, http.StatusConflict, "no fair trade offer has been made yet")
		return
	}
	if plan.TargetAssetID == nil || plan.TargetPlayerID == nil {
		respondErr(w, http.StatusConflict, "plan is missing target asset or player")
		return
	}

	// Transfer targeted asset to preparer (or demand keep_assets winner).
	recipient, err := AssetRecipientForPlan(ctx, q, plan)
	if err != nil {
		respondInternalErr(w, r, "could not resolve asset recipient", err)
		return
	}
	if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      *plan.TargetAssetID,
		OwnerID: recipient,
	}); err != nil {
		respondInternalErr(w, r, "could not transfer targeted asset", err)
		return
	}
	// Transfer offered asset to target player.
	if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      *ec.FairTradeAssetID,
		OwnerID: *plan.TargetPlayerID,
	}); err != nil {
		respondInternalErr(w, r, "could not transfer offered asset", err)
		return
	}

	accepted := true
	ec.FairTradeAccepted = &accepted
	ec.Phase = gamepkg.ECPhaseDone // fair trade accepted → resolution complete
	if err := saveResolutionData(ctx, q, plan.ID, *resData); err != nil {
		respondInternalErr(w, r, "could not save decision", err)
		return
	}

	if err := q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
		ID:     plan.ID,
		Result: new(makeOutcome),
	}); err != nil {
		respondInternalErr(w, r, "could not resolve plan", err)
		return
	}

	ta, _ := q.GetAssetByID(ctx, *plan.TargetAssetID)
	oa, _ := q.GetAssetByID(ctx, *ec.FairTradeAssetID)
	if h, hasHub := manager.Get(game.ID); hasHub {
		h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
			Asset:      ta,
			OldOwnerID: *plan.TargetPlayerID,
			NewOwnerID: recipient,
		})
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
	// Log both legs of the fair-trade swap; EmitPlanResolved only records the
	// outcome, not which peers changed hands.
	EmitAssetTaken(ctx, q, manager, game.ID, ta, *plan.TargetPlayerID, recipient, &game.CurrentRow)
	EmitAssetTaken(ctx, q, manager, game.ID, oa, plan.PreparerID, *plan.TargetPlayerID, &game.CurrentRow)
	EmitPlanResolved(ctx, q, manager, *plan, makeOutcome)

	respond(w, http.StatusOK, map[string]any{
		"plan_id": plan.ID,
		"result":  "make",
		"note":    "fair trade accepted; assets exchanged",
	})
}

func declineFairTrade(
	r *http.Request,
	ctx context.Context, w http.ResponseWriter, q *dbgen.Queries,
	resData *ResolutionData, plan *dbgen.Plan, player *dbgen.Player,
	manager *hub.Manager, game dbgen.Game,
) {
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the preparer can accept or decline")
		return
	}
	declined := false
	ec := resData.EnsureExchangeCourtiers()
	ec.FairTradeAccepted = &declined
	ec.Phase = gamepkg.ECPhaseRoll // declined → dice roll + post-roll choice
	if err := saveResolutionData(ctx, q, plan.ID, *resData); err != nil {
		respondInternalErr(w, r, "could not save decision", err)
		return
	}

	h := ecHandler{}
	difficulty, err := h.ComputeDifficulty(ctx, q, plan, resData)
	if err != nil {
		respondInternalErr(w, r, "could not compute difficulty", err)
		return
	}
	roll, err := createPlanRoll(ctx, q, manager, &game, plan, difficulty, player.ID)
	if err != nil {
		respondInternalErr(w, r, "could not create dice roll", err)
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
// target player must tear one marginalia from one of the preparer's assets.
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
		ec := resData.EnsureExchangeCourtiers()
		if !ec.MessyBreakRequired {
			respondErr(w, http.StatusConflict, "no messy break is required for this plan")
			return
		}
		if ec.MessyBreakDone {
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
		if asset.OwnerID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "messy break must target one of the preparer's assets")
			return
		}

		// breakMarginalia tears + (if it was the last) destroys, emitting the
		// right events — fixing the prior inline tear that skipped auto-destroy.
		if _, err := breakMarginalia(ctx, q, manager, &asset, &m, player.ID); err != nil {
			respondInternalErr(w, r, "could not break asset", err)
			return
		}

		ec.MessyBreakDone = true
		ec.Phase = gamepkg.ECPhaseDone // last target-side step on a made roll
		if err := saveResolutionData(ctx, q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record messy break", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
			"asset_id":      asset.ID,
		})
	}
}

// ── Claim Peer (mar riposte / forfeit) ────────────────────────────────────────

// ecClaimPeerHandler handles POST /api/plans/:planId/claim-peer.
//
// On a mar, the target player takes one of the preparer's peers (riposte or
// forfeit). Each chosen riposte/forfeit option allows one claim; completion is
// blocked until all claims are made. Request body: {"asset_id": A}
func ecClaimPeerHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanExchangeCourtiers)
		if !ok {
			return
		}
		ctx := r.Context()
		if plan.TargetPlayerID == nil || player.ID != *plan.TargetPlayerID {
			respondErr(w, http.StatusForbidden, "only the target player claims a peer")
			return
		}
		if !planRollIsMar(ctx, deps.Q, plan) {
			respondErr(w, http.StatusConflict, "claim-peer is only available on a mar result")
			return
		}

		var body struct {
			AssetID int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AssetID == 0 {
			respondErr(w, http.StatusBadRequest, "asset_id is required")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		ec := resData.EnsureExchangeCourtiers()
		if ec.PeerClaimsDone >= ec.PeerClaimsRequired {
			respondErr(w, http.StatusConflict, "no peer claims remain for this plan")
			return
		}

		asset, err := deps.Q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.GameID != plan.GameID || asset.OwnerID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "you may only claim one of the preparer's peers")
			return
		}
		if asset.AssetType != model.AssetPeer {
			respondErr(w, http.StatusBadRequest, "asset must be a peer")
			return
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "that peer has been destroyed")
			return
		}

		if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID:      asset.ID,
			OwnerID: player.ID,
		}); err != nil {
			respondInternalErr(w, r, "could not transfer peer", err)
			return
		}

		ec.PeerClaimsDone++
		if ec.PeerClaimsDone >= ec.PeerClaimsRequired {
			ec.Phase = gamepkg.ECPhaseDone // all peer claims taken
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record claim", err)
			return
		}

		updated, _ := deps.Q.GetAssetByID(ctx, asset.ID)
		broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
			Asset:      updated,
			OldOwnerID: plan.PreparerID,
			NewOwnerID: player.ID,
		})
		ecLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("%s seized the peer %q from %s.",
				playerDisplayName(ctx, deps.Q, player.ID), asset.Name,
				playerDisplayName(ctx, deps.Q, plan.PreparerID)))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"asset_id":      asset.ID,
			"claims_done":   ec.PeerClaimsDone,
			"claims_needed": ec.PeerClaimsRequired,
		})
	}
}

// ── Riposte Break ─────────────────────────────────────────────────────────────

// ecRiposteBreakHandler handles POST /api/plans/:planId/riposte-break.
//
// Riposte lets the preparer break one of their own peers before it is claimed.
// Optional and only available when "riposte" was chosen. Request body:
// {"marginalia_id": M}
func ecRiposteBreakHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanExchangeCourtiers)
		if !ok {
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer may break their own peer")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		ec := resData.EnsureExchangeCourtiers()
		if !ec.RiposteAllowed {
			respondErr(w, http.StatusConflict, "riposte was not chosen for this plan")
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
		m, err := deps.Q.GetMarginaliaByID(ctx, body.MarginaliaID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "marginalia not found")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}
		asset, err := deps.Q.GetAssetByID(ctx, m.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.GameID != plan.GameID || asset.OwnerID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "you may only break one of your own peers")
			return
		}
		// Riposte damages the peer before the target takes it — it must survive
		// to be claimed, so refuse to tear its last intact marginalium (which
		// would destroy it and deadlock the pending claim).
		if intact, cErr := deps.Q.CountIntactMarginalia(ctx, asset.ID); cErr == nil && intact <= 1 {
			respondErr(w, http.StatusConflict,
				"cannot break the peer's last marginalia — it must survive to be claimed")
			return
		}

		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break peer", err)
			return
		}
		ecLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("The preparer damaged their own %q before surrendering it.", asset.Name))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
			"asset_id":      asset.ID,
			"destroyed":     destroyed,
		})
	}
}

// ResolvingWaitees narrows a resolving Exchange Courtiers to AwaitCourtierResponse
// during its target-driven sub-steps, so the bar blocks on the target player
// (never the preparer or focus player). It reads the explicit Phase cursor; the
// preparer's steps (accept/decline, make choice, riposte break, completion) fall
// through to the generic plan_resolving case (which names the preparer).
func (ecHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	if plan.TargetPlayerID == nil {
		return model.RowState{}, false
	}
	target := *plan.TargetPlayerID
	blockTarget := model.RowState{Kind: model.RowStateAwaitCourtierResponse, ActingPlayerIDs: []int64{target}}

	switch loadResolutionData(plan.ResolutionData).ExchangeCourtiers.CurrentPhase() {
	case gamepkg.ECPhaseFairTrade:
		// Target owes the opening offer until one is made; then the preparer
		// owes accept/decline (generic).
		ec := loadResolutionData(plan.ResolutionData).ExchangeCourtiers
		if ec == nil || ec.FairTradeAssetID == nil {
			return blockTarget, true
		}
		return model.RowState{}, false
	case gamepkg.ECPhaseRoll:
		// A marred roll hands the option choices to the target; block on them
		// until they submit (which advances the phase out of roll). The pre-roll
		// window and a made roll are the preparer's, so they ride the generic
		// case (planRollIsMar is false until the roll resolves as mar).
		if planRollIsMar(ctx, q, plan) {
			return blockTarget, true
		}
		return model.RowState{}, false
	case gamepkg.ECPhaseMessyBreak, gamepkg.ECPhasePeerClaims:
		// Target owes the messy break / remaining peer claims.
		return blockTarget, true
	case gamepkg.ECPhaseDone:
		// Resolution complete; the preparer completes (generic).
		return model.RowState{}, false
	}
	return model.RowState{}, false
}
