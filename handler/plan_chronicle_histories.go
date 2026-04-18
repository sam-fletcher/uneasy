package handler

// handler/plan_chronicle_histories.go — Chronicle Histories plan handler (Phase 3b).
//
// Chronicle Histories (knowledge, delay 5): The preparer investigates a
// historical problem through invoked artifacts.
//
// Difficulty = max(preparer's knowledge rank, count of invoked artifacts).
// NOTE: artifacts may be invoked during the pre-roll scene, so difficulty is
// recomputed when OnResolve is called (after the invoke phase closes).
//
// Preparing: preparation_notes required (the historical problem).
//
// Pre-Roll: preparer invokes artifacts via the extra route. After invoking,
// the preparer calls resolve to close the invoke phase and create the roll.
//
// Make: choose N options equal to the dice result (repeatable):
//   - "break_artifact"   → tear a marginalia on an invoked artifact
//   - "invoke_another"   → invoke another artifact (added to InvokedArtifactIDs)
//   - "echo_present"     → narrative only
//   - "total_control"    → requires consent from affected players (narrative)
//
// Mar: ALL players (not just preparer) each choose one option from the make list.
// Non-preparer choices go through the mar-choice extra route.
//
// TODO(make-demands keep_assets): CH currently does not award any new asset
// to the preparer — artifacts are invoked, not gained, and marginalia are
// torn in place. If future rules add a preparer-gained asset to this plan,
// route its owner through gamepkg.AssetRecipientForPlan(ctx, q, plan).
//
// Extra routes:
//   POST /api/plans/:planId/invoke-artifact   {"asset_id": N}
//   POST /api/plans/:planId/break-artifact    {"asset_id": N, "marginalia_id": M}
//   POST /api/plans/:planId/mar-choice        {"choice": "...", "asset_id": N}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanChronicleHistories, chHandler{})
}

type chHandler struct{}

func (chHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: 5}
}

func (chHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (int16, string) {
	if v.Notes == "" {
		return 0, "chronicle_histories requires preparation_notes describing the historical problem"
	}
	return 0, ""
}

func (chHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	resData *ResolutionData,
) (int16, error) {
	rank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryKnowledge)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer knowledge rank: %w", err)
	}
	return gamepkg.ChronicleHistoriesDifficulty(rank, *resData), nil
}

// OnResolve creates the dice roll using the current difficulty (which accounts
// for all artifacts invoked so far in the pre-roll phase).
func (chHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResolutionData(plan.ResolutionData)
	difficulty, err := chHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

func (chHandler) ApplyChoice(
	_ context.Context,
	_ *PlanDeps,
	_ *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	_ string,
) error {
	// Narrative choices; mechanical effects go through extra routes.
	return nil
}

func (chHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return nil
}

func (chHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"invoke-artifact": chInvokeArtifactHandler(deps),
		"break-artifact":  chBreakArtifactHandler(deps),
		"mar-choice":      chMarChoiceHandler(deps),
	}
}

// ── Invoke Artifact ───────────────────────────────────────────────────────────

// chInvokeArtifactHandler handles POST /api/plans/:planId/invoke-artifact.
//
// Usable during the pre-roll scene (plan status = 'resolving' before roll is
// created) and during make when "invoke_another" is chosen. Any artifact
// belonging to any player in the game may be invoked.
//
// Request body: {"asset_id": N}
func chInvokeArtifactHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanChronicleHistories {
			respondErr(w, http.StatusBadRequest, "invoke-artifact is only for Chronicle Histories")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can invoke artifacts")
			return
		}

		var body struct {
			AssetID int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AssetID == 0 {
			respondErr(w, http.StatusBadRequest, "asset_id is required")
			return
		}

		ctx := r.Context()

		asset, err := deps.Q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "artifact not found")
			return
		}
		if asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "artifact does not belong to this game")
			return
		}
		if asset.AssetType != model.AssetArtifact {
			respondErr(w, http.StatusBadRequest, "target asset must be an artifact")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)

		// Idempotent: don't add duplicates.
		if !slices.Contains(resData.InvokedArtifactIDs, body.AssetID) {
			resData.InvokedArtifactIDs = append(resData.InvokedArtifactIDs, body.AssetID)
		}

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save invoked artifact")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":              plan.ID,
			"invoked_artifact_ids": resData.InvokedArtifactIDs,
		})
	}
}

// ── Break Artifact ────────────────────────────────────────────────────────────

// chBreakArtifactHandler handles POST /api/plans/:planId/break-artifact.
//
// Tears a marginalia on an artifact that has been invoked in this plan.
// Usable by the preparer (make option "break_artifact") or by any player
// during mar (handled by game consensus; this endpoint enforces the invoked-
// artifact constraint but not make/mar context).
//
// Request body: {"asset_id": N, "marginalia_id": M}
func chBreakArtifactHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanChronicleHistories {
			respondErr(w, http.StatusBadRequest, "break-artifact is only for Chronicle Histories")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			AssetID      int64 `json:"asset_id"`
			MarginaliaID int64 `json:"marginalia_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AssetID == 0 || body.MarginaliaID == 0 {
			respondErr(w, http.StatusBadRequest, "asset_id and marginalia_id are required")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		if !slices.Contains(resData.InvokedArtifactIDs, body.AssetID) {
			respondErr(w, http.StatusBadRequest, "artifact has not been invoked in this plan")
			return
		}

		m, err := deps.Q.GetMarginaliaByID(ctx, body.MarginaliaID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "marginalia not found")
			return
		}
		if m.AssetID != body.AssetID {
			respondErr(w, http.StatusBadRequest, "marginalia does not belong to the specified artifact")
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
				AssetID:  body.AssetID,
				Position: m.Position,
				TornByID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"asset_id":      body.AssetID,
			"marginalia_id": m.ID,
		})
	}
}

// ── Mar Choice ────────────────────────────────────────────────────────────────

// chMarChoiceHandler handles POST /api/plans/:planId/mar-choice.
//
// During a Chronicle Histories mar result, ALL players (not just the preparer)
// each choose one option from the make list. This route lets non-preparer
// players submit their choice. Preparer choices still go through make-choice.
//
// Any player in the game (including the preparer calling this route again) may
// call this. Choices are recorded in ResData.Choices appended with
// "playerID:choice" encoding so multiple players' choices are tracked.
//
// Request body: {"choice": "break_artifact|invoke_another|echo_present|total_control", "asset_id": N}
// (asset_id required for break_artifact and invoke_another)
func chMarChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanChronicleHistories {
			respondErr(w, http.StatusBadRequest, "mar-choice is only for Chronicle Histories")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			Choice  string `json:"choice"`
			AssetID *int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Choice == "" {
			respondErr(w, http.StatusBadRequest, "choice is required")
			return
		}

		validChoices := []string{"break_artifact", "invoke_another", "echo_present", "total_control"}
		if !slices.Contains(validChoices, body.Choice) {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("choice must be one of: %v", validChoices))
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)

		// Apply mechanical effect.
		switch body.Choice {
		case "break_artifact":
			// Caller should follow up with the break-artifact route.
			// We just record the choice here.
		case "invoke_another":
			if body.AssetID != nil {
				if !slices.Contains(resData.InvokedArtifactIDs, *body.AssetID) {
					asset, err := deps.Q.GetAssetByID(ctx, *body.AssetID)
					if err == nil && asset.GameID == plan.GameID && asset.AssetType == model.AssetArtifact {
						resData.InvokedArtifactIDs = append(resData.InvokedArtifactIDs, *body.AssetID)
					}
				}
			}
		case "echo_present", "total_control":
			// Purely narrative.
		}

		// Record as "playerID:choice" in Choices.
		entry := fmt.Sprintf("%d:%s", player.ID, body.Choice)
		resData.Choices = append(resData.Choices, entry)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save mar choice")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": player.ID,
			"choice":    body.Choice,
		})
	}
}
