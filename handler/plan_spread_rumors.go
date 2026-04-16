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
//   - "take_asset"    → transfer target asset (social consent assumed)
//   - "hide_source"   → set rumors.source_player_id = NULL; write secret on own asset
//   - "reveal_source" → set rumors.source_player_id = preparer_id
//
// Mar: the target player describes a counter-rumor about the preparer. They
// choose options from the make list, applied against the preparer's assets,
// equal to (difficulty - result). Effects go through extra routes.
//
// Extra routes:
//   POST /api/plans/:planId/break-target    {"marginalia_id": M}
//   POST /api/plans/:planId/take-asset      {"consent": true}
//   POST /api/plans/:planId/hide-source     {"secret_asset_id": N, "secret_text": "..."}

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
	RegisterPlan(model.PlanSpreadRumors, srHandler{})
}

type srHandler struct{}

func (srHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 4}
}

func (srHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (int16, string) {
	if v.TargetAssetID == nil {
		return 0, "spread_rumors requires target_asset_id"
	}
	if v.Notes == "" {
		return 0, "spread_rumors requires preparation_notes with the rumor text"
	}
	return 0, ""
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
	if rumorText == "" {
		rumorText = "(no rumor text)"
	}

	// Count existing rumors for display_order.
	existingRumors, _ := deps.Q.ListRumors(ctx, plan.GameID)
	displayOrder := int16(len(existingRumors))

	// On make, source is the preparer by default. On mar, the "counter-rumor"
	// targets the preparer — but we still record the original preparer as source
	// unless hide_source is chosen later.
	var sourcePlayerID *int64
	if result == makeOutcome {
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
	resData.RumorID = &rumor.ID

	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(model.EventRumorCreated, model.RumorCreatedPayload{Rumor: rumor})
	}

	// Apply inline choices.
	for _, choice := range choices {
		switch choice {
		case "leverage_target":
			if plan.TargetAssetID != nil {
				if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
					ID:          *plan.TargetAssetID,
					IsLeveraged: true,
				}); err != nil {
					return fmt.Errorf("could not leverage target asset: %w", err)
				}
				if h, ok := deps.Manager.Get(plan.GameID); ok {
					h.BroadcastEvent(model.EventAssetLeveraged, model.AssetIDPayload{AssetID: *plan.TargetAssetID})
				}
			}
		case "reveal_source":
			resData.SourceHidden = false
			// Source is already set to preparer above; nothing more needed.
		case "hide_source":
			// Handled via the extra route (requires secret text).
			resData.SourceHidden = true
		}
	}

	return nil
}

func (srHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return nil
}

func (srHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"break-target": srBreakTargetHandler(deps),
		"take-asset":   srTakeAssetHandler(deps),
		"hide-source":  srHideSourceHandler(deps),
	}
}

// ── Break Target ──────────────────────────────────────────────────────────────

// srBreakTargetHandler handles POST /api/plans/:planId/break-target.
// Tears a marginalia on the target asset.
// Request body: {"marginalia_id": M}
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
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can use this route")
			return
		}
		if plan.TargetAssetID == nil {
			respondErr(w, http.StatusConflict, "plan has no target asset")
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
		if m.AssetID != *plan.TargetAssetID {
			respondErr(w, http.StatusBadRequest, "marginalia does not belong to the target asset")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		if err := deps.Q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
			ID:       m.ID,
			TornByID: &player.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not tear marginalia")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
				AssetID:  *plan.TargetAssetID,
				Position: m.Position,
				TornByID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"marginalia_id": m.ID,
		})
	}
}

// ── Take Asset ────────────────────────────────────────────────────────────────

// srTakeAssetHandler handles POST /api/plans/:planId/take-asset.
// Transfers the target asset to the preparer. Consent is social.
// Request body: {"consent": true}
func srTakeAssetHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSpreadRumors {
			respondErr(w, http.StatusBadRequest, "take-asset is only for Spread Rumors")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can use this route")
			return
		}
		if plan.TargetAssetID == nil {
			respondErr(w, http.StatusConflict, "plan has no target asset")
			return
		}

		var body struct {
			Consent bool `json:"consent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !body.Consent {
			respondErr(w, http.StatusBadRequest, "consent must be true to transfer asset")
			return
		}

		ctx := r.Context()

		asset, err := deps.Q.GetAssetByID(ctx, *plan.TargetAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "target asset not found")
			return
		}
		oldOwnerID := asset.OwnerID

		if err := deps.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID:      asset.ID,
			OwnerID: plan.PreparerID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not transfer asset")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			updated, _ := deps.Q.GetAssetByID(ctx, asset.ID)
			h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
				Asset:      updated,
				OldOwnerID: oldOwnerID,
				NewOwnerID: plan.PreparerID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":  plan.ID,
			"asset_id": asset.ID,
		})
	}
}

// ── Hide Source ───────────────────────────────────────────────────────────────

// srHideSourceHandler handles POST /api/plans/:planId/hide-source.
// Removes source attribution from the rumor and writes a secret on one of
// the preparer's own assets recording the hidden source.
// Request body: {"secret_asset_id": N, "secret_text": "..."}
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
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can use this route")
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

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		if resData.RumorID == nil {
			respondErr(w, http.StatusConflict, "rumor has not been created yet; call make-choice first")
			return
		}

		// Validate the secret-bearing asset belongs to the preparer.
		secretAsset, err := deps.Q.GetAssetByID(ctx, body.SecretAssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "secret asset not found")
			return
		}
		if secretAsset.OwnerID != plan.PreparerID || secretAsset.GameID != plan.GameID {
			respondErr(w, http.StatusForbidden, "secret asset must be your own asset in this game")
			return
		}

		// Remove source attribution from the rumor.
		if err := deps.Q.SetRumorSourceHidden(ctx, *resData.RumorID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not hide rumor source")
			return
		}

		// Write the secret on the chosen asset.
		secretText := body.SecretText
		if secretText == "" {
			secretText = "Source of a rumor (hidden)"
		}
		if _, err := deps.Q.CreateSecret(ctx, dbgen.CreateSecretParams{
			AssetID:  body.SecretAssetID,
			AuthorID: player.ID,
			Text:     secretText,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not write secret")
			return
		}

		resData.SourceHidden = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save hide-source state")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":         plan.ID,
			"rumor_id":        *resData.RumorID,
			"secret_asset_id": body.SecretAssetID,
		})
	}
}
