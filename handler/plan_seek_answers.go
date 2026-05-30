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
	"slices"

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

func (saHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.Notes == "" {
		return nil, "seek_answers requires preparation_notes describing research methods and topics"
	}
	return nil, ""
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

// ApplyChoice records the make/mar choices (kept in ResData.MakeMarChoices by
// MakeChoice) and, on a mar, fixes the self-flaw penalty. Mechanical effects
// (break_resource, reveal_secret) are performed afterwards via extra routes.
//
// Mar penalty (rules): "apply 'describe a flaw' to your own resource assets a
// number of times equal to (difficulty − result)." Because each resource may
// be flawed at most once and resources can only gain marginalia mid-resolution
// (never spawn anew), the effective requirement is capped here, once, at the
// number of the preparer's eligible own resources.
func (saHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	_ []string,
	result string,
) error {
	if result != marOutcome {
		saLog(ctx, deps, plan, model.SeverityDefault,
			"Seek Answers succeeded — the preparer presses their inquiries.")
		return nil
	}

	roll, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil {
		return fmt.Errorf("could not load roll for mar penalty: %w", err)
	}
	diff := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		diff = *roll.AdjustedDifficulty
	}
	var res int16
	if roll.Result != nil {
		res = *roll.Result
	}
	nominal := max(diff-res, 0)

	eligible, err := saEligibleOwnResources(ctx, deps, plan.GameID, plan.PreparerID, nil)
	if err != nil {
		return fmt.Errorf("could not count preparer resources: %w", err)
	}
	required := min(nominal, int16(len(eligible)))

	sa := resData.EnsureSeekAnswers()
	sa.MarSelfFlawsRequired = required
	saLog(ctx, deps, plan, model.SeverityImportant,
		fmt.Sprintf("Seek Answers marred — the preparer must describe a flaw in %d of their own resources.", required))
	return nil
}

// CanComplete blocks completion of a marred Seek Answers until the preparer has
// applied the full self-flaw penalty.
func (saHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	sa := resData.SeekAnswers
	if sa == nil {
		return nil
	}
	if sa.MarSelfFlawsApplied < sa.MarSelfFlawsRequired {
		return fmt.Errorf("you must describe a flaw in %d more of your own resources before completing",
			sa.MarSelfFlawsRequired-sa.MarSelfFlawsApplied)
	}
	return nil
}

// saEligibleOwnResources returns the player's non-destroyed resource assets
// that still have at least one intact marginalium and are not present in
// `flawed` (already flawed this resolution).
func saEligibleOwnResources(
	ctx context.Context,
	deps *PlanDeps,
	gameID, ownerID int64,
	flawed []int64,
) ([]dbgen.Asset, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	var out []dbgen.Asset
	for _, a := range assets {
		if a.GameID != gameID || a.AssetType != model.AssetResource || a.IsDestroyed {
			continue
		}
		if slices.Contains(flawed, a.ID) {
			continue
		}
		marg, err := deps.Q.ListIntactMarginalia(ctx, a.ID)
		if err != nil || len(marg) == 0 {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// saLog emits a Seek Answers action-log entry anchored to the plan's row.
func saLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.seek_answers",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

// MaxChoices: both make and mar choose options from the make list equal to the
// result. (On a mar the preparer additionally flaws their own resources
// difficulty − result times, applied via the break-resource route.)
func (saHandler) MaxChoices(_ string, rollResult, _ int16) int {
	return int(rollResult)
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
			respondErr(w, http.StatusForbidden, "only the preparer can break resources")
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

		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.EnsureSeekAnswers()

		// "Describe a flaw in any resource asset that has been overlooked until
		// now" — each resource may be flawed at most once per resolution.
		if slices.Contains(sa.FlawedResourceIDs, asset.ID) {
			respondErr(w, http.StatusConflict, "this resource has already been flawed in this plan")
			return
		}

		// On a mar, a flaw on the preparer's own resource discharges the
		// self-flaw penalty; a flaw on another player's resource is one of the
		// preparer's make-list options.
		isMar := planRollIsMar(ctx, deps.Q, plan)
		isPenalty := isMar && asset.OwnerID == plan.PreparerID

		// Break = tear one marginalium (auto-destroy if it was the last) — the
		// canonical effect, shared with every other plan.
		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break resource", err)
			return
		}

		sa.FlawedResourceIDs = append(sa.FlawedResourceIDs, asset.ID)
		if isPenalty && sa.MarSelfFlawsApplied < sa.MarSelfFlawsRequired {
			sa.MarSelfFlawsApplied++
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save flaw state", err)
			return
		}

		verb := "described a flaw in"
		if destroyed {
			verb = "destroyed"
		}
		saLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s %s %q.",
			playerDisplayName(ctx, deps.Q, player.ID), verb, asset.Name))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"asset_id":      asset.ID,
			"marginalia_id": m.ID,
			"destroyed":     destroyed,
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
			respondErr(w, http.StatusForbidden, "only the preparer can reveal secrets")
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
			respondInternalErr(w, r, "could not grant secret visibility", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
			AssetID:  body.AssetID,
			PlayerID: player.ID,
		})

		respond(w, http.StatusOK, map[string]any{
			"plan_id":  plan.ID,
			"asset_id": body.AssetID,
		})
	}
}
