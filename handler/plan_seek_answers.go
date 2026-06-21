package handler

// handler/plan_seek_answers.go — Seek Answers plan handler (Phase 3b).
//
// Seek Answers (knowledge, delay 4): The preparer investigates a topic.
// Difficulty = preparer's rank on the knowledge track.
//
// Preparing: description of research methods/topics (preparation_notes required).
//
// Pre-Roll: OnResolve opens the plan in 'resolving' with no roll. The preparer
// restates their methods and describes one thing they've learned so far via the
// cast-roll route, which posts that narration as their own scene post, computes
// difficulty, and creates the dice roll. (Mirrors Chronicle Histories' cast-roll
// pre-roll, minus the artifact invocations — Seek Answers' pre-roll is purely
// narrative and does not affect difficulty.)
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
//   POST /api/plans/:planId/seek-cast-roll   {"narration": "..."}
//   POST /api/plans/:planId/break-resource   {"asset_id": N, "marginalia_id": M}
//   POST /api/plans/:planId/reveal-secret    {"asset_id": N}

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

// OnResolve opens the pre-roll narration step and returns nil — no roll yet.
// The plan sits in 'resolving' with PreRollDone=false until the preparer
// restates their methods, describes one thing they've learned, and casts the
// dice via the cast-roll route (which posts that narration, computes difficulty,
// and creates the roll). Mirrors Chronicle Histories. No log entry here — the
// standard plan.resolving post already marks the start, and the preparer's
// pre-roll narration is logged by cast-roll.
func (saHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	sa := resData.EnsureSeekAnswers()
	sa.PreRollDone = false
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not open pre-roll step: %w", err)
	}
	// Return nil — the roll is created later when the preparer calls cast-roll.
	return nil, nil
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

	// Imperative shell: load the snapshot, then let the pure rule decide the
	// penalty. saEligibleOwnResources builds a game.ResourceFlawView snapshot
	// from the DB and applies the pure eligibility filter.
	eligible, err := saEligibleOwnResources(ctx, deps, plan.GameID, plan.PreparerID, nil)
	if err != nil {
		return fmt.Errorf("could not count preparer resources: %w", err)
	}
	required := gamepkg.SeekAnswersMarPenalty(diff, res, int16(len(eligible)))

	sa := resData.EnsureSeekAnswers()
	sa.MarSelfFlawsRequired = required
	saLog(ctx, deps, plan, model.SeverityImportant,
		fmt.Sprintf("Seek Answers marred — the preparer must describe a flaw in %d of their own resources.", required))
	return nil
}

// CanComplete blocks completion of a marred Seek Answers until the preparer has
// applied the full self-flaw penalty, and blocks any Seek Answers while a
// question is still awaiting its target's answer or veto.
func (saHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	sa := resData.SeekAnswers
	if sa == nil {
		return nil
	}
	if sa.PendingQuestion != nil {
		return errors.New("a question is awaiting an answer before you can complete")
	}
	if sa.MarSelfFlawsApplied < sa.MarSelfFlawsRequired {
		return fmt.Errorf("you must describe a flaw in %d more of your own resources before completing",
			sa.MarSelfFlawsRequired-sa.MarSelfFlawsApplied)
	}
	return nil
}

// saEligibleOwnResources returns the IDs of the player's resource assets that
// can still be flawed this resolution. It is the imperative shell for the pure
// game.EligibleSelfFlawResourceIDs rule: it builds the domain snapshot from the
// DB (the owner's in-game assets plus each one's intact-marginalia count), then
// hands the decision to the pure filter.
func saEligibleOwnResources(
	ctx context.Context,
	deps *PlanDeps,
	gameID, ownerID int64,
	flawed []int64,
) ([]int64, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	views := make([]gamepkg.ResourceFlawView, 0, len(assets))
	for _, a := range assets {
		if a.GameID != gameID {
			continue
		}
		intact := 0
		// Only resources can be flawed; skip the marginalia query otherwise.
		if a.AssetType == model.AssetResource && !a.IsDestroyed {
			marg, err := deps.Q.ListIntactMarginalia(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			intact = len(marg)
		}
		views = append(views, gamepkg.ResourceFlawView{
			AssetID:               a.ID,
			AssetType:             a.AssetType,
			IsDestroyed:           a.IsDestroyed,
			IntactMarginaliaCount: intact,
		})
	}
	return gamepkg.EligibleSelfFlawResourceIDs(views, flawed), nil
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
		"seek-cast-roll":  saCastRollHandler(deps),
		"break-resource":  saBreakResourceHandler(deps),
		"reveal-secret":   saRevealSecretHandler(deps),
		"declare-truth":   saDeclareTruthHandler(deps),
		"ask-question":    saAskQuestionHandler(deps),
		"veto-question":   saVetoQuestionHandler(deps),
		"answer-question": saAnswerQuestionHandler(deps),
	}
}

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
		narration := strings.TrimSpace(body.Narration)
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
//nolint:gocognit // possibly improvable later
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

		// Break = tear one marginalia (auto-destroy if it was the last) — the
		// canonical effect, shared with every other plan.
		destroyed, err := breakMarginalia(ctx, deps.Q, deps.Manager, &asset, &m, player.ID)
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

		verb := "described a flaw in"
		if destroyed {
			verb = "destroyed"
		}
		saLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf("%s %s %s.%s",
			playerDisplayName(ctx, deps.Q, player.ID), verb, assetMark(asset.Name),
			brokenAssetDetail(ctx, deps.Q, asset.OwnerID, &m, destroyed)))

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
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record reveal-secret progress", err)
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

// ── Declare Truth ───────────────────────────────────────────────────────────────

// saDeclareTruthHandler handles POST /api/plans/:planId/declare-truth.
//
// The make-list option "Declare something true about the world." Narrative: the
// preparer records the truth, which is logged at Default severity. (The "no
// contradictions" constraint is social, enforced at the table.)
// Request body: {"text": "..."}
func saDeclareTruthHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := saRequirePreparerResolving(w, r, deps.Q)
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
		text := strings.TrimSpace(body.Text)
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
		plan, _, ok := saRequirePreparerResolving(w, r, deps.Q)
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
		question := strings.TrimSpace(body.Question)
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
		answer := strings.TrimSpace(body.Answer)
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

		sa.PendingQuestion = nil
		sa.CurrentAskVetoed = false
		sa.AskQuestionDone++
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not record answer", err)
			return
		}
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

// saRequirePreparerResolving is saRequireResolving plus a preparer-only check —
// for the make-list routes only the preparer drives (declare/ask).
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

// ResolvingWaitees returns AwaitQuestionAnswer while a Seek Answers "ask a player
// a question" pick is waiting on the target's answer or veto. ActingPlayerIDs
// names the target, so the table blocks on them rather than the resolving plan's
// focus player. No pending question → ride the generic PlanResolving case.
func (saHandler) ResolvingWaitees(_ context.Context, _ *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	resData := loadResolutionData(plan.ResolutionData)
	if sa := resData.SeekAnswers; sa != nil && sa.PendingQuestion != nil {
		target := sa.PendingQuestion.TargetID
		return model.RowState{Kind: model.RowStateAwaitQuestionAnswer, ActingPlayerIDs: []int64{target}}, true
	}
	return model.RowState{}, false
}
