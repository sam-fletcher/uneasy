package handler

// handler/plan_spread_propaganda.go — Spread Propaganda plan handler (Phase 3b).
//
// Spread Propaganda (esteem, delay 3): The preparer spreads a message.
// Difficulty = preparer's rank on the esteem track.
//
// Make options: "wide", "targeted", "incite" (narrative).
// Mar options:
//   (a) "backfire"  — rumor reflects back at preparer (narrative)
//   (b) "censured"  — esteem lockout: preparer's next plan cannot be an esteem plan
//   (c) "dismissed" — no one believes it (narrative)
//   (d) "co-opt"    — top interferer spreads their own propaganda immediately
//
// Esteem lockout (b): sets ResData.SpreadPropaganda.EsteemLockout = true.
// Checked in validatePlanPreparation for all esteem-category plan types.
//
// Recursive resolve (d): finds the top interferer by dice count (ties broken
// by best esteem rank), creates a new SP plan at current_row with status
// 'resolving', and creates a dice roll. Depth capped at 1.

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

func init() {
	RegisterPlan(model.PlanSpreadPropaganda, spHandler{})
}

type spHandler struct{}

func (spHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 3}
}

func (spHandler) ValidatePreparation(_ context.Context, _ *ValidationContext) (*int16, string) {
	return nil, "" // no plan-specific prerequisites; fixed delay
}

func (spHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer ranking: %w", err)
	}
	return gamepkg.SpreadPropagandaDifficulty(preparerRank), nil
}

// OnResolve creates the dice roll immediately (no pre-roll step).
func (spHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResolutionData(plan.ResolutionData)
	difficulty, err := spHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

// ApplyChoice resolves a Spread Propaganda make/mar.
//
// Make: always creates an artifact representing the societal shift (the rules'
// only make effect; "create_artifact" is the single make option in the UI).
//
// Mar (option keys match frontend MAR_OPTIONS.spread_propaganda):
//   - "lay_low"      → esteem lockout (preparer's next plan cannot be esteem)
//   - "break_self"   → preparer breaks one of their own assets (extra route)
//   - "give_peer"    → a peer leaves the preparer's retinue (extra route)
//   - "counter_prop" → the top interferer spreads their own propaganda now
func (spHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	if result == makeOutcome {
		return applySpreadPropagandaMake(ctx, deps, plan, resData)
	}

	for _, choice := range choices {
		switch choice {
		case "lay_low":
			resData.EnsureSpreadPropaganda().EsteemLockout = true
			spLog(ctx, deps, plan, model.SeverityDefault,
				fmt.Sprintf("%s must keep their head down — their next plan cannot involve esteem.",
					playerDisplayName(ctx, deps.Q, plan.PreparerID)))

		case "give_peer":
			// Asset picker — the actual transfer happens via the give-peer route.
			resData.EnsureSpreadPropaganda().GivePeerRequired = true

		case "break_self":
			// Asset picker — the actual break happens via the break-self route.
			resData.EnsureSpreadPropaganda().BreakSelfRequired = true

		case "counter_prop":
			if err := applyCoOpt(ctx, deps, plan, resData); err != nil {
				return err
			}
		}
	}
	return nil
}

// applySpreadPropagandaMake creates the artifact the make step mandates. The
// artifact is named from the preparation notes (the message) and routed
// through AssetRecipientForPlan so a resolved Make Demands keep_assets winner
// claims it. Idempotent: a second call is a no-op.
func applySpreadPropagandaMake(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
) error {
	sp := resData.EnsureSpreadPropaganda()
	if sp.ArtifactID != nil {
		return nil
	}

	recipient, err := AssetRecipientForPlan(ctx, deps.Q, plan)
	if err != nil {
		return fmt.Errorf("could not resolve asset recipient: %w", err)
	}

	// The artifact is created with a neutral placeholder name; the preparer
	// names it afterwards via the name-asset route. (Deliberately NOT derived
	// from the propaganda message — the artifact represents the societal shift,
	// which the preparer narrates, not a copy of their talking points.)
	asset, err := deps.Q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    plan.GameID,
		OwnerID:   recipient,
		CreatorID: plan.PreparerID,
		AssetType: model.AssetArtifact,
		Name:      propagandaArtifactNameDefault,
	})
	if err != nil {
		return fmt.Errorf("could not create societal-shift artifact: %w", err)
	}
	sp.ArtifactID = &asset.ID

	broadcastEvent(deps.Manager, plan.GameID, model.EventAssetCreated,
		model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: []dbgen.Marginalium{}}})
	spLog(ctx, deps, plan, model.SeverityDefault,
		fmt.Sprintf("%s reshaped society — created a new artifact to be named.",
			playerDisplayName(ctx, deps.Q, plan.PreparerID)))
	return nil
}

// CanComplete blocks completion until any chosen asset-picker mar effects have
// been performed.
func (spHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	sp := resData.SpreadPropaganda
	if sp == nil {
		return nil
	}
	if sp.GivePeerRequired && !sp.GivePeerDone {
		return errors.New("preparer must give a peer to another player (POST /plans/{planId}/give-peer)")
	}
	if sp.BreakSelfRequired && !sp.BreakSelfDone {
		return errors.New("preparer must break one of their own assets (POST /plans/{planId}/break-self)")
	}
	return nil
}

func (spHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"give-peer":     spGivePeerHandler(deps),
		"break-self":    spBreakSelfHandler(deps),
		"name-artifact": spNameAssetHandler(deps),
	}
}

// spNameAssetHandler handles POST /api/plans/:planId/name-artifact.
//
// The preparer names the artifact the made plan created (it starts with a
// placeholder). Optional; does not gate completion.
func spNameAssetHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nameCreatedPlanAsset(w, r, deps, model.PlanSpreadPropaganda,
			func(rd *ResolutionData) *int64 {
				if rd.SpreadPropaganda == nil {
					return nil
				}
				return rd.SpreadPropaganda.ArtifactID
			},
			func(rd *ResolutionData) { rd.EnsureSpreadPropaganda().ArtifactNamed = true },
		)
	}
}

// MaxChoices: make creates exactly one artifact; the mar list is chosen equal
// to (difficulty − result).
func (spHandler) MaxChoices(result string, rollResult, difficulty int16) int {
	if result == makeOutcome {
		return 1
	}
	return int(difficulty - rollResult)
}

// spLog emits a Spread Propaganda action-log entry anchored to the plan's row.
func spLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.spread_propaganda",
		severity, body, plan.RowNumber, &plan.ID, nil,
		map[string]any{"plan_id": plan.ID})
}

// ── Co-opt (recursive propaganda) ────────────────────────────────────────────

// applyCoOpt implements SP mar option (d): the top interferer spreads their
// own propaganda at the current row, resolving immediately.
func applyCoOpt(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
) error {
	// Depth cap: recursive plans cannot co-opt again.
	if sp := resData.SpreadPropaganda; sp != nil && sp.OriginalPlanID != nil {
		return errors.New("co-opt is not available on a recursive propaganda plan")
	}

	// Find the resolved roll for this plan.
	roll, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil {
		return fmt.Errorf("could not find dice roll for plan: %w", err)
	}

	// Find top interferer(s).
	interferers, err := deps.Q.ListInterferenceDiceByRoll(ctx, roll.ID)
	if err != nil || len(interferers) == 0 {
		return errors.New("co-opt is not available: no interference dice were committed to this roll")
	}

	topCount := interferers[0].DiceCount
	topPlayerID, err := pickBestEsteemRanked(ctx, deps.Q, plan.GameID, interferers, topCount)
	if err != nil {
		return fmt.Errorf("could not determine top interferer: %w", err)
	}

	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not load game: %w", err)
	}

	count, err := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    game.ID,
		RowNumber: new(game.CurrentRow),
	})
	if err != nil {
		count = 0
	}

	// Create the recursive SP plan.
	recursivePlan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:           game.ID,
		PlanType:         model.PlanSpreadPropaganda,
		Category:         model.CategoryEsteem,
		PreparerID:       topPlayerID,
		TargetPlayerID:   nil,
		TargetAssetID:    nil,
		RowNumber:        new(game.CurrentRow),
		RowOrder:         int16(count),
		PreparedAtRow:    game.CurrentRow,
		PreparationNotes: nil,
	})
	if err != nil {
		return fmt.Errorf("could not create recursive propaganda plan: %w", err)
	}

	// Mark it as resolving immediately (skips the pending phase).
	err = deps.Q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID:     recursivePlan.ID,
		Status: model.PlanResolving,
	})
	if err != nil {
		return fmt.Errorf("could not mark recursive plan as resolving: %w", err)
	}

	// Tag it in ResData so its own co-opt option is blocked.
	parentID := plan.ID
	recursiveResData := ResolutionData{
		SpreadPropaganda: &SpreadPropagandaResolutionData{OriginalPlanID: &parentID},
	}
	if err = saveResolutionData(ctx, deps.Q, recursivePlan.ID, recursiveResData); err != nil {
		return fmt.Errorf("could not save recursive plan data: %w", err)
	}

	// Compute difficulty for the recursive plan (top interferer's esteem rank).
	difficulty, err := spHandler{}.ComputeDifficulty(ctx, deps.Q, &recursivePlan, &recursiveResData)
	if err != nil {
		return fmt.Errorf("could not compute recursive plan difficulty: %w", err)
	}

	// Create the dice roll. The normal leverage window then opens.
	if _, err = createPlanRoll(ctx, deps.Q, deps.Manager, &game, &recursivePlan, difficulty, topPlayerID); err != nil {
		return fmt.Errorf("could not create recursive dice roll: %w", err)
	}

	// Record the recursive plan ID in the parent's ResData.
	resData.EnsureSpreadPropaganda().RecursivePlanID = &recursivePlan.ID

	broadcastEvent(deps.Manager, game.ID, model.EventSPRecursivePlan, model.SPRecursivePlanPayload{
		ParentPlanID:    plan.ID,
		RecursivePlanID: recursivePlan.ID,
		PreparerID:      topPlayerID,
	})
	spLog(ctx, deps, plan, model.SeverityImportant,
		fmt.Sprintf("%s seized the moment and spread propaganda of their own — it resolves now.",
			playerDisplayName(ctx, deps.Q, topPlayerID)))

	return nil
}

// pickBestEsteemRanked selects the player with the lowest esteem rank
// (= highest status) among those tied at topCount interference dice.
func pickBestEsteemRanked(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	interferers []dbgen.ListInterferenceDiceByRollRow,
	topCount int64,
) (int64, error) {
	bestRank := int16(999)
	bestPlayerID := int64(0)

	for _, row := range interferers {
		if row.DiceCount < topCount {
			break // rows are ordered by dice_count DESC
		}
		rank, err := playerRankInCategory(ctx, q, gameID, row.PlayerID, model.CategoryEsteem)
		if err != nil {
			continue
		}
		if rank < bestRank {
			bestRank = rank
			bestPlayerID = row.PlayerID
		}
	}

	if bestPlayerID == 0 {
		if len(interferers) > 0 {
			return interferers[0].PlayerID, nil // fallback
		}
		return 0, errors.New("no interferers found")
	}
	return bestPlayerID, nil
}

// hasEsteemLockout lives in handler/eligibility.go (it queries Postgres).

// ── Give Peer (mar option a) ──────────────────────────────────────────────────

// spGivePeerHandler handles POST /api/plans/:planId/give-peer.
//
// Mar option (a): a peer leaves the preparer's retinue and is handed to
// another player. Only the preparer may call it, only after "give_peer" was
// chosen. Request body: {"peer_asset_id": A, "to_player_id": P}
func spGivePeerHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSpreadPropaganda {
			respondErr(w, http.StatusBadRequest, "give-peer is only for Spread Propaganda")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer gives up a peer")
			return
		}

		var body struct {
			PeerAssetID int64 `json:"peer_asset_id"`
			ToPlayerID  int64 `json:"to_player_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PeerAssetID == 0 || body.ToPlayerID == 0 {
			respondErr(w, http.StatusBadRequest, "peer_asset_id and to_player_id are required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sp := resData.EnsureSpreadPropaganda()
		if !sp.GivePeerRequired {
			respondErr(w, http.StatusConflict, "this plan did not choose to give up a peer")
			return
		}
		if sp.GivePeerDone {
			respondErr(w, http.StatusConflict, "a peer has already been given up for this plan")
			return
		}

		asset, err := deps.Q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "peer asset not found")
			return
		}
		if asset.GameID != plan.GameID || asset.OwnerID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "peer must be one of your own assets")
			return
		}
		if asset.AssetType != model.AssetPeer {
			respondErr(w, http.StatusBadRequest, "asset must be a peer")
			return
		}
		if body.ToPlayerID == plan.PreparerID {
			respondErr(w, http.StatusBadRequest, "give the peer to another player, not yourself")
			return
		}
		recipient, err := deps.Q.GetPlayerByID(ctx, body.ToPlayerID)
		if err != nil || recipient.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "recipient must be a player at this table")
			return
		}

		if _, err := takeAssetEffect(
			ctx,
			deps.Q,
			deps.Manager,
			plan.GameID,
			asset.ID,
			plan.PreparerID,
			body.ToPlayerID,
		); err != nil {
			respondInternalErr(w, r, "could not transfer peer", err)
			return
		}

		sp.GivePeerDone = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record give-peer", err)
			return
		}

		spLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s lost the peer %s to %s.",
				playerDisplayName(ctx, deps.Q, plan.PreparerID), assetMark(asset.Name),
				playerDisplayName(ctx, deps.Q, body.ToPlayerID)))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"peer_asset_id": asset.ID,
			"to_player_id":  body.ToPlayerID,
		})
	}
}

// ── Break Self (mar option c) ─────────────────────────────────────────────────

// spBreakSelfHandler handles POST /api/plans/:planId/break-self.
//
// Mar option (c): "word of your laughable ideas gets around — break yourself."
// The preparer tears one marginalia from one of their own assets. Only the
// preparer may call it, only after "break_self" was chosen.
// Request body: {"marginalia_id": M}
func spBreakSelfHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSpreadPropaganda {
			respondErr(w, http.StatusBadRequest, "break-self is only for Spread Propaganda")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer breaks themselves")
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
		resData := loadResolutionData(plan.ResolutionData)
		sp := resData.EnsureSpreadPropaganda()
		if !sp.BreakSelfRequired {
			respondErr(w, http.StatusConflict, "this plan did not choose to break yourself")
			return
		}
		if sp.BreakSelfDone {
			respondErr(w, http.StatusConflict, "you have already broken an asset for this plan")
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
		asset, err := deps.Q.GetAssetByID(ctx, m.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.GameID != plan.GameID || asset.OwnerID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "you must break one of your own assets")
			return
		}

		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break asset", err)
			return
		}

		sp.BreakSelfDone = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record break-self", err)
			return
		}

		spLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s broke their own %s amid the backlash.%s",
				playerDisplayName(ctx, deps.Q, plan.PreparerID), assetMark(asset.Name),
				brokenAssetDetail(ctx, deps.Q, asset.OwnerID, &m, destroyed)))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
			"asset_id":      asset.ID,
			"destroyed":     destroyed,
		})
	}
}
