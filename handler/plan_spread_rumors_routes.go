package handler

// handler/plan_spread_rumors_routes.go — HTTP route handlers for Spread
// Rumors' break/hide/take-consent/forfeit sub-flow (the ExtraRoutes
// registered in plan_spread_rumors.go). See that file for the plan's
// contract implementation and full lifecycle doc.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── Forfeit Step ──────────────────────────────────────────────────────────────

// srForfeitStepHandler handles POST /api/plans/:planId/sr-forfeit-step.
//
// Discharges the remaining picks of a depletable step as a no-op when no valid
// target remains: break_target with the target asset out of intact marginalia
// (make) or all the preparer's assets out of them (mar), or hide_source with no
// asset of the actor's own to tuck the source-secret under. Mirrors Seek Answers'
// seek-forfeit-step — without it a committed pick with no target wedges the plan,
// since CanComplete now blocks until break_target/hide_source picks are consumed.
// The server re-verifies that NO eligible target exists before discharging.
//
// Request body: {"step": "break_target" | "hide_source"}
func srForfeitStepHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanSpreadRumors)
		if !ok {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		ctx := r.Context()
		onMar, authz := srAuthorizeActor(ctx, w, deps.Q, plan, player)
		if !authz {
			return
		}
		var body struct {
			Step string `json:"step"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		sr := resData.EnsureSpreadRumors()

		var remaining, eligible int
		var noun string
		var err error
		switch body.Step {
		case srOptBreakTarget:
			remaining = pickedChoiceCount(&resData, srOptBreakTarget) - sr.BreakTargetDone
			eligible, err = srEligibleBreakTargets(ctx, deps, plan, onMar)
			noun = "marginalia to tear"
		case srOptHideSource:
			remaining = pickedChoiceCount(&resData, srOptHideSource) - sr.HideSourceDone
			eligible, err = srEligibleHideAssets(ctx, deps, plan, player.ID)
			noun = "asset to hide the source under"
		default:
			respondErr(w, http.StatusBadRequest, "step must be break_target or hide_source")
			return
		}
		if err != nil {
			respondInternalErr(w, r, "could not count eligible targets", err)
			return
		}
		if remaining <= 0 {
			respondErr(w, http.StatusConflict, "no remaining picks to forfeit for this step")
			return
		}
		if eligible > 0 {
			respondErr(w, http.StatusConflict, "valid targets remain — complete the step instead of forfeiting")
			return
		}

		switch body.Step {
		case srOptBreakTarget:
			sr.BreakTargetDone += remaining
		case srOptHideSource:
			sr.HideSourceDone += remaining
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record forfeit", err)
			return
		}

		picks := "picks"
		if remaining == 1 {
			picks = "pick"
		}
		srLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s had no eligible %s — %d %s forfeited.",
			playerDisplayName(ctx, deps.Q, player.ID), noun, remaining, picks))

		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied,
			model.PlanChoiceAppliedPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "step": body.Step, "forfeited": remaining})
	}
}

// srEligibleBreakTargets counts marginalia the actor could still tear with a
// break_target pick: on a make, the intact marginalia on the (undestroyed) target
// asset; on a mar, the intact marginalia across all the preparer's assets.
func srEligibleBreakTargets(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, onMar bool) (int, error) {
	if onMar {
		assets, err := deps.Q.ListAssetsByOwner(ctx, plan.PreparerID)
		if err != nil {
			return 0, err
		}
		count := 0
		for _, a := range assets {
			if a.GameID != plan.GameID || a.IsDestroyed {
				continue
			}
			n, err := deps.Q.CountIntactMarginalia(ctx, a.ID)
			if err != nil {
				return 0, err
			}
			count += int(n)
		}
		return count, nil
	}
	if plan.TargetAssetID == nil {
		return 0, nil
	}
	asset, err := deps.Q.GetAssetByID(ctx, *plan.TargetAssetID)
	if err != nil {
		return 0, err
	}
	if asset.IsDestroyed {
		return 0, nil
	}
	n, err := deps.Q.CountIntactMarginalia(ctx, asset.ID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// srEligibleHideAssets counts the actor's own undestroyed assets available to
// tuck a hidden-source secret under. A secret can stack on any one of them, so a
// single eligible asset satisfies every remaining hide_source pick — the count is
// only ever used as "is it zero?".
func srEligibleHideAssets(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, actorID int64) (int, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, actorID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, a := range assets {
		if a.GameID == plan.GameID && !a.IsDestroyed {
			count++
		}
	}
	return count, nil
}

// stashSecretRumor implements the Spread Rumors "keep it secret for now" prep
// option: it moves the rumor text (currently in plan.PreparationNotes) into a
// hidden Secret on one of the preparer's own assets, clears the public note so
// ListPlans can't leak it, and records the secret metadata in resolution_data.
// plan is refreshed in place. Runs inside the prepare transaction (q tx-scoped);
// returns an httpErr on a bad secret asset or a write failure.
func stashSecretRumor(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, preparerID, secretAssetID int64,
	plan *dbgen.Plan,
) error {
	secretAsset, err := q.GetAssetByID(ctx, secretAssetID)
	if err != nil || secretAsset.GameID != gameID || secretAsset.OwnerID != preparerID || secretAsset.IsDestroyed {
		return httpErr(http.StatusBadRequest, "secret asset must be one of your own intact assets")
	}
	rumorText := ""
	if plan.PreparationNotes != nil {
		rumorText = *plan.PreparationNotes
	}
	secret, err := q.CreateSecret(ctx, dbgen.CreateSecretParams{
		AssetID:  secretAsset.ID,
		AuthorID: preparerID,
		Text:     rumorText,
	})
	if err != nil {
		return httpErr(http.StatusInternalServerError, "could not stash secret rumor")
	}
	if err := q.SetPlanPreparationNotes(ctx, dbgen.SetPlanPreparationNotesParams{
		ID:               plan.ID,
		PreparationNotes: nil,
	}); err != nil {
		return httpErr(http.StatusInternalServerError, "could not clear secret rumor note")
	}
	resData := loadResolutionData(plan.ResolutionData)
	sr := resData.EnsureSpreadRumors()
	sr.IsSecret = true
	sr.SecretAssetID = &secretAsset.ID
	sr.SecretID = &secret.ID
	if err := saveResolutionData(ctx, q, plan.ID, resData); err != nil {
		return httpErr(http.StatusInternalServerError, "could not save secret rumor state")
	}
	if refreshed, err := q.GetPlanByID(ctx, plan.ID); err == nil {
		*plan = refreshed
	}
	return nil
}

// srAuthorizeActor returns (onMar, ok). onMar is true when the caller is the
// target-asset owner acting during a mar result. It responds with the
// appropriate HTTP error if the caller is not authorized.
func srAuthorizeActor(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	player *dbgen.Player,
) (onMar bool, ok bool) {
	if player.ID == plan.PreparerID {
		return false, true
	}
	// Non-preparer: allowed only if (a) plan has a target asset, (b) caller owns
	// it, and (c) the roll resolved as "mar".
	if plan.TargetAssetID == nil {
		respondErr(w, http.StatusForbidden, "only the preparer can use this route")
		return false, false
	}
	asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "target asset not found")
		return false, false
	}
	if player.ID != asset.OwnerID {
		respondErr(w, http.StatusForbidden, "only the preparer or the target asset's owner can use this route")
		return false, false
	}
	roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil || roll.Outcome == nil || *roll.Outcome != marOutcome {
		respondErr(w, http.StatusForbidden, "target asset's owner can only act on a mar result")
		return false, false
	}
	return true, true
}

// ── Break Target ──────────────────────────────────────────────────────────────

// srBreakTargetHandler handles POST /api/plans/:planId/break-target.
//
// On make (preparer): tears a marginalia on the plan's target asset.
// Request body: {"marginalia_id": M}
//
// On mar (target-asset owner): tears a marginalia on one of the preparer's
// assets (the counter-rumor applies to preparer assets).
// Request body: {"marginalia_id": M, "asset_id": A}
//
//nolint:gocognit // possibly improvable later
func srBreakTargetHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSpreadRumors {
			respondErr(w, http.StatusBadRequest, "break-target is only for Spread Rumors")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		ctx := r.Context()
		onMar, authz := srAuthorizeActor(ctx, w, deps.Q, plan, player)
		if !authz {
			return
		}
		if !onMar && plan.TargetAssetID == nil {
			respondErr(w, http.StatusConflict, "plan has no target asset")
			return
		}

		// Server-authoritative completion: a stale client (re-prompted after a
		// refresh) must not tear more marginalia than were picked.
		resData := loadResolutionData(plan.ResolutionData)
		sr := resData.EnsureSpreadRumors()
		if sr.BreakTargetDone >= pickedChoiceCount(&resData, srOptBreakTarget) {
			respondErr(w, http.StatusConflict, "break-target already completed for this plan")
			return
		}

		var body struct {
			MarginaliaID int64  `json:"marginalia_id"`
			AssetID      *int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MarginaliaID == 0 {
			respondErr(w, http.StatusBadRequest, "marginalia_id is required")
			return
		}

		// Determine which asset's marginalia must match.
		var expectedAssetID int64
		if onMar {
			if body.AssetID == nil {
				respondErr(w, http.StatusBadRequest, "asset_id is required on mar (one of the preparer's assets)")
				return
			}
			preparerAsset, err := deps.Q.GetAssetByID(ctx, *body.AssetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, "asset not found")
				return
			}
			if preparerAsset.OwnerID != plan.PreparerID || preparerAsset.GameID != plan.GameID {
				respondErr(w, http.StatusBadRequest, "asset must be one of the preparer's assets in this game")
				return
			}
			expectedAssetID = preparerAsset.ID
		} else {
			expectedAssetID = *plan.TargetAssetID
		}

		m, err := deps.Q.GetMarginaliaByID(ctx, body.MarginaliaID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "marginalia not found")
			return
		}
		if m.AssetID != expectedAssetID {
			respondErr(w, http.StatusBadRequest, "marginalia does not belong to the specified asset")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		asset, err := deps.Q.GetAssetByID(ctx, expectedAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}

		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break target asset", err)
			return
		}
		// breakMarginalia logs the asset.destroyed post when a tear removes the
		// last marginalia, but not the tear itself — emit the canonical
		// marginalia.torn post so the break shows in the action log either way.
		if g, gErr := deps.Q.GetGameByID(ctx, plan.GameID); gErr == nil {
			EmitMarginaliaTorn(ctx, deps.Q, deps.Manager, plan.GameID, asset, m, player.ID, destroyed, logRow(g))
		}

		sr.BreakTargetDone++
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record break-target progress", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
		})
	}
}

// ── Take Asset (consent-gated) ─────────────────────────────────────────────────

// srRequestTakeConsentHandler handles POST /api/plans/:planId/request-take-consent.
//
// The aggressor (preparer on make, target-asset owner on mar) submits their full
// set of make/mar picks together with the specific assets they want to take
// (one asset_id per "take_asset" pick). Nothing is committed: the picks and
// asset list are stashed in resolution_data and the victim — the player who owns
// those assets — is asked to agree or disagree. ComputeRowState then surfaces
// await_take_consent so the WaitingOnBar names the victim and the table holds.
//
// The take may claim ANY of the victim's assets (not only the rumor's target
// asset). On make the victim is the target asset's owner; on mar the victim is
// the preparer. If the aggressor would be taking from themselves (a rumor about
// their own asset), no one else's consent is needed and the choices commit
// immediately.
//
// Request body: {"choices": [...], "result": "make"|"mar", "take_asset_ids": [A, …]}
//
// srBuildTakeConsentRequest decodes and validates a take-consent request body:
// the result must match the roll and fit the dice budget, there must be exactly
// one asset per "take_asset" pick, and every named asset must be a distinct,
// intact asset owned by the victim (the target-asset owner on make, the
// preparer on mar). On any problem it writes the HTTP error and returns ok=false.
//

func srBuildTakeConsentRequest(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	deps *PlanDeps,
	plan *dbgen.Plan,
	player *dbgen.Player,
	onMar bool,
) (*gamepkg.TakeConsentRequest, bool) {
	var body struct {
		Choices      []string `json:"choices"`
		Result       string   `json:"result"`
		TakeAssetIDs []int64  `json:"take_asset_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid JSON")
		return nil, false
	}
	if body.Result != makeOutcome && body.Result != marOutcome {
		respondErr(w, http.StatusBadRequest, "result must be 'make' or 'mar'")
		return nil, false
	}

	// The result must match the roll, and the picks must fit the dice budget.
	roll, rollErr := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
	if rollErr == nil && roll.Outcome != nil && *roll.Outcome != body.Result {
		respondErr(w, http.StatusConflict,
			fmt.Sprintf("result '%s' does not match roll outcome '%s'", body.Result, *roll.Outcome))
		return nil, false
	}
	var rollPtr *dbgen.DiceRoll
	if rollErr == nil {
		rollPtr = &roll
	}
	if !enforceChoiceBudget(w, srHandler{}, rollPtr, body.Result, body.Choices) {
		return nil, false
	}

	// Exactly one asset per "take_asset" pick.
	k := 0
	for _, c := range body.Choices {
		if c == "take_asset" {
			k++
		}
	}
	if k == 0 {
		respondErr(w, http.StatusBadRequest, "choices contain no take_asset to consent to")
		return nil, false
	}
	if len(body.TakeAssetIDs) != k {
		respondErr(w, http.StatusBadRequest,
			fmt.Sprintf("expected %d take_asset_ids to match your take_asset picks", k))
		return nil, false
	}

	// Resolve the victim: the player who would lose the assets.
	var victimID int64
	if onMar {
		victimID = plan.PreparerID
	} else {
		if plan.TargetAssetID == nil {
			respondErr(w, http.StatusConflict, "plan has no target asset")
			return nil, false
		}
		targetAsset, err := deps.Q.GetAssetByID(ctx, *plan.TargetAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "target asset not found")
			return nil, false
		}
		victimID = targetAsset.OwnerID
	}

	// Every named asset must be a distinct, intact asset the victim owns.
	seen := make(map[int64]bool, len(body.TakeAssetIDs))
	for _, aid := range body.TakeAssetIDs {
		if seen[aid] {
			respondErr(w, http.StatusBadRequest, "take_asset_ids must be distinct")
			return nil, false
		}
		seen[aid] = true
		asset, err := deps.Q.GetAssetByID(ctx, aid)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return nil, false
		}
		if asset.GameID != plan.GameID || asset.OwnerID != victimID {
			respondErr(w, http.StatusBadRequest, "each asset must belong to the player losing it")
			return nil, false
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusBadRequest, "cannot take a destroyed asset")
			return nil, false
		}
	}

	return &gamepkg.TakeConsentRequest{
		Choices:     body.Choices,
		Result:      body.Result,
		AssetIDs:    body.TakeAssetIDs,
		VictimID:    victimID,
		RequestedBy: player.ID,
	}, true
}

func srRequestTakeConsentHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanSpreadRumors)
		if !ok {
			return
		}
		ctx := r.Context()
		onMar, authz := srAuthorizeActor(ctx, w, deps.Q, plan, player)
		if !authz {
			return
		}

		req, ok := srBuildTakeConsentRequest(ctx, w, r, deps, plan, player, onMar)
		if !ok {
			return
		}

		// Self-consent: taking from yourself needs no one else's agreement.
		if req.VictimID == player.ID {
			if err := srCommitTakeConsent(ctx, deps, plan, req); err != nil {
				respondInternalErr(w, r, "could not apply choices", err)
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, model.EventRumorTakeConsentResolved,
				model.RumorTakeConsentPayload{PlanID: plan.ID})
			broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
			respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "committed": true})
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		sr := resData.EnsureSpreadRumors()
		sr.PendingTakeConsent = req
		sr.TakeAssetDenied = false
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save consent request", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventRumorTakeConsentRequested,
			model.RumorTakeConsentPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"pending":   true,
			"victim_id": req.VictimID,
		})
	}
}

// srRespondTakeConsentHandler handles POST /api/plans/:planId/respond-take-consent.
//
// Only the victim named in the open request may respond. On agree the stashed
// choices are committed (rumor created, leverage/reveal applied) and each named
// asset transfers to the aggressor. On disagree nothing is committed, the option
// is flagged denied (so the aggressor's picker disables it), and the aggressor
// returns to the option picker.
//
// Request body: {"agree": true|false}
func srRespondTakeConsentHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanSpreadRumors)
		if !ok {
			return
		}
		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		sr := resData.SpreadRumors
		if sr == nil || sr.PendingTakeConsent == nil {
			respondErr(w, http.StatusConflict, "no pending take-asset consent request")
			return
		}
		req := sr.PendingTakeConsent
		if player.ID != req.VictimID {
			respondErr(w, http.StatusForbidden, "only the asset owner may respond to this consent request")
			return
		}

		var body struct {
			Agree bool `json:"agree"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if !body.Agree {
			sr.PendingTakeConsent = nil
			sr.TakeAssetDenied = true
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save consent response", err)
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, model.EventRumorTakeConsentResolved,
				model.RumorTakeConsentPayload{PlanID: plan.ID})
			broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
			respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "agreed": false})
			return
		}

		if err := srCommitTakeConsent(ctx, deps, plan, req); err != nil {
			respondInternalErr(w, r, "could not apply consented choices", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventRumorTakeConsentResolved,
			model.RumorTakeConsentPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "agreed": true})
	}
}

// srCommitTakeConsent applies an agreed-to (or self-consented) take: it records
// the make/mar choices and runs srHandler.ApplyChoice exactly as MakeChoice
// would (creating the rumor row and applying inline leverage/reveal effects),
// then transfers each named asset to the aggressor and marks the take resolved.
// Clears the pending request. Mirrors the commit half of MakeChoice so the two
// paths stay in sync.
func srCommitTakeConsent(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	req *gamepkg.TakeConsentRequest,
) error {
	resData := loadResolutionData(plan.ResolutionData)
	resData.MakeMarChoices = make([]Choice, len(req.Choices))
	for i, opt := range req.Choices {
		resData.MakeMarChoices[i] = Choice{Option: opt}
	}
	if err := (srHandler{}).ApplyChoice(ctx, deps, plan, &resData, req.Choices, req.Result); err != nil {
		return fmt.Errorf("apply choices: %w", err)
	}
	// The aggressor receives the assets: the preparer on make, the target-asset
	// owner on mar — i.e. whoever requested the take. When the take is the
	// preparer's own gain (make), a resolved Make Demands keep_assets winner
	// intercepts it; a third party's mar riposte against the preparer is left
	// untouched (see AssetRecipientForPlan's preparer-scoped contract).
	recipient := req.RequestedBy
	if req.RequestedBy == plan.PreparerID {
		var rerr error
		if recipient, rerr = AssetRecipientForPlan(ctx, deps.Q, plan); rerr != nil {
			return fmt.Errorf("resolve asset recipient: %w", rerr)
		}
	}
	for _, aid := range req.AssetIDs {
		if err := transferRumorAsset(ctx, deps, plan, aid, recipient); err != nil {
			return err
		}
	}
	sr := resData.EnsureSpreadRumors()
	sr.TakeResolved = true
	sr.PendingTakeConsent = nil
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return fmt.Errorf("save resolution data: %w", err)
	}
	return nil
}

// transferRumorAsset transfers a single asset to newOwnerID and emits the
// asset.taken event + action-log post. Shared by the consent-grant path.
func transferRumorAsset(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	assetID, newOwnerID int64,
) error {
	asset, err := deps.Q.GetAssetByID(ctx, assetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	oldOwnerID := asset.OwnerID
	updated, err := takeAssetEffect(ctx, deps.Q, deps.Manager, plan.GameID, asset.ID, oldOwnerID, newOwnerID)
	if err != nil {
		return fmt.Errorf("could not transfer asset: %w", err)
	}
	if g, gErr := deps.Q.GetGameByID(ctx, plan.GameID); gErr == nil {
		EmitAssetTaken(ctx, deps.Q, deps.Manager, plan.GameID, updated, oldOwnerID, newOwnerID, logRow(g))
	}
	return nil
}

// ── Hide Source ───────────────────────────────────────────────────────────────

// srHideSourceHandler handles POST /api/plans/:planId/hide-source.
//
// Removes source attribution from the rumor and writes a secret on one of the
// actor's own assets recording the hidden source. On a make result the actor
// is the preparer; on a mar result the actor is the target-asset owner (who
// is hiding themselves as the source of the counter-rumor).
//
// The secret's text is auto-derived from the rumor itself ("You were the
// source of the rumor: …") so the actor only has to pick the asset to tuck it
// under. secret_text is an optional override.
// Request body: {"secret_asset_id": N, "secret_text"?: "..."}
func srHideSourceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSpreadRumors {
			respondErr(w, http.StatusBadRequest, "hide-source is only for Spread Rumors")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		ctx := r.Context()
		if _, authz := srAuthorizeActor(ctx, w, deps.Q, plan, player); !authz {
			return
		}

		var body struct {
			SecretAssetID int64  `json:"secret_asset_id"`
			SecretText    string `json:"secret_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SecretAssetID == 0 {
			respondErr(w, http.StatusBadRequest, "secret_asset_id is required")
			return
		}
		secretTextField, ok := textField(w, "secret_text", body.SecretText, maxNarrativeLen)
		if !ok {
			return
		}
		body.SecretText = secretTextField

		resData := loadResolutionData(plan.ResolutionData)
		sr := resData.SpreadRumors
		if sr == nil || sr.RumorID == nil {
			respondErr(w, http.StatusConflict, "rumor has not been created yet; call make-choice first")
			return
		}
		// Server-authoritative completion: a stale client (re-prompted after a
		// refresh) must not write more source-secrets than were picked.
		if sr.HideSourceDone >= pickedChoiceCount(&resData, srOptHideSource) {
			respondErr(w, http.StatusConflict, "hide-source already completed for this plan")
			return
		}

		// Validate the secret-bearing asset belongs to the caller.
		secretAsset, err := deps.Q.GetAssetByID(ctx, body.SecretAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "secret asset not found")
			return
		}
		if secretAsset.OwnerID != player.ID || secretAsset.GameID != plan.GameID {
			respondErr(w, http.StatusForbidden, "secret asset must be your own asset in this game")
			return
		}

		// Remove source attribution from the rumor.
		if err := deps.Q.SetRumorSourceHidden(ctx, *sr.RumorID); err != nil {
			respondInternalErr(w, r, "could not hide rumor source", err)
			return
		}

		// Write the secret on the chosen asset. By default the secret simply
		// records the rumor it conceals the source of; secret_text overrides it.
		secretText := strings.TrimSpace(body.SecretText)
		if secretText == "" {
			rumorText := "(no rumor text)"
			if rumor, rErr := deps.Q.GetRumorByID(ctx, *sr.RumorID); rErr == nil {
				rumorText = rumor.Text
			}
			secretText = fmt.Sprintf("You were the source of the rumor: %q", rumorText)
		}
		if _, err := deps.Q.CreateSecret(ctx, dbgen.CreateSecretParams{
			AssetID:  body.SecretAssetID,
			AuthorID: player.ID,
			Text:     secretText,
		}); err != nil {
			respondInternalErr(w, r, "could not write secret", err)
			return
		}

		sr.SourceHidden = true
		sr.HideSourceDone++
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save hide-source state", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":         plan.ID,
			"rumor_id":        *sr.RumorID,
			"secret_asset_id": body.SecretAssetID,
		})
	}
}
