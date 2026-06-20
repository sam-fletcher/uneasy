package handler

// handler/plan_spread_rumors.go — Spread Rumors plan handler (Phase 3b).
//
// Spread Rumors (esteem, delay 4): The preparer starts a rumor about an asset.
// Difficulty depends on the target:
//   - Target is a main character: 6 - target's esteem rank (target's esteem status)
//   - Any other asset:            preparer's rank on the esteem track
//
// Preparing: target_asset_id required; preparation_notes holds the rumor text.
//
// Make: server creates a rumors row. Then choose N options equal to dice result
// (repeatable):
//   - "break_target"  → tear a marginalia on the target asset
//   - "leverage_target" → leverage the target asset
//   - "take_asset"    → transfer any of the victim's assets (requires the
//                       victim's consent — see request/respond-take-consent)
//   - "hide_source"   → set rumors.source_player_id = NULL; write secret on own asset
//   - "reveal_source" → set rumors.source_player_id = preparer_id
//
// Mar: the target player describes a counter-rumor about the preparer. They
// choose options from the make list, applied against the preparer's assets,
// equal to (difficulty - result). Effects go through extra routes.
//
// Extra routes (all accept either the preparer on a make result, or the
// target-asset owner on a mar result):
//   POST /api/plans/:planId/break-target          {"marginalia_id": M, "asset_id"?: A}
//   POST /api/plans/:planId/hide-source           {"secret_asset_id": N, "secret_text"?: "..."}
//   POST /api/plans/:planId/request-take-consent  {"choices": [...], "result": "...", "take_asset_ids": [A, …]}
//   POST /api/plans/:planId/respond-take-consent  {"agree": true|false}
//
// On mar, asset_id specifies which of the preparer's assets the target player
// is tearing (the counter-rumor applies to preparer assets, not the plan's
// target asset).
//
// "take asset" is consent-gated: unlike break/leverage (which only touch the
// plan's original target asset), a take can claim ANY of the victim's assets —
// the target-asset owner on make, the preparer on mar — and the aggressor may
// take several (one per "take_asset" pick). The aggressor names the specific
// assets up front via request-take-consent; nothing is committed until the
// victim agrees via respond-take-consent. On agree the choices are applied and
// the assets transfer; on disagree nothing happens and the aggressor returns to
// the option picker with "take asset" disabled. See srRequestTakeConsentHandler.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSpreadRumors, srHandler{})
}

type srHandler struct{}

func (srHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 4}
}

func (srHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.TargetAssetID == nil {
		return nil, "spread_rumors requires target_asset_id"
	}
	if v.Notes == "" {
		return nil, "spread_rumors requires preparation_notes with the rumor text"
	}
	return nil, ""
}

func (srHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	if plan.TargetAssetID == nil {
		return 0, errors.New("spread_rumors plan has no target asset")
	}
	asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID)
	if err != nil {
		return 0, fmt.Errorf("could not load target asset: %w", err)
	}

	if asset.IsMainCharacter {
		// Difficulty = target player's esteem STATUS = 6 - rank.
		if asset.OwnerID == 0 {
			return 0, errors.New("main character asset has no owner")
		}
		targetRank, errRank := playerRankInCategory(ctx, q, plan.GameID, asset.OwnerID, model.CategoryEsteem)
		if errRank != nil {
			return 0, fmt.Errorf("could not determine target esteem rank: %w", errRank)
		}
		return gamepkg.SpreadRumorsDifficulty(targetRank, true), nil
	}

	// Difficulty = preparer's esteem rank.
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer esteem rank: %w", err)
	}
	return gamepkg.SpreadRumorsDifficulty(preparerRank, false), nil
}

// OnResolve creates the dice roll.
func (srHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResolutionData(plan.ResolutionData)
	difficulty, err := srHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

// ApplyChoice creates the rumors row on make and handles "leverage_target" and
// "reveal_source" which are pure DB ops. "break_target", "take_asset", and
// "hide_source" go through extra routes.
func (srHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not load game: %w", err)
	}

	// Create the rumor row (both make and mar create one; on mar the target
	// player describes a counter-rumor which is narrative — we still create
	// a placeholder rumor from the preparation_notes).
	rumorText := ""
	if plan.PreparationNotes != nil {
		rumorText = *plan.PreparationNotes
	}
	// If the preparer kept the rumor secret, its text lives in the Secret on
	// their asset rather than the (blanked) plan note. A Make now spreads it
	// publicly, so pull the text back out. On a Mar the secret stays hidden —
	// the rumor row is the target's counter-rumor placeholder instead.
	if sr := resData.SpreadRumors; result == makeOutcome && sr != nil && sr.IsSecret && sr.SecretID != nil {
		if secret, sErr := deps.Q.GetSecretByID(ctx, *sr.SecretID); sErr == nil {
			rumorText = secret.Text
		}
	}
	if rumorText == "" {
		rumorText = "(no rumor text)"
	}

	// Count existing rumors for display_order.
	existingRumors, _ := deps.Q.ListRumors(ctx, plan.GameID)
	displayOrder := int16(len(existingRumors))

	// Source attribution drives both the Rumors panel ("Spread by: …") and the
	// chat-log message. On make the preparer is the source — unless they chose
	// "hide_source", in which case the rumor is anonymous from the start (the
	// hide-source sub-flow later records the secret on one of their assets). On
	// mar the counter-rumor is left unattributed.
	hideSource := slices.Contains(choices, "hide_source")
	var sourcePlayerID *int64
	if result == makeOutcome && !hideSource {
		sourcePlayerID = &plan.PreparerID
	}

	rumor, err := deps.Q.CreateRumor(ctx, dbgen.CreateRumorParams{
		GameID:         game.ID,
		Text:           rumorText,
		TargetAssetID:  plan.TargetAssetID,
		OriginPlanID:   &plan.ID,
		SourcePlayerID: sourcePlayerID,
		DisplayOrder:   displayOrder,
	})
	if err != nil {
		return fmt.Errorf("could not create rumor: %w", err)
	}
	resData.EnsureSpreadRumors().RumorID = &rumor.ID
	if hideSource {
		resData.EnsureSpreadRumors().SourceHidden = true
	}

	broadcastEvent(deps.Manager, plan.GameID, model.EventRumorCreated, model.RumorCreatedPayload{Rumor: rumor})
	// Chat-log entry. EmitRumorCreated names the source when one is set and
	// stays anonymous otherwise, so a hidden spreader is never named here.
	EmitRumorCreated(ctx, deps.Q, deps.Manager, plan.GameID, rumor)

	// Apply inline choices.
	for _, choice := range choices {
		switch choice {
		case "leverage_target":
			// On mar the counter-rumor would leverage one of the preparer's
			// assets, but the flat choices list carries no asset picker. Treat
			// as narrative-only on mar; leverage the target asset on make.
			if result == marOutcome {
				continue
			}
			if plan.TargetAssetID != nil {
				if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
					ID:          *plan.TargetAssetID,
					IsLeveraged: true,
				}); err != nil {
					return fmt.Errorf("could not leverage target asset: %w", err)
				}
				broadcastEvent(
					deps.Manager,
					plan.GameID,
					model.EventAssetLeveraged,
					model.AssetIDPayload{AssetID: *plan.TargetAssetID},
				)
				if asset, aErr := deps.Q.GetAssetByID(ctx, *plan.TargetAssetID); aErr == nil {
					EmitAssetLeveraged(ctx, deps.Q, deps.Manager, plan.GameID, asset, game.CurrentRow)
				}
			}
		case "reveal_source":
			resData.EnsureSpreadRumors().SourceHidden = false
			// Source is already set to preparer above; nothing more needed.
		case "hide_source":
			// Handled via the extra route (requires secret text).
			resData.EnsureSpreadRumors().SourceHidden = true
		}
	}

	return nil
}

func (srHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return nil
}

// MaxChoices: make picks equal to the result (repeatable); the mar counter-rumor
// picks equal to (difficulty − result).
func (srHandler) MaxChoices(result string, rollResult, difficulty int16) int {
	if result == makeOutcome {
		return int(rollResult)
	}
	return int(difficulty - rollResult)
}

// PreparedDescriptor customizes the plan.prepared log post. It always names the
// target asset the rumor is about; when the preparer kept the rumor secret it
// names the asset holding the hidden text rather than the text itself (which
// would otherwise leak the secret), otherwise it states the rumor openly.
func (srHandler) PreparedDescriptor(
	ctx context.Context,
	q *dbgen.Queries,
	plan dbgen.Plan,
	resData *ResolutionData,
) (string, bool) {
	about := fallbackAssetName
	if plan.TargetAssetID != nil {
		about = assetDisplayName(ctx, q, *plan.TargetAssetID)
	}
	if sr := resData.SpreadRumors; sr != nil && sr.IsSecret && sr.SecretAssetID != nil {
		keeper := assetDisplayName(ctx, q, *sr.SecretAssetID)
		return fmt.Sprintf(
			"prepared Spread Rumors: there's a rumor brewing about %s, but it's a secret kept by %s.",
			about, keeper), true
	}
	notes := ""
	if plan.PreparationNotes != nil {
		notes = strings.TrimSpace(*plan.PreparationNotes)
	}
	if notes == "" {
		return fmt.Sprintf("prepared Spread Rumors: there's a rumor brewing about %s.", about), true
	}
	return fmt.Sprintf("prepared Spread Rumors: there's a rumor brewing about %s — %q", about, notes), true
}

func (srHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"break-target":         srBreakTargetHandler(deps),
		"hide-source":          srHideSourceHandler(deps),
		"request-take-consent": srRequestTakeConsentHandler(deps),
		"respond-take-consent": srRespondTakeConsentHandler(deps),
	}
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
		if sr.BreakTargetDone >= pickedChoiceCount(&resData, "break_target") {
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
			EmitMarginaliaTorn(ctx, deps.Q, deps.Manager, plan.GameID, asset, m, player.ID, destroyed, g.CurrentRow)
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
	// owner on mar — i.e. whoever requested the take.
	for _, aid := range req.AssetIDs {
		if err := transferRumorAsset(ctx, deps, plan, aid, req.RequestedBy); err != nil {
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
	if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID:      asset.ID,
		OwnerID: newOwnerID,
	}); err != nil {
		return fmt.Errorf("could not transfer asset: %w", err)
	}
	updated, _ := deps.Q.GetAssetByID(ctx, asset.ID)
	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
			Asset:      updated,
			OldOwnerID: oldOwnerID,
			NewOwnerID: newOwnerID,
		})
	}
	if g, gErr := deps.Q.GetGameByID(ctx, plan.GameID); gErr == nil {
		EmitAssetTaken(ctx, deps.Q, deps.Manager, plan.GameID, updated, oldOwnerID, newOwnerID, &g.CurrentRow)
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

		resData := loadResolutionData(plan.ResolutionData)
		sr := resData.SpreadRumors
		if sr == nil || sr.RumorID == nil {
			respondErr(w, http.StatusConflict, "rumor has not been created yet; call make-choice first")
			return
		}
		// Server-authoritative completion: a stale client (re-prompted after a
		// refresh) must not write more source-secrets than were picked.
		if sr.HideSourceDone >= pickedChoiceCount(&resData, "hide_source") {
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

// ResolvingWaitees returns AwaitTakeConsent while a Spread Rumors "take asset"
// choice is waiting on the victim's agree/disagree. ActingPlayerIDs names the
// victim (the asset owner), so the table blocks on them rather than the resolving
// plan's focus player. No pending consent → ride the generic PlanResolving case.
func (srHandler) ResolvingWaitees(_ context.Context, _ *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	if sr := resData.SpreadRumors; sr != nil && sr.PendingTakeConsent != nil {
		victim := sr.PendingTakeConsent.VictimID
		return model.RowState{Kind: model.RowStateAwaitTakeConsent, ActingPlayerIDs: []int64{victim}}, true
	}
	return model.RowState{}, false
}
