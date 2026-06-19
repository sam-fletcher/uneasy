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
// InvokePhaseClosed=false) without creating a roll. The preparer then sets the
// scene in one shot via cast-roll: it records the chosen artifact invocations,
// posts the scene narration, closes the phase, and creates the dice roll.
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
// NOTE: CH currently does not award any new asset to the preparer — artifacts
// are invoked, not gained, and marginalia are torn in place. If future rules
// add a preparer-gained asset to this plan, route its owner through
// AssetRecipientForPlan(ctx, q, plan).
//
// Extra routes:
//   POST /api/plans/:planId/cast-roll         {"artifact_ids": [N], "scene": "..."}
//   POST /api/plans/:planId/make-step         {"option": "...", "narration": "...", "asset_id": N, "marginalia_id": M}
//   POST /api/plans/:planId/mar-choice        {"choice": "...", "asset_id": N}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

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
// Mirrors Propose Decree's council shape: the plan sits in 'resolving' with
// InvokePhaseClosed=false until the preparer sets the scene, chooses the
// artifacts to invoke, and casts the dice via the cast-roll route (which records
// the invocations, posts the scene, computes difficulty, and creates the roll).
// No log entry here — the standard plan.resolving post already marks the start,
// and the preparer's own scene text is logged by cast-roll.
func (chHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	ch := resData.EnsureChronicleHistories()
	ch.InvokePhaseClosed = false
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("could not open invoke phase: %w", err)
	}
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

// CanComplete enforces both resolution gates without roll access:
//   - mar: every player present when the mar scene began must have submitted
//     exactly one choice ("all players choose one option from the make list");
//   - make: the preparer must have submitted all MakeBudget options
//     ("choose options equal to your result"), one per make-step.
func (chHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	ch := resData.ChronicleHistories
	if ch == nil {
		return nil
	}
	if ch.MarActive {
		submitted := chDistinctMarChoosers(resData)
		if submitted < ch.MarRequiredChoices {
			return fmt.Errorf("%d of %d players still need to choose a mar option",
				ch.MarRequiredChoices-submitted, ch.MarRequiredChoices)
		}
		return nil
	}
	if ch.MakeBudget > 0 && ch.MakeChoicesDone < ch.MakeBudget {
		return fmt.Errorf("%d of %d make choices still need to be submitted",
			ch.MakeBudget-ch.MakeChoicesDone, ch.MakeBudget)
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
		"cast-roll":  chCastRollHandler(deps),
		"make-step":  chMakeStepHandler(deps),
		"mar-choice": chMarChoiceHandler(deps),
	}
}

// ── Cast Roll ─────────────────────────────────────────────────────────────────

// chCastRollHandler handles POST /api/plans/:planId/cast-roll.
//
// The preparer sets the pre-roll scene in one shot: they choose the artifacts to
// invoke (all at once, so the difficulty meter settles before the roll) and
// optionally write the scene. This route records the invoked artifacts, posts
// the scene as the preparer's narration anchored to the plan row, closes the
// invoke phase, computes difficulty = max(knowledge rank, #invoked), and casts
// the dice.
//
// Request body: {"artifact_ids": [N, ...], "scene": "..."}
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

		var body struct {
			ArtifactIDs []int64 `json:"artifact_ids"`
			Scene       string  `json:"scene"`
		}
		// The body is optional (a scene with no invocations is valid); ignore a
		// decode error so an empty body resolves to the zero struct.
		_ = json.NewDecoder(r.Body).Decode(&body)

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

		// Validate and record the invoked artifacts (deduped). Each must be an
		// artifact belonging to a player in this game.
		invoked := make([]int64, 0, len(body.ArtifactIDs))
		for _, id := range body.ArtifactIDs {
			if slices.Contains(invoked, id) {
				continue
			}
			asset, err := deps.Q.GetAssetByID(ctx, id)
			if err != nil || asset.GameID != plan.GameID || asset.AssetType != model.AssetArtifact {
				respondErr(w, http.StatusBadRequest, "each artifact_id must be an artifact in this game")
				return
			}
			invoked = append(invoked, id)
		}
		ch.InvokedArtifactIDs = invoked

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}

		// Close the invoke phase so the difficulty we compute is final.
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

		// Log the preparer's scene as their own narration, anchored to the plan
		// row (replaces the old OnResolve pre-roll system message).
		if scene := strings.TrimSpace(body.Scene); scene != "" {
			post, perr := deps.Q.CreatePlayerMessage(ctx, dbgen.CreatePlayerMessageParams{
				GameID:    plan.GameID,
				AuthorID:  &player.ID,
				Body:      scene,
				RowNumber: plan.RowNumber,
				PlanID:    &plan.ID,
			})
			if perr == nil {
				if h, ok := deps.Manager.Get(plan.GameID); ok {
					h.BroadcastEvent(model.EventScenePostCreated, model.ScenePostCreatedPayload{Post: post})
				}
			}
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"roll":    roll,
		})
	}
}

// ── Make Step ───────────────────────────────────────────────────────────────

// chMakeStepHandler handles POST /api/plans/:planId/make-step.
//
// On a made roll the preparer chooses options "equal to your result", one at a
// time, so any surrounding narration can be posted to chat between picks. Each
// step applies its mechanical effect inline (reusing chApplyMarEffect, the same
// helper the mar path uses) and folds the player's optional narration into the
// single action-log entry alongside the mechanical note.
//
// Completion is server-authoritative: MakeBudget (the dice result) is captured
// on the first step, MakeChoicesDone counts submitted steps, and the route
// rejects any step beyond MakeBudget so a stale client can't over-pick.
//
// Request body: {"option": "...", "narration": "...", "asset_id": N, "marginalia_id": M}
func chMakeStepHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanChronicleHistories {
			respondErr(w, http.StatusBadRequest, "make-step is only for Chronicle Histories")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		// The preparer resolves their own plan; a perform_steps demand winner may
		// drive the choice in their place (same actor set as the generic make).
		if player.ID != plan.PreparerID &&
			!makeChoiceAllowedNonPreparer(r.Context(), deps.Q, plan, player) {
			respondErr(w, http.StatusForbidden, "only the plan's preparer can make choices")
			return
		}

		ctx := r.Context()

		// Make steps are only valid on a made roll; capture the result as the budget.
		roll, rollErr := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr != nil || roll.Outcome == nil || *roll.Outcome != makeOutcome {
			respondErr(w, http.StatusConflict, "make-step is only valid when the plan rolled a make")
			return
		}
		if roll.Result == nil || *roll.Result < 1 {
			respondErr(w, http.StatusConflict, "the roll has no result yet")
			return
		}

		var body struct {
			Option       string `json:"option"`
			Narration    string `json:"narration"`
			AssetID      *int64 `json:"asset_id"`
			MarginaliaID *int64 `json:"marginalia_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Option == "" {
			respondErr(w, http.StatusBadRequest, "option is required")
			return
		}
		validChoices := []string{"break_artifact", "invoke_another", "echo_present", "total_control"}
		if !slices.Contains(validChoices, body.Option) {
			respondErr(w, http.StatusBadRequest, fmt.Sprintf("option must be one of: %v", validChoices))
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		ch := resData.EnsureChronicleHistories()

		// Capture the budget once, on the first step, and announce the success.
		// (Chronicle's make path uses make-step, not the generic make-choice
		// route that would otherwise emit this via ApplyChoice.)
		if ch.MakeBudget == 0 {
			ch.MakeBudget = *roll.Result
			chLog(ctx, deps, plan, model.SeverityImportant,
				"Chronicle Histories succeeded — the preparer shapes the scene from history.")
		}
		// Server-authoritative completion: never accept more than the budget.
		if ch.MakeChoicesDone >= ch.MakeBudget {
			respondErr(w, http.StatusConflict, "all make choices have already been submitted")
			return
		}

		// Apply the mechanical effect (break_artifact / invoke_another mutate
		// state and DB; echo_present / total_control are narrative). Reuses the
		// same helper as the mar path.
		logBody, status, msg := chApplyMarEffect(ctx, deps, plan, ch, player.ID, marEffectInput{
			choice: body.Option, assetID: body.AssetID, marginaliaID: body.MarginaliaID,
		})
		if status != 0 {
			respondErr(w, status, msg)
			return
		}

		// Record as a make-path Choice (PlayerID nil) and advance the counter.
		resData.MakeMarChoices = append(resData.MakeMarChoices, Choice{Option: body.Option})
		ch.MakeChoicesDone++

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save make choice", err)
			return
		}

		// Fold the player's narration into the single mechanical log entry.
		if narration := strings.TrimSpace(body.Narration); narration != "" {
			logBody = logBody + " " + narration
		}
		chLog(ctx, deps, plan, model.SeverityDefault, logBody)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":      plan.ID,
			"option":       body.Option,
			"choices_done": ch.MakeChoicesDone,
			"make_budget":  ch.MakeBudget,
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
//     tears the marginalia via breakMarginalia (auto-destroy on the last).
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

// ResolvingWaitees narrows a marred Chronicle Histories to AwaitChronicleChoices:
// every player present when the mar scene began must each submit one option. The
// bar names those who still owe a choice (game players minus the distinct
// submitters in MakeMarChoices). The make path and a fully-chosen mar return
// false and ride the generic plan_resolving case (the preparer completes).
func (chHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	if !planRollIsMar(ctx, q, plan) {
		return model.RowState{}, false
	}
	resData := loadResolutionData(plan.ResolutionData)
	submitted := map[int64]bool{}
	for _, c := range resData.MakeMarChoices {
		if c.PlayerID != nil {
			submitted[*c.PlayerID] = true
		}
	}
	players, err := q.GetPlayersByGame(ctx, plan.GameID)
	if err != nil {
		return model.RowState{}, false
	}
	var pending []int64
	for i := range players {
		if !submitted[players[i].ID] {
			pending = append(pending, players[i].ID)
		}
	}
	if len(pending) == 0 {
		return model.RowState{}, false
	}
	return model.RowState{Kind: model.RowStateAwaitChronicleChoices, ActingPlayerIDs: pending}, true
}
