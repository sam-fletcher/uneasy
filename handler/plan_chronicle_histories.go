package handler

// handler/plan_chronicle_histories.go — Chronicle Histories plan handler (Phase 3b).
//
// Chronicle Histories (knowledge, delay 5): The preparer investigates a
// historical problem through invoked artifacts.
//
// Difficulty = max(preparer's knowledge rank, count of invoked artifacts).
// NOTE: artifacts may be invoked during the pre-roll scene, so difficulty is
// computed in the cast-roll route (after the invoke phase closes), not at
// kickoff.
//
// Preparing: preparation_notes required (the historical problem).
//
// Pre-Roll: OnResolve opens the invoke phase (status 'resolving',
// InvokePhaseClosed=false) without creating a roll. The preparer invokes
// artifacts via the invoke-artifact route, then calls cast-roll to close the
// phase and create the dice roll. This mirrors Propose Decree's
// council → call-roll shape.
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
// route its owner through AssetRecipientForPlan(ctx, q, plan).
//
// Extra routes:
//   POST /api/plans/:planId/invoke-artifact   {"asset_id": N}
//   POST /api/plans/:planId/cast-roll         (preparer closes invoke phase, casts dice)
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

func (chHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.Notes == "" {
		return nil, "chronicle_histories requires preparation_notes describing the historical problem"
	}
	return nil, ""
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

// OnResolve opens the pre-roll invoke phase and returns nil — no roll yet.
// Mirrors Propose Decree's council shape: the preparer invokes artifacts via
// the invoke-artifact route during the pre-roll scene, then closes the phase
// and casts the dice via cast-roll (which computes difficulty and creates the
// roll). The plan sits in 'resolving' with InvokePhaseClosed=false until then.
func (chHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	ch := resData.EnsureChronicleHistories()
	ch.InvokePhaseClosed = false
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not open invoke phase: %w", err)
	}
	chLog(ctx, deps, plan, model.SeverityDefault,
		"Chronicle Histories pre-roll: set a scene from the past and invoke artifacts to shed light on it.")
	// Return nil — the roll is created later when the preparer calls cast-roll.
	return nil, nil
}

// ApplyChoice records the preparer's make choices (kept in MakeMarChoices by
// MakeChoice). Mechanical effects go through extra routes; here we only emit
// the action-log entry. Mar choices do NOT flow through here — every player
// submits via the mar-choice route — so this is a make-only path.
func (chHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	result string,
) error {
	if result == makeOutcome {
		chLog(ctx, deps, plan, model.SeverityImportant,
			"Chronicle Histories succeeded — the preparer shapes the scene from history.")
	}
	return nil
}

// MaxChoices caps the preparer's make picks at the dice result ("choose options
// equal to your result"). The mar path is per-player (one each) and is driven
// by the mar-choice route, not make-choice, so it carries no fixed budget here.
func (chHandler) MaxChoices(result string, rollResult, _ int16) int {
	if result == makeOutcome {
		return int(rollResult)
	}
	return -1
}

// CanComplete blocks a mar resolution until every player present when the mar
// scene began has submitted exactly one choice ("all players choose one option
// from the make list"). The make path has no completion prerequisite.
func (chHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	ch := resData.ChronicleHistories
	if ch == nil || !ch.MarActive {
		return nil
	}
	submitted := chDistinctMarChoosers(resData)
	if submitted < ch.MarRequiredChoices {
		return fmt.Errorf("%d of %d players still need to choose a mar option",
			ch.MarRequiredChoices-submitted, ch.MarRequiredChoices)
	}
	return nil
}

// chDistinctMarChoosers counts distinct players who have submitted a mar choice
// (entries in MakeMarChoices with a non-nil PlayerID).
func chDistinctMarChoosers(resData *ResolutionData) int16 {
	seen := map[int64]struct{}{}
	for _, c := range resData.MakeMarChoices {
		if c.PlayerID != nil {
			seen[*c.PlayerID] = struct{}{}
		}
	}
	return int16(len(seen))
}

// chLog emits a Chronicle Histories action-log entry anchored to the plan row.
func chLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.chronicle_histories",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

func (chHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"invoke-artifact": chInvokeArtifactHandler(deps),
		"cast-roll":       chCastRollHandler(deps),
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
			respondErr(w, http.StatusForbidden, "only the preparer can invoke artifacts")
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
		ch := resData.EnsureChronicleHistories()

		// The invoke-artifact route is only for the pre-roll scene. Once the
		// roll has been created (cast-roll sets InvokePhaseClosed), further
		// artifact invocations must come through the mar-choice "invoke_another"
		// path, which records narrative state without affecting difficulty.
		if ch.InvokePhaseClosed {
			respondErr(w, http.StatusConflict, "invoke phase is closed; the dice roll has been cast")
			return
		}

		// Idempotent: don't add duplicates.
		if !slices.Contains(ch.InvokedArtifactIDs, body.AssetID) {
			ch.InvokedArtifactIDs = append(ch.InvokedArtifactIDs, body.AssetID)
		}

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save invoked artifact", err)
			return
		}

		chLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s invoked %q to shed light on the past.",
				playerDisplayName(ctx, deps.Q, player.ID), asset.Name))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":              plan.ID,
			"invoked_artifact_ids": ch.InvokedArtifactIDs,
		})
	}
}

// ── Cast Roll ─────────────────────────────────────────────────────────────────

// chCastRollHandler handles POST /api/plans/:planId/cast-roll.
//
// The preparer closes the pre-roll invoke phase and casts the dice. Difficulty
// is computed here as max(knowledge rank, #invoked artifacts), so it reflects
// every artifact invoked during the pre-roll scene. Mirrors Propose Decree's
// call-roll: OnResolve left the plan in 'resolving' with no roll; this route
// creates it.
func chCastRollHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanChronicleHistories {
			respondErr(w, http.StatusBadRequest, "cast-roll is only for Chronicle Histories")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer can cast the roll")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		ch := resData.EnsureChronicleHistories()
		if ch.InvokePhaseClosed {
			respondErr(w, http.StatusConflict, "the dice have already been cast")
			return
		}

		// Guard against a duplicate roll if cast-roll is somehow called twice
		// before InvokePhaseClosed lands.
		existingRoll, rollErr := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && existingRoll.ID != 0 {
			respondErr(w, http.StatusConflict, "a roll has already been created for this plan")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}

		// Close the invoke phase first so any late invoke-artifact call is
		// rejected and can't affect the difficulty we're about to compute.
		ch.InvokePhaseClosed = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not close invoke phase", err)
			return
		}

		difficulty, err := chHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondInternalErr(w, r, "could not compute difficulty", err)
			return
		}

		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}

		chLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("The pre-roll scene closes with %d artifact(s) invoked — the dice are cast (difficulty %d).",
				len(ch.InvokedArtifactIDs), difficulty))

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"roll":    roll,
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
		ch := resData.ChronicleHistories
		if ch == nil || !slices.Contains(ch.InvokedArtifactIDs, body.AssetID) {
			respondErr(w, http.StatusBadRequest, "artifact has not been invoked in this plan")
			return
		}

		artifact, err := deps.Q.GetAssetByID(ctx, body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "artifact not found")
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

		// Break = tear one marginalium (auto-destroy if it was the last) — the
		// canonical effect, shared with every other plan.
		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &artifact, &m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break artifact", err)
			return
		}

		chLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s %s the invoked artifact %q.",
			playerDisplayName(ctx, deps.Q, player.ID), breakVerb(destroyed), artifact.Name))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"asset_id":      body.AssetID,
			"marginalia_id": m.ID,
			"destroyed":     destroyed,
		})
	}
}

// ── Mar Choice ────────────────────────────────────────────────────────────────

// chMarChoiceHandler handles POST /api/plans/:planId/mar-choice.
//
// During a Chronicle Histories mar result, ALL players present choose one
// option from the make list. Each player submits exactly once via this route;
// the plan cannot complete until every player has chosen (enforced by
// CanComplete against the player count captured on the first submission).
//
// Mechanical effects are applied immediately and atomically:
//   - break_artifact: requires asset_id + marginalia_id (an invoked artifact);
//     tears the marginalium via breakMarginalia (auto-destroy on the last).
//   - invoke_another: requires asset_id; adds the artifact to the invoked list.
//   - echo_present / total_control: narrative only.
//
// Request body: {"choice": "...", "asset_id": N, "marginalia_id": M}
//

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

		ctx := r.Context()

		// Mar choices are only valid on a marred roll.
		if !planRollIsMar(ctx, deps.Q, plan) {
			respondErr(w, http.StatusConflict, "mar-choice is only valid when the plan rolled a mar")
			return
		}

		var body struct {
			Choice       string `json:"choice"`
			AssetID      *int64 `json:"asset_id"`
			MarginaliaID *int64 `json:"marginalia_id"`
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

		resData := loadResolutionData(plan.ResolutionData)
		ch := resData.EnsureChronicleHistories()

		// One choice per player.
		for _, c := range resData.MakeMarChoices {
			if c.PlayerID != nil && *c.PlayerID == player.ID {
				respondErr(w, http.StatusConflict, "you have already chosen a mar option")
				return
			}
		}

		// Capture the gate target once, when the mar scene begins.
		if !ch.MarActive {
			count, err := deps.Q.CountPlayersInGame(ctx, plan.GameID)
			if err != nil {
				respondInternalErr(w, r, "could not count players", err)
				return
			}
			ch.MarActive = true
			ch.MarRequiredChoices = int16(count)
		}

		// Apply the mechanical effect (break_artifact / invoke_another mutate
		// state and DB; echo_present / total_control are narrative).
		logBody, status, msg := chApplyMarEffect(ctx, deps, plan, ch, player.ID, marEffectInput{
			choice: body.Choice, assetID: body.AssetID, marginaliaID: body.MarginaliaID,
		})
		if status != 0 {
			respondErr(w, status, msg)
			return
		}

		// Record as a Choice with PlayerID set so the panel can attribute it.
		resData.MakeMarChoices = append(resData.MakeMarChoices, Choice{
			PlayerID: &player.ID,
			Option:   body.Choice,
		})

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save mar choice", err)
			return
		}

		chLog(ctx, deps, plan, model.SeverityDefault, logBody)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":          plan.ID,
			"player_id":        player.ID,
			"choice":           body.Choice,
			"submitted":        chDistinctMarChoosers(&resData),
			"required_choices": ch.MarRequiredChoices,
		})
	}
}

// marEffectInput bundles the request params for one mar choice's mechanical
// effect.
type marEffectInput struct {
	choice       string
	assetID      *int64
	marginaliaID *int64
}

// chApplyMarEffect performs the mechanical effect of one mar choice and returns
// the action-log body. On a validation/lookup failure it returns a non-zero
// HTTP status and message for the caller to relay; status 0 means success.
// break_artifact and invoke_another mutate `ch`/DB; the narrative options are
// no-ops.
func chApplyMarEffect(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	ch *ChronicleHistoriesResolutionData,
	playerID int64,
	in marEffectInput,
) (logBody string, status int, msg string) {
	who := playerDisplayName(ctx, deps.Q, playerID)
	switch in.choice {
	case "break_artifact":
		if in.assetID == nil || in.marginaliaID == nil {
			return "", http.StatusBadRequest, "break_artifact requires asset_id and marginalia_id"
		}
		if !slices.Contains(ch.InvokedArtifactIDs, *in.assetID) {
			return "", http.StatusBadRequest, "artifact has not been invoked in this plan"
		}
		artifact, err := deps.Q.GetAssetByID(ctx, *in.assetID)
		if err != nil {
			return "", http.StatusNotFound, "artifact not found"
		}
		m, err := deps.Q.GetMarginaliaByID(ctx, *in.marginaliaID)
		if err != nil {
			return "", http.StatusNotFound, "marginalia not found"
		}
		if m.AssetID != *in.assetID {
			return "", http.StatusBadRequest, "marginalia does not belong to the specified artifact"
		}
		if m.IsTorn {
			return "", http.StatusConflict, "marginalia is already torn"
		}
		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &artifact, &m, playerID)
		if err != nil {
			return "", http.StatusInternalServerError, "could not break artifact"
		}
		return fmt.Sprintf("%s %s the invoked artifact %q.", who, breakVerb(destroyed), artifact.Name), 0, ""
	case "invoke_another":
		if in.assetID == nil {
			return "", http.StatusBadRequest, "invoke_another requires asset_id"
		}
		asset, err := deps.Q.GetAssetByID(ctx, *in.assetID)
		if err != nil || asset.GameID != plan.GameID || asset.AssetType != model.AssetArtifact {
			return "", http.StatusBadRequest, "asset must be an artifact in this game"
		}
		if !slices.Contains(ch.InvokedArtifactIDs, *in.assetID) {
			ch.InvokedArtifactIDs = append(ch.InvokedArtifactIDs, *in.assetID)
		}
		return fmt.Sprintf("%s invoked %q.", who, asset.Name), 0, ""
	default:
		// echo_present / total_control: purely narrative.
		return fmt.Sprintf("%s chose %q.", who, in.choice), 0, ""
	}
}
