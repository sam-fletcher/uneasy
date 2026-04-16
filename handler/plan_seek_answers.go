package handler

// handler/plan_seek_answers.go — Seek Answers plan handler (Phase 3b).
//
// Seek Answers (knowledge, delay 4): The preparer investigates a topic.
// Difficulty = preparer's rank on the knowledge track.
//
// Preparing: description of research methods/topics (preparation_notes required).
//
// Make: choose N options equal to the dice result (repeatable):
//   - "break_resource"  → tear a marginalia on a target resource asset
//   - "declare_truth"   → narrative (posted as scene_post)
//   - "ask_question"    → narrative; knowledge-ranked players may veto once
//   - "reveal_secret"   → grant secret visibility on all secrets of a chosen asset
//
// Mar: choose options equal to the result from the same list. Then the server
// applies "describe a flaw" (break_resource) to the preparer's own resource
// assets equal to (difficulty - result); the preparer chooses which ones.
//
// Extra routes:
//   POST /api/plans/:planId/break-resource   {"asset_id": N, "marginalia_id": M}
//   POST /api/plans/:planId/reveal-secret    {"asset_id": N}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSeekAnswers, saHandler{})
}

type saHandler struct{}

func (saHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: 4}
}

func (saHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (int16, string) {
	if v.Notes == "" {
		return 0, "seek_answers requires preparation_notes describing research methods and topics"
	}
	return 0, ""
}

func (saHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	rank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryKnowledge)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer knowledge rank: %w", err)
	}
	return gamepkg.SeekAnswersDifficulty(rank), nil
}

// OnResolve creates the dice roll immediately (no pre-roll step).
func (saHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResolutionData(plan.ResolutionData)
	difficulty, err := saHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

func (saHandler) ApplyChoice(
	_ context.Context,
	_ *PlanDeps,
	_ *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	_ string,
) error {
	// Choices are recorded via ResData.Choices (done in MakeChoice).
	// Mechanical effects (break_resource, reveal_secret) use extra routes.
	return nil
}

func (saHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return nil
}

func (saHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"break-resource": saBreakResourceHandler(deps),
		"reveal-secret":  saRevealSecretHandler(deps),
	}
}

// ── Break Resource ────────────────────────────────────────────────────────────

// saBreakResourceHandler handles POST /api/plans/:planId/break-resource.
//
// Tears a marginalia on a target resource asset. Used for:
//   - Make option "break_resource": target is any resource in the game.
//   - Mar penalty: preparer's own resource assets.
//
// Request body: {"asset_id": N, "marginalia_id": M}
func saBreakResourceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSeekAnswers {
			respondErr(w, http.StatusBadRequest, "break-resource is only for Seek Answers")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can break resources")
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

		asset, err := deps.Q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
			return
		}
		if asset.AssetType != model.AssetResource {
			respondErr(w, http.StatusBadRequest, "target asset must be a resource")
			return
		}

		m, err := deps.Q.GetMarginaliaByID(ctx, body.MarginaliaID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "marginalia not found")
			return
		}
		if m.AssetID != body.AssetID {
			respondErr(w, http.StatusBadRequest, "marginalia does not belong to the specified asset")
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
				AssetID:  asset.ID,
				Position: m.Position,
				TornByID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"asset_id":      asset.ID,
			"marginalia_id": m.ID,
		})
	}
}

// ── Reveal Secret ─────────────────────────────────────────────────────────────

// saRevealSecretHandler handles POST /api/plans/:planId/reveal-secret.
//
// Grants the preparer secret visibility on all secrets of a chosen asset.
//
// Request body: {"asset_id": N}
func saRevealSecretHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanSeekAnswers {
			respondErr(w, http.StatusBadRequest, "reveal-secret is only for Seek Answers")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the focus player can reveal secrets")
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
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		if asset.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
			return
		}

		if err := deps.Q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
			AssetID:  body.AssetID,
			PlayerID: player.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not grant secret visibility")
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
				AssetID:  body.AssetID,
				PlayerID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":  plan.ID,
			"asset_id": body.AssetID,
		})
	}
}
