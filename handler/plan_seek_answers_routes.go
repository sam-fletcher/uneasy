package handler

// handler/plan_seek_answers_routes.go — HTTP route handlers for Seek
// Answers' cast-roll/break/reveal/question sub-flow (the ExtraRoutes
// registered in plan_seek_answers.go). See that file for the plan's
// contract implementation and full lifecycle doc.

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

// ── Cast Roll (pre-roll) ──────────────────────────────────────────────────────

// saCastRollHandler handles POST /api/plans/:planId/seek-cast-roll. (The route
// key differs from Chronicle's cast-roll because extra-route keys share one
// global path namespace across plan types.)
//
// The pre-roll narration step: the preparer restates their research methods and
// describes one thing they've learned so far. This route posts that narration as
// the preparer's own scene post (anchored to the plan row), computes difficulty,
// closes the pre-roll, and casts the dice. The narration is required — it is the
// whole of the Seek Answers pre-roll.
//
// Request body: {"narration": "..."}
func saCastRollHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := saRequirePreparerResolving(w, r, deps.Q)
		if !ok {
			return
		}

		var body struct {
			Narration string `json:"narration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		narration, ok := textField(w, "narration", body.Narration, maxLongTextLen)
		if !ok {
			return
		}
		if narration == "" {
			respondErr(w, http.StatusBadRequest,
				"restate your methods and describe one thing you've learned before casting")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.EnsureSeekAnswers()
		if sa.PreRollDone {
			respondErr(w, http.StatusConflict, "the dice have already been cast")
			return
		}
		// Guard against a duplicate roll if cast-roll is somehow called twice
		// before PreRollDone lands.
		if existing, rErr := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID); rErr == nil && existing.ID != 0 {
			respondErr(w, http.StatusConflict, "a roll has already been created for this plan")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}
		difficulty, err := saHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondInternalErr(w, r, "could not compute difficulty", err)
			return
		}

		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}

		sa.PreRollDone = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not close pre-roll step", err)
			return
		}

		// Log the preparer's pre-roll narration as their own scene post, anchored
		// to the plan row (the same shape Chronicle Histories uses for its scene).
		post, perr := deps.Q.CreatePlayerMessage(ctx, dbgen.CreatePlayerMessageParams{
			GameID:    plan.GameID,
			AuthorID:  &player.ID,
			Body:      narration,
			RowNumber: plan.RowNumber,
			PlanID:    &plan.ID,
		})
		if perr == nil {
			if h, ok := deps.Manager.Get(plan.GameID); ok {
				h.BroadcastEvent(model.EventScenePostCreated, model.ScenePostCreatedPayload{Post: post})
			}
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"roll":    roll,
		})
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
//
// marginalia_id may be omitted (or 0) when the resource is blank — it has no
// marginalia to name, so the break destroys it outright (see resolveBreakTarget).
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
		// Breaking a resource is the preparer's make/mar resolution step, so a Make
		// Demands perform_steps winner may drive it in their stead. The mar self-flaw
		// penalty still targets the preparer's own resources (asset.OwnerID ==
		// plan.PreparerID below), regardless of who performs the break.
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
			return
		}

		var body struct {
			AssetID      int64 `json:"asset_id"`
			MarginaliaID int64 `json:"marginalia_id"`
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
		if asset.AssetType != model.AssetResource {
			respondErr(w, http.StatusBadRequest, "target asset must be a resource")
			return
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "asset is already destroyed")
			return
		}

		// nil m = a blank resource, flawed out of existence rather than torn.
		m, err := resolveBreakTarget(ctx, deps.Q, &asset, body.MarginaliaID)
		if err != nil {
			respondHTTPErr(w, r, err)
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

		// A flaw on the preparer's own resource on a mar discharges the self-flaw
		// penalty; every other flaw — make-list breaks on a make, and breaks on
		// ANOTHER player's resource on a mar — is one of the "choose options from
		// the make list equal to your result" picks (the rules give those on a mar
		// too). So the make-list cap applies to both makes and mar foreign breaks.
		isMar := planRollIsMar(ctx, deps.Q, plan)
		isPenalty := isMar && asset.OwnerID == plan.PreparerID

		// Server-authoritative completion: a flaw can't run more times than it was
		// owed — the self-flaw penalty is capped at the required count, a make-list
		// break (incl. a mar foreign break) at the picked count. Closes both the
		// duplicate-on-refresh window and the rules gap where a mar foreign break
		// was unbounded by the result.
		switch {
		case isPenalty:
			if sa.MarSelfFlawsApplied >= sa.MarSelfFlawsRequired {
				respondErr(w, http.StatusConflict, "all required self-flaws have already been applied")
				return
			}
		default:
			if int(sa.BreakResourceDone) >= pickedChoiceCount(&resData, "break_resource") {
				respondErr(w, http.StatusConflict, "break-resource already completed for this plan")
				return
			}
		}

		// Break = tear one marginalia (auto-destroy if it was the last), or
		// destroy a blank resource outright — the canonical effect, shared with
		// every other plan.
		destroyed, err := breakAsset(ctx, deps.Q, deps.Manager, &asset, m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not break resource", err)
			return
		}

		sa.FlawedResourceIDs = append(sa.FlawedResourceIDs, asset.ID)
		switch {
		case isPenalty:
			sa.MarSelfFlawsApplied++
		default:
			sa.BreakResourceDone++
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save flaw state", err)
			return
		}

		saLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s %s %s.%s",
			playerDisplayName(ctx, deps.Q, player.ID), breakVerb(destroyed), assetMark(asset.Name),
			brokenAssetDetail(ctx, deps.Q, asset.OwnerID, m, destroyed)))

		// marginalia_id echoes 0 for a blank resource — nothing was torn.
		var tornID int64
		if m != nil {
			tornID = m.ID
		}
		respond(w, http.StatusOK, map[string]any{
			"plan_id":       plan.ID,
			"asset_id":      asset.ID,
			"marginalia_id": tornID,
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
		// Revealing an asset's underside is the preparer's make resolution step, so a
		// Make Demands perform_steps winner may drive it in their stead. The
		// visibility is granted to the caller (player.ID) — the demander performing
		// the step is the one who learns the secret.
		if !requireResolutionActor(w, r.Context(), deps.Q, plan, player.ID) {
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
		// "Ask a PLAYER to show you the underside of a specific one of THEIR
		// assets" — the target is someone else's asset, never your own.
		if asset.OwnerID == plan.PreparerID {
			respondErr(w, http.StatusBadRequest, "reveal-secret targets another player's asset")
			return
		}

		// Server-authoritative completion: a stale client (re-prompted after a
		// refresh) must not reveal more assets than the picked reveal-secret count.
		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.EnsureSeekAnswers()
		if int(sa.RevealSecretDone) >= pickedChoiceCount(&resData, "reveal_secret") {
			respondErr(w, http.StatusConflict, "reveal-secret already completed for this plan")
			return
		}

		if err := deps.Q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
			AssetID:  body.AssetID,
			PlayerID: player.ID,
		}); err != nil {
			respondInternalErr(w, r, "could not grant secret visibility", err)
			return
		}

		sa.RevealSecretDone++
		sa.RevealedAssetIDs = append(sa.RevealedAssetIDs, asset.ID)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record reveal-secret progress", err)
			return
		}

		// Log the fact (not the content) so the action log records what happened —
		// every other make-list step logs, and this one previously didn't. The
		// existence of an asset's secrets is public; only their text stays gated.
		saLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s learned the secrets of %s.",
			playerDisplayName(ctx, deps.Q, player.ID), assetMark(asset.Name)))

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

// ── Forfeit Step ──────────────────────────────────────────────────────────────

// saForfeitStepHandler handles POST /api/plans/:planId/seek-forfeit-step.
//
// Discharges the remaining picks of a depletable step as a no-op when no valid
// target remains. Three steps can deplete: a make-list "break_resource" (every
// breakable resource already torn/destroyed), a make-list "reveal_secret" (no
// asset left whose secrets the preparer can't already read), and the mar
// "mar_penalty" self-flaw (no eligible own resource left). Without this escape
// hatch such a plan wedges — a committed pick can never be satisfied, so
// CanComplete blocks forever and the picker shows a dead, disabled button. This
// can arise from over-picking a category past its live target count, or from a
// concurrent plan removing the last target between commit and resolution.
//
// The server re-verifies that NO eligible target exists before discharging, so a
// stale or malicious client can't forfeit a step it could still perform. Nothing
// happens to any asset; the forfeit is logged for transparency.
//
// Request body: {"step": "break_resource" | "reveal_secret" | "mar_penalty"}
func saForfeitStepHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := saRequireActorResolving(w, r, deps.Q)
		if !ok {
			return
		}
		var body struct {
			Step string `json:"step"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.EnsureSeekAnswers()
		isMar := planRollIsMar(ctx, deps.Q, plan)

		var remaining, eligible int
		var noun string
		var err error
		switch body.Step {
		case "break_resource":
			remaining = pickedChoiceCount(&resData, "break_resource") - int(sa.BreakResourceDone)
			eligible, err = saEligibleBreakTargetCount(ctx, deps, plan, sa, isMar)
			noun = "resource to break"
		case "reveal_secret":
			remaining = pickedChoiceCount(&resData, "reveal_secret") - int(sa.RevealSecretDone)
			eligible, err = saEligibleRevealTargetCount(ctx, deps, plan)
			noun = "asset whose secrets to learn"
		case "mar_penalty":
			remaining = int(sa.MarSelfFlawsRequired - sa.MarSelfFlawsApplied)
			var own []int64
			own, err = saEligibleOwnResources(ctx, deps, plan.GameID, plan.PreparerID, sa.FlawedResourceIDs)
			eligible = len(own)
			noun = "own resource to break"
		default:
			respondErr(w, http.StatusBadRequest, "step must be break_resource, reveal_secret, or mar_penalty")
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
		case "break_resource":
			sa.BreakResourceDone += int16(remaining)
		case "reveal_secret":
			sa.RevealSecretDone += int16(remaining)
		case "mar_penalty":
			sa.MarSelfFlawsApplied += int16(remaining)
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record forfeit", err)
			return
		}

		picks := "picks"
		if remaining == 1 {
			picks = "pick"
		}
		saLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s had no eligible %s — %d %s forfeited.",
			playerDisplayName(ctx, deps.Q, player.ID), noun, remaining, picks))

		// The forfeit advances the preparer's sub-flow (a done-counter jumps), which
		// lives in resolution_data. Nudge non-actors to refetch the plan, since
		// broadcastRowState alone won't refresh resolution_data on their clients.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied,
			model.PlanChoiceAppliedPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "step": body.Step, "forfeited": remaining})
	}
}

// saEligibleBreakTargetCount counts resources the preparer could still break as a
// make-list pick: any breakable resource in the game, not yet flawed this plan,
// excluding the preparer's own on a mar (those route through the penalty flow).
// Mirror of the frontend brResourcesWithMarginalia picker.
func saEligibleBreakTargetCount(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, sa *gamepkg.SeekAnswersResolutionData, isMar bool,
) (int, error) {
	assets, err := deps.Q.ListAssetsByGame(ctx, plan.GameID)
	if err != nil {
		return 0, err
	}
	flawed := make(map[int64]bool, len(sa.FlawedResourceIDs))
	for _, id := range sa.FlawedResourceIDs {
		flawed[id] = true
	}
	count := 0
	for _, a := range assets {
		if a.AssetType != model.AssetResource || a.IsDestroyed || flawed[a.ID] {
			continue
		}
		if isMar && a.OwnerID == plan.PreparerID {
			continue
		}
		breakable, err := assetIsBreakable(ctx, deps.Q, a.ID)
		if err != nil {
			return 0, err
		}
		if breakable {
			count++
		}
	}
	return count, nil
}

// saEligibleRevealTargetCount counts assets whose secrets the preparer could
// still learn: another player's undestroyed asset that holds at least one secret
// the preparer can't yet read. Mirror of the frontend revealableAssets picker
// (preparer view).
func saEligibleRevealTargetCount(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (int, error) {
	assets, err := deps.Q.ListAssetsByGame(ctx, plan.GameID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, a := range assets {
		if a.IsDestroyed || a.OwnerID == plan.PreparerID {
			continue
		}
		total, err := deps.Q.CountSecretsByAsset(ctx, a.ID)
		if err != nil {
			return 0, err
		}
		if total == 0 {
			continue
		}
		visible, err := deps.Q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{
			AssetID:  a.ID,
			PlayerID: plan.PreparerID,
		})
		if err != nil {
			return 0, err
		}
		if int64(len(visible)) < total {
			count++
		}
	}
	return count, nil
}

// ── Declare Truth ───────────────────────────────────────────────────────────────

// saDeclareTruthHandler handles POST /api/plans/:planId/declare-truth.
//
// The make-list option "Declare something true about the world." Narrative: the
// preparer records the truth, which is logged at Default severity. (The "no
// contradictions" constraint is social, enforced at the table.)
// Request body: {"text": "..."}
func saDeclareTruthHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := saRequireActorResolving(w, r, deps.Q)
		if !ok {
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		text, ok := textField(w, "text", body.Text, maxNarrativeLen)
		if !ok {
			return
		}
		if text == "" {
			respondErr(w, http.StatusBadRequest, "truth text is required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.EnsureSeekAnswers()
		if int(sa.DeclareTruthDone) >= pickedChoiceCount(&resData, "declare_truth") {
			respondErr(w, http.StatusConflict, "declare-truth already completed for this plan")
			return
		}

		saLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s declared a truth: %q", playerDisplayName(ctx, deps.Q, player.ID), text))

		sa.DeclareTruthDone++
		sa.DeclaredTruths = append(sa.DeclaredTruths, text)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record declared truth", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID})
	}
}

// ── Ask Question ────────────────────────────────────────────────────────────────

// saAskQuestionHandler handles POST /api/plans/:planId/ask-question.
//
// The make-list option "Ask a player a question, which they must respond
// truthfully to." The preparer names a target (another player) and a question;
// it then awaits the target's answer or — if the target outranks the preparer on
// the knowledge track — a one-time veto. Only one question may be open at a time.
// Request body: {"target_id": N, "question": "..."}
func saAskQuestionHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, _, ok := saRequireActorResolving(w, r, deps.Q)
		if !ok {
			return
		}
		var body struct {
			TargetID int64  `json:"target_id"`
			Question string `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		question, ok := textField(w, "question", body.Question, maxNarrativeLen)
		if !ok {
			return
		}
		if body.TargetID == 0 || question == "" {
			respondErr(w, http.StatusBadRequest, "target_id and question are required")
			return
		}
		if body.TargetID == plan.PreparerID {
			respondErr(w, http.StatusBadRequest, "ask another player, not yourself")
			return
		}

		ctx := r.Context()
		target, err := deps.Q.GetPlayerByID(ctx, body.TargetID)
		if err != nil || target.GameID != plan.GameID {
			respondErr(w, http.StatusBadRequest, "target must be a player at this table")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.EnsureSeekAnswers()
		if sa.PendingQuestion != nil {
			respondErr(w, http.StatusConflict, "resolve the pending question before asking another")
			return
		}
		if int(sa.AskQuestionDone) >= pickedChoiceCount(&resData, "ask_question") {
			respondErr(w, http.StatusConflict, "ask-question already completed for this plan")
			return
		}

		// The target may veto only the FIRST formulation, and only if they
		// outrank the preparer on knowledge (lower rank number = higher status).
		vetoable := false
		if !sa.CurrentAskVetoed {
			targetRank, tErr := playerRankInCategory(ctx, deps.Q, plan.GameID, body.TargetID, model.CategoryKnowledge)
			preparerRank, pErr := playerRankInCategory(
				ctx,
				deps.Q,
				plan.GameID,
				plan.PreparerID,
				model.CategoryKnowledge,
			)
			if tErr == nil && pErr == nil && targetRank < preparerRank {
				vetoable = true
			}
		}

		sa.PendingQuestion = &gamepkg.SeekAnswersQuestion{
			TargetID: body.TargetID,
			Question: question,
			Vetoable: vetoable,
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record question", err)
			return
		}
		// The question text lives in the plan's resolution_data, which the target
		// watches via their plan panel. broadcastRowState only updates row_state on
		// the client, not the plan — so nudge non-actors to refetch the plan, else
		// the target's answer/veto UI stays hidden until a manual page refresh.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied,
			model.PlanChoiceAppliedPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "target_id": body.TargetID, "vetoable": vetoable})
	}
}

// saVetoQuestionHandler handles POST /api/plans/:planId/veto-question.
//
// The target — who outranks the preparer on knowledge — vetoes the open
// question once; the preparer must then ask another, which cannot be vetoed.
func saVetoQuestionHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := saRequireResolving(w, r, deps.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.SeekAnswers
		if sa == nil || sa.PendingQuestion == nil {
			respondErr(w, http.StatusConflict, "no question is awaiting a response")
			return
		}
		q := sa.PendingQuestion
		if player.ID != q.TargetID {
			respondErr(w, http.StatusForbidden, "only the player being asked may veto")
			return
		}
		if !q.Vetoable {
			respondErr(w, http.StatusConflict, "this question cannot be vetoed")
			return
		}

		saLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
			"%s vetoed %s's question — they outrank them on knowledge and must be asked another.",
			playerDisplayName(ctx, deps.Q, player.ID), playerDisplayName(ctx, deps.Q, plan.PreparerID)))

		sa.PendingQuestion = nil
		sa.CurrentAskVetoed = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record veto", err)
			return
		}
		// Clearing the pending question changes the preparer's panel (back to the
		// ask form). Nudge non-actors to refetch the plan, since broadcastRowState
		// alone won't refresh resolution_data on their clients.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied,
			model.PlanChoiceAppliedPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "vetoed": true})
	}
}

// saAnswerQuestionHandler handles POST /api/plans/:planId/answer-question.
//
// The target answers the open question truthfully; the question and answer are
// logged at Default severity and the ask-question pick is marked complete.
// Request body: {"answer": "..."}
func saAnswerQuestionHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := saRequireResolving(w, r, deps.Q)
		if !ok {
			return
		}
		var body struct {
			Answer string `json:"answer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		answer, ok := textField(w, "answer", body.Answer, maxNarrativeLen)
		if !ok {
			return
		}
		if answer == "" {
			respondErr(w, http.StatusBadRequest, "answer is required")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		sa := resData.SeekAnswers
		if sa == nil || sa.PendingQuestion == nil {
			respondErr(w, http.StatusConflict, "no question is awaiting an answer")
			return
		}
		q := sa.PendingQuestion
		if player.ID != q.TargetID {
			respondErr(w, http.StatusForbidden, "only the player being asked may answer")
			return
		}

		saLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s asked %s: %q — they answered: %q",
			playerDisplayName(ctx, deps.Q, plan.PreparerID), playerDisplayName(ctx, deps.Q, player.ID),
			q.Question, answer))

		sa.AnsweredQuestions = append(sa.AnsweredQuestions, gamepkg.SeekAnswersAnsweredQuestion{
			TargetID: q.TargetID,
			Question: q.Question,
			Answer:   answer,
		})
		sa.PendingQuestion = nil
		sa.CurrentAskVetoed = false
		sa.AskQuestionDone++
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record answer", err)
			return
		}
		// The answer advances the preparer's sub-flow (ask_question_done++, pending
		// cleared). Nudge non-actors to refetch the plan, since broadcastRowState
		// alone won't refresh resolution_data on their clients.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied,
			model.PlanChoiceAppliedPayload{PlanID: plan.ID})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID})
	}
}

// saRequireResolving loads the plan + caller and checks it's a Seek Answers plan
// in resolving status. Used by routes any participant may call (veto/answer).
func saRequireResolving(
	w http.ResponseWriter, r *http.Request, q *dbgen.Queries,
) (*dbgen.Plan, *dbgen.Player, bool) {
	plan, player, ok := requirePlanAccess(w, r, q)
	if !ok {
		return nil, nil, false
	}
	if plan.PlanType != model.PlanSeekAnswers {
		respondErr(w, http.StatusBadRequest, "this route is only for Seek Answers")
		return nil, nil, false
	}
	if plan.Status != model.PlanResolving {
		respondErr(w, http.StatusConflict, "plan is not in resolving status")
		return nil, nil, false
	}
	return plan, player, true
}

// saRequirePreparerResolving is saRequireResolving plus a strict preparer-only
// check — for the roll trigger (seek-cast-roll). The preparer still rolls even
// when a Make Demands perform_steps win has transferred the post-roll resolution
// steps, so this route is never relaxed.
func saRequirePreparerResolving(
	w http.ResponseWriter, r *http.Request, q *dbgen.Queries,
) (*dbgen.Plan, *dbgen.Player, bool) {
	plan, player, ok := saRequireResolving(w, r, q)
	if !ok {
		return nil, nil, false
	}
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the preparer can use this route")
		return nil, nil, false
	}
	return plan, player, true
}

// saRequireActorResolving is saRequireResolving plus the resolution-actor check —
// for the post-roll make-list resolution routes (declare-truth, ask-question,
// forfeit-step; break-resource and reveal-secret check inline). The actor is
// normally the preparer, but a Make Demands perform_steps win transfers the role
// to the winner (locking the preparer out). The roll trigger stays preparer-only
// via saRequirePreparerResolving.
func saRequireActorResolving(
	w http.ResponseWriter, r *http.Request, q *dbgen.Queries,
) (*dbgen.Plan, *dbgen.Player, bool) {
	plan, player, ok := saRequireResolving(w, r, q)
	if !ok {
		return nil, nil, false
	}
	if !requireResolutionActor(w, r.Context(), q, plan, player.ID) {
		return nil, nil, false
	}
	return plan, player, true
}
