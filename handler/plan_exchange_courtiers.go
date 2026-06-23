package handler

// handler/plan_exchange_courtiers.go — Exchange Courtiers plan handler.
//
// Exchange Courtiers (power, delay 5): The preparer attempts to take a peer
// from the target player. Resolution starts with a fair-trade pre-roll where
// the target names a peer in the *preparer's* retinue (the "requested" peer) to
// receive in exchange; the preparer accepts (the peers swap, no roll) or
// declines (a dice roll is created and the normal roll flow proceeds).
//
// Make options (preparer): "legal" (standard transfer), "messy" (transfer +
// target breaks one of the preparer's assets), "conspiracy" (narrative).
// Mar options (target-driven), all acting on the requested peer: "fair_trade"
// (the offered swap goes through), "forfeit" (the target takes the requested
// peer), "riposte" (same, but the preparer may break it first).

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

// AutoCompleteAfterChoice resolves Exchange Courtiers without a manual Complete
// click once its resolution cursor reaches ECPhaseDone — the ending view holds
// no decision, and the asset transfers already emitted their own log entries.
// Every terminal path lands on ECPhaseDone (a bare make/mar choice, or the
// messy-break / riposte-break sub-step that follows), so this covers them all;
// the messy and riposte choices park on their own phases until the sub-step is
// done, so they don't auto-complete early.
func (ecHandler) AutoCompleteAfterChoice(_ *dbgen.Plan, resData *ResolutionData) bool {
	return resData.ExchangeCourtiers.CurrentPhase() == gamepkg.ECPhaseDone
}

func (ecHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	ec := resData.ExchangeCourtiers
	if ec == nil {
		return nil
	}
	if ec.MessyBreakRequired && !ec.MessyBreakDone {
		return errors.New("target player must first break a marginalia (POST /plans/{planId}/messy-break)")
	}
	if ec.RiposteAllowed && !ec.RiposteBreakResolved {
		return errors.New(
			"preparer must first break or surrender the requested peer (POST /plans/{planId}/riposte-break)",
		)
	}
	return nil
}

func (ecHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"fair-trade":    fairTradeHandler(deps.Q, deps.Manager),
		"messy-break":   messyBreakHandler(deps.Q, deps.Manager),
		"riposte-break": ecRiposteBreakHandler(deps),
	}
}

// ecMakeLevels / ecMarLevels map each option to its rules "level" — the number
// printed on the rules card. The chosen option's level may not exceed the
// player's margin (result−difficulty for make; difficulty−result for mar). Make
// starts at 0 (Messy is forced even on a bare make); mar starts at 1 (a mar
// always falls short by ≥1).
var (
	ecMakeLevels = map[string]int16{"messy": 0, "legal": 1, "conspiracy": 2}
	ecMarLevels  = map[string]int16{"fair_trade": 1, "riposte": 2, "forfeit": 3}
)

// ValidateChoices enforces the EC rule: choose exactly one option, and its level
// may not exceed the margin between the result and the difficulty.
func (ecHandler) ValidateChoices(result string, rollResult, difficulty int16, choices []string) error {
	if len(choices) != 1 {
		return errors.New("choose exactly one option")
	}
	levels, margin := ecMakeLevels, rollResult-difficulty
	if result == marOutcome {
		levels, margin = ecMarLevels, difficulty-rollResult
	}
	level, ok := levels[choices[0]]
	if !ok {
		return fmt.Errorf("unknown option %q", choices[0])
	}
	if level > margin {
		return fmt.Errorf("option %q is beyond your margin of %d — pick a lower option", choices[0], margin)
	}
	return nil
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
			fmt.Sprintf("%s took the peer %s from %s.",
				playerDisplayName(ctx, deps.Q, recipient), assetMark(ta.Name),
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

// applyExchangeCourtiersMar records the target player's single mar choice. All
// three act on the requested peer (FairTradeAssetID), which the target named in
// the pre-roll trade:
//   - "fair_trade" → the offered swap goes through: targeted peer → preparer,
//     requested peer → target (inline).
//   - "forfeit"    → the target takes the requested peer (inline); no swap.
//   - "riposte"    → the preparer may break the requested peer first, then it
//     passes to the target (deferred to the riposte-break route).
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
				fmt.Sprintf("A fair trade: %s passed to %s after all.",
					assetMark(ta.Name), playerDisplayName(ctx, deps.Q, recipient)))
			// Second leg: the peer the target named from the preparer's retinue
			// passes back to them, completing the swap they offered pre-roll. If
			// no offer was made (the preparer skipped straight to the roll), only
			// the targeted peer changes hands.
			if ec.FairTradeAssetID != nil {
				if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
					ID:      *ec.FairTradeAssetID,
					OwnerID: *plan.TargetPlayerID,
				}); err != nil {
					return err
				}
				oa, _ := deps.Q.GetAssetByID(ctx, *ec.FairTradeAssetID)
				broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
					Asset:      oa,
					OldOwnerID: plan.PreparerID,
					NewOwnerID: *plan.TargetPlayerID,
				})
				ecLog(ctx, deps, plan, model.SeverityImportant,
					fmt.Sprintf("%s received %s in exchange.",
						playerDisplayName(ctx, deps.Q, *plan.TargetPlayerID), assetMark(oa.Name)))
			}
		case "forfeit":
			// The target takes the requested peer outright; no swap.
			if ec.FairTradeAssetID != nil {
				if err := ecGivePeerToTarget(ctx, deps, plan, *ec.FairTradeAssetID); err != nil {
					return err
				}
			}
		case "riposte":
			// The preparer gets first crack at the requested peer before it
			// passes to the target — deferred to the riposte-break route.
			ec.RiposteAllowed = true
		}
	}
	// Advance the cursor: a riposte still owes the preparer's break/surrender
	// step (which performs the transfer); fair_trade and forfeit both resolve
	// inline, so they're done. This is what lets the WaitingOnBar stop blocking
	// on the target without a separate "mar choices submitted" marker.
	if ec.RiposteAllowed {
		ec.Phase = gamepkg.ECPhaseRiposte
	} else {
		ec.Phase = gamepkg.ECPhaseDone
	}
	return nil
}

// ecGivePeerToTarget transfers a peer from the preparer to the target player,
// broadcasting the change and logging it. Used by the mar forfeit and riposte
// paths, where the target takes the peer they requested in the pre-roll trade.
func ecGivePeerToTarget(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, assetID int64) error {
	if plan.TargetPlayerID == nil {
		return nil
	}
	if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      assetID,
		OwnerID: *plan.TargetPlayerID,
	}); err != nil {
		return err
	}
	a, _ := deps.Q.GetAssetByID(ctx, assetID)
	broadcastEvent(deps.Manager, plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
		Asset:      a,
		OldOwnerID: plan.PreparerID,
		NewOwnerID: *plan.TargetPlayerID,
	})
	ecLog(ctx, deps, plan, model.SeverityImportant,
		fmt.Sprintf("%s took the requested peer %s from %s.",
			playerDisplayName(ctx, deps.Q, *plan.TargetPlayerID), assetMark(a.Name),
			playerDisplayName(ctx, deps.Q, plan.PreparerID)))
	return nil
}

// ── FairTrade extra route ─────────────────────────────────────────────────────

// fairTradeHandler handles POST /api/plans/:planId/fair-trade.
//
// Exchange Courtiers only. Three sub-actions via the body:
//
//	{"action": "offer",   "offered_asset_id": 123} — target names a peer in the
//	                                                  preparer's retinue to receive
//	{"action": "accept"}                           — preparer accepts; peers swapped
//	{"action": "decline"}                          — preparer declines; dice roll created
//
// Per the rules the target "indicates a peer in your [the preparer's] retinue
// that would serve as a fair trade"; accepting swaps it for the targeted peer.
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
			offerFairTrade(r, ctx, w, q, manager, &resData, plan, player, body.OfferedAssetID)
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
	ctx context.Context, w http.ResponseWriter, q *dbgen.Queries, manager *hub.Manager,
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
	// The rules have the target indicate a peer in the *preparer's* retinue to
	// receive in exchange for the targeted peer — not one of their own.
	if asset.OwnerID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "the fair trade must name one of the preparer's peers")
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
	// Nudge the other clients (the preparer, who now owes accept/decline) to
	// refetch — only the offering target gets this HTTP response.
	broadcastEvent(manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
		PlanID: plan.ID,
	})
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
	ec := resData.EnsureExchangeCourtiers()
	// The target must name a peer before the roll (the rules' pre-roll step), so
	// forfeit/riposte have a peer to take. The only exception is a preparer with
	// no peers to request — then the roll proceeds with nothing to surrender.
	if ec.FairTradeAssetID == nil {
		peers, err := q.CountPeerAssets(ctx, dbgen.CountPeerAssetsParams{
			GameID: plan.GameID, OwnerID: plan.PreparerID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not count preparer peers", err)
			return
		}
		if peers > 0 {
			respondErr(w, http.StatusConflict,
				"the target must propose a fair trade before you can roll")
			return
		}
	}
	declined := false
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

		// The messy break is the final step of a made roll, so the plan resolves
		// itself here. finalize fans out the resolved + row-state events; only
		// nudge a plain choice-applied refetch in the (unexpected) case it didn't.
		resolved, err := maybeAutoComplete(ctx, q, manager, ecHandler{}, plan, &resData, planResultString(ctx, q, plan))
		if err != nil {
			respondInternalErr(w, r, "could not complete plan", err)
			return
		}
		if !resolved {
			broadcastEvent(manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
				PlanID: plan.ID,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
			"asset_id":      asset.ID,
			"resolved":      resolved,
		})
	}
}

// ── Riposte Break ─────────────────────────────────────────────────────────────

// ecRiposteBreakHandler handles POST /api/plans/:planId/riposte-break.
//
// On a riposte the preparer acts on the requested peer (the one the target named
// in the pre-roll trade) before it passes to them: break a marginalia on it
// ({"marginalia_id": M}) or surrender it intact ({"action": "skip"}). Either
// way the peer then transfers to the target and the plan can complete. Only
// available when "riposte" was chosen, and only once.
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
		if ec.RiposteBreakResolved {
			respondErr(w, http.StatusConflict, "the riposte break has already been resolved")
			return
		}
		if ec.FairTradeAssetID == nil {
			respondErr(w, http.StatusConflict, "no peer was requested, so there is nothing to surrender")
			return
		}
		requestedID := *ec.FairTradeAssetID

		var body struct {
			Action       string `json:"action"`
			MarginaliaID int64  `json:"marginalia_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		ctx := r.Context()

		// Skip: the preparer surrenders the requested peer intact.
		if body.Action == "skip" {
			ecRiposteSurrender(ctx, w, r, deps, plan, &resData, requestedID)
			return
		}

		if body.MarginaliaID == 0 {
			respondErr(w, http.StatusBadRequest, "marginalia_id is required (or action 'skip')")
			return
		}

		m, err := deps.Q.GetMarginaliaByID(ctx, body.MarginaliaID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "marginalia not found")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}
		// The break must land on the requested peer — that is the one being
		// surrendered ("your peer, whom you may first break").
		if m.AssetID != requestedID {
			respondErr(w, http.StatusBadRequest, "the break must be on the requested peer")
			return
		}
		asset, err := deps.Q.GetAssetByID(ctx, requestedID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		// The peer must survive the break so it can still pass to the target, so
		// refuse to tear its last intact marginalia (which would destroy it).
		if intact, cErr := deps.Q.CountIntactMarginalia(ctx, asset.ID); cErr == nil && intact <= 1 {
			respondErr(w, http.StatusConflict,
				"cannot break the peer's last marginalia — it must survive to be surrendered")
			return
		}

		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break peer", err)
			return
		}
		ecLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("The preparer damaged their own %s before surrendering it.%s", assetMark(asset.Name),
				brokenAssetDetail(ctx, deps.Q, asset.OwnerID, &m, destroyed)))
		if err := ecGivePeerToTarget(ctx, deps, plan, requestedID); err != nil {
			respondInternalErr(w, r, "could not surrender peer", err)
			return
		}
		ec.RiposteBreakResolved = true
		ec.Phase = gamepkg.ECPhaseDone
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record riposte break", err)
			return
		}

		resolved, err := ecAutoCompleteAfterRiposte(ctx, deps, plan, &resData)
		if err != nil {
			respondInternalErr(w, r, "could not complete plan", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
			"asset_id":      asset.ID,
			"destroyed":     destroyed,
			"resolved":      resolved,
		})
	}
}

// ecRiposteSurrender handles the riposte "skip" sub-action: the preparer
// surrenders the requested peer intact, which transfers it to the target and
// lands the plan on ECPhaseDone, so it auto-completes.
func ecRiposteSurrender(
	ctx context.Context, w http.ResponseWriter, r *http.Request,
	deps *PlanDeps, plan *dbgen.Plan, resData *ResolutionData, requestedID int64,
) {
	if err := ecGivePeerToTarget(ctx, deps, plan, requestedID); err != nil {
		respondInternalErr(w, r, "could not surrender peer", err)
		return
	}
	ec := resData.EnsureExchangeCourtiers()
	ec.RiposteBreakResolved = true
	ec.Phase = gamepkg.ECPhaseDone
	if err := saveResolutionData(ctx, deps.Q, plan.ID, *resData); err != nil {
		respondInternalErr(w, r, "could not record riposte skip", err)
		return
	}
	resolved, err := ecAutoCompleteAfterRiposte(ctx, deps, plan, resData)
	if err != nil {
		respondInternalErr(w, r, "could not complete plan", err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "skipped": true, "resolved": resolved})
}

// ecAutoCompleteAfterRiposte resolves the plan once the preparer's riposte
// break/surrender — the final mar step — is recorded. finalize fans out the
// resolved + row-state events; if it didn't resolve (unexpected), fall back to a
// plain choice-applied nudge so clients still refetch. Returns whether it
// resolved the plan.
func ecAutoCompleteAfterRiposte(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, resData *ResolutionData,
) (bool, error) {
	resolved, err := maybeAutoComplete(
		ctx, deps.Q, deps.Manager, ecHandler{}, plan, resData, planResultString(ctx, deps.Q, plan))
	if err != nil {
		return false, err
	}
	if !resolved {
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
			PlanID: plan.ID,
		})
	}
	return resolved, nil
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
	case gamepkg.ECPhaseRiposte:
		// The preparer must break or surrender the requested peer; the row waits
		// on them (generic), not the target.
		return model.RowState{}, false
	case gamepkg.ECPhaseMessyBreak:
		// Target owes the messy break.
		return blockTarget, true
	case gamepkg.ECPhaseDone:
		// Resolution complete; the preparer completes (generic).
		return model.RowState{}, false
	}
	return model.RowState{}, false
}
