package handler

// handler/plan_resolution.go — Plan resolution: get, resolve, make-choice,
// and complete — the lifecycle a prepared plan goes through once
// current_row reaches it. See plans.go for creation/listing/eligibility and
// its doc comment for the full lifecycle narrative.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// ── GetPlan ───────────────────────────────────────────────────────────────────

// GetPlan handles GET /api/plans/:planId.
func GetPlan(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, s.Q)
		if !ok {
			return
		}
		sanitizeLiaiseKeptSecretsForViewer(plan, player.ID)

		resData := loadResolutionData(plan.ResolutionData)

		var difficulty int16
		if h, supported := GetHandler(plan.PlanType); supported {
			difficulty, _ = h.ComputeDifficulty(r.Context(), s.Q, plan, &resData)
		}

		respond(w, http.StatusOK, map[string]any{
			"plan":            plan,
			"difficulty":      difficulty,
			"resolution_data": resData,
		})
	}
}

// ── ResolvePlan ───────────────────────────────────────────────────────────────

// kickoffPlanResolution flips a pending plan to 'resolving', broadcasts the
// plan.resolving event, and invokes the plan handler's OnResolve hook (which
// usually creates a dice roll, but for some plan types performs other
// initialization or even fully resolves the plan, e.g. Make War).
//
// Caller responsibilities:
//   - plan must be in 'pending' status with row_number == game.current_row.
//     Callers that come from a freshly-computed RowState (kind=plan_pending)
//     satisfy this by construction.
//   - The caller is responsible for any row_state broadcast that should
//     follow. Most callers go through broadcastRowState which handles both
//     the kickoff and the final broadcast in a single helper call.
//
// Returns the dice roll if one was created, or nil. Errors are returned
// verbatim — the auto-kickoff path logs and leaves the plan pending; the
// HTTP endpoint surfaces them as 500.
func kickoffPlanResolution(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	plan *dbgen.Plan,
) (*dbgen.DiceRoll, error) {
	h, supported := GetHandler(plan.PlanType)
	if !supported {
		return nil, fmt.Errorf("no handler for plan type %q", plan.PlanType)
	}

	if err := q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID:     plan.ID,
		Status: model.PlanResolving,
	}); err != nil {
		return nil, fmt.Errorf("set plan status: %w", err)
	}
	// Refresh the local copy so the broadcast payload reflects the new status.
	plan.Status = model.PlanResolving

	if hub, hasHub := manager.Get(plan.GameID); hasHub {
		hub.BroadcastEvent(model.EventPlanResolving, model.PlanPayload{Plan: *plan})
	}
	EmitPlanResolving(ctx, q, manager, *plan)
	maybeOpenPlanScene(ctx, q, manager, plan)

	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: manager}
	return h.OnResolve(ctx, deps, plan)
}

// ResolvePlan handles POST /api/plans/:planId/resolve.
//
// Normally the kickoff happens automatically inside advanceAndBroadcastRowState
// whenever the table enters kind=plan_pending. This endpoint remains as a
// retry/escape hatch for the rare case where OnResolve fails — the row state
// stays pending and the focus player can re-trigger via this endpoint.
func ResolvePlan(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, plan, ok := requirePlanPreparer(w, r, s.Q)
		if !ok {
			return
		}
		if plan.Status != model.PlanPending {
			respondErr(w, http.StatusConflict, "plan is not in pending status")
			return
		}
		if plan.RowNumber == nil || *plan.RowNumber != game.CurrentRow {
			respondErr(w, http.StatusConflict, "plan is not scheduled for the current row")
			return
		}

		ctx := r.Context()
		roll, err := kickoffPlanResolution(ctx, s.Q, manager, plan)
		if err != nil {
			respondInternalErr(w, r, "could not begin resolution", err)
			return
		}
		broadcastRowState(ctx, s.Q, manager, game.ID)

		resp := map[string]any{"plan_id": plan.ID}
		if roll != nil {
			resp["roll"] = roll
		}
		respond(w, http.StatusOK, resp)
	}
}

// ── MakeChoice ────────────────────────────────────────────────────────────────

// MakeChoice handles POST /api/plans/:planId/make-choice.
//
// Called after the dice roll resolves. Records the make/mar option choices
// and executes any server-side mechanical effects via h.ApplyChoice().
//
// Request body:
//
//	{
//	  "choices": ["legal"],  // option key strings (plan-specific)
//	  "result": "make"       // "make" or "mar" — must match the roll outcome
//	}
func MakeChoice(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, s.Q)
		if !ok {
			return
		}
		game, err := s.Q.GetGameByID(r.Context(), plan.GameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}
		// The plan's resolution actor drives make-choice — normally the preparer,
		// but a Make Demands perform_steps win transfers that to the winner (and
		// locks out the preparer). A perform_steps winner does NOT inherit a
		// target-driven mar, and a few plans hand the mar to their target/victim
		// regardless — makeChoiceAuthorized encodes both carve-outs.
		if !makeChoiceAuthorized(r.Context(), s.Q, plan, player) {
			respondErr(w, http.StatusForbidden, "only the plan's preparer can do this")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			Choices []string `json:"choices"`
			Result  string   `json:"result"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Result != makeOutcome && body.Result != marOutcome {
			respondErr(w, http.StatusBadRequest, "result must be 'make' or 'mar'")
			return
		}

		// Spread Rumors' "take asset" is consent-gated and must go through
		// request-take-consent (which names the specific assets and asks the
		// victim). A direct make-choice carrying it would commit the other
		// choices without ever obtaining consent, so reject it here.
		if plan.PlanType == model.PlanSpreadRumors && slices.Contains(body.Choices, "take_asset") {
			respondErr(w, http.StatusBadRequest,
				"take_asset requires the target's consent; use request-take-consent")
			return
		}

		ctx := r.Context()

		// Verify result matches the linked dice roll's outcome (if one exists).
		roll, rollErr := s.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && roll.Outcome != nil && *roll.Outcome != body.Result {
			respondErr(w, http.StatusConflict,
				fmt.Sprintf("result '%s' does not match roll outcome '%s'", body.Result, *roll.Outcome))
			return
		}

		h, supported := GetHandler(plan.PlanType)
		if !supported {
			respondErr(w, http.StatusInternalServerError, "no handler for this plan type")
			return
		}

		var rollPtr *dbgen.DiceRoll
		if rollErr == nil {
			rollPtr = &roll
		}
		if !enforceChoiceBudget(w, h, rollPtr, body.Result, body.Choices) {
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		resData.MakeMarChoices = make([]Choice, len(body.Choices))
		for i, opt := range body.Choices {
			resData.MakeMarChoices[i] = Choice{Option: opt}
		}

		deps := &PlanDeps{Store: s, Manager: manager}
		if err := h.ApplyChoice(ctx, deps, plan, &resData, body.Choices, body.Result); err != nil {
			respondInternalErr(w, r, "could not apply plan effects", err)
			return
		}

		if err := saveResolutionData(ctx, s.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save choices", err)
			return
		}

		// When the choice leaves nothing further to decide, plans that opt into
		// auto-completion (Exchange Courtiers) resolve themselves here rather than
		// waiting on a separate "Complete plan" click — finalize already fans out
		// the resolved + row-state events, so other clients refetch.
		resolved, err := maybeAutoComplete(ctx, s.Q, manager, h, plan, &resData, body.Result)
		if err != nil {
			respondInternalErr(w, r, "could not auto-complete plan", err)
			return
		}

		// make-choice can mutate shared resolution state that other players are
		// watching (e.g. the duel's phase flips to 'done' once the winner claims
		// stakes). Only the acting client gets this HTTP response, so nudge
		// everyone else to refetch the plan — otherwise their panel stays on the
		// pre-choice "waiting" state until a manual page refresh. Skip when we
		// auto-resolved: the resolved broadcast above already triggers a refetch.
		if !resolved {
			broadcastEvent(manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
				PlanID: plan.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":              plan.ID,
			"choices":              body.Choices,
			"result":               body.Result,
			"resolved":             resolved,
			"messy_break_required": resData.ExchangeCourtiers != nil && resData.ExchangeCourtiers.MessyBreakRequired,
		})
	}
}

// ── CompletePlan ──────────────────────────────────────────────────────────────

// CompletePlan handles POST /api/plans/:planId/complete.
//
// Marks the plan as resolved. Calls h.CanComplete() to check for any pending
// prerequisites (e.g. EC messy break). The result is taken from the linked
// dice roll's outcome; if no roll exists the result is read from the plan's
// stored result field (e.g. EC fair trade accept path).
func CompletePlan(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, plan, ok := requirePlanPreparer(w, r, s.Q)
		if !ok {
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		h, supported := GetHandler(plan.PlanType)
		if !supported {
			respondErr(w, http.StatusInternalServerError, "no handler for this plan type")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		if err := h.CanComplete(plan, &resData); err != nil {
			respondErr(w, http.StatusConflict, "cannot complete plan: "+err.Error())
			return
		}

		// Determine result from roll outcome or existing plan result (fair trade).
		resultStr := planResultString(ctx, s.Q, plan)
		if resultStr == "" {
			respondErr(w, http.StatusConflict, "cannot complete plan: no roll outcome and no stored result")
			return
		}

		if err := finalizePlanResolution(ctx, s.Q, manager, plan, resultStr); err != nil {
			respondInternalErr(w, r, "could not complete plan", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"result":  resultStr,
		})
	}
}

// planResultString resolves a plan's outcome from its linked dice roll, falling
// back to the stored plan.Result (set by roll-less paths like the EC fair-trade
// accept). Returns "" when neither is available.
func planResultString(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) string {
	if roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID); err == nil && roll.Outcome != nil {
		return *roll.Outcome
	}
	if plan.Result != nil {
		return *plan.Result
	}
	return ""
}

// finalizePlanResolution marks a resolving plan resolved and fans out the
// resolution events — the shared tail of the explicit CompletePlan endpoint and
// the auto-completion paths (see maybeAutoComplete). Assumes CanComplete has
// already passed and resultStr is non-empty ("make"/"mar"/"cancelled").
func finalizePlanResolution(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan *dbgen.Plan, resultStr string,
) error {
	if err := q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
		ID:     plan.ID,
		Result: &resultStr,
	}); err != nil {
		return err
	}
	broadcastEvent(manager, plan.GameID, model.EventPlanResolved, model.PlanResolvedPayload{
		PlanID: plan.ID,
		Result: resultStr,
	})
	EmitPlanResolved(ctx, q, manager, *plan, resultStr)
	broadcastRowState(ctx, q, manager, plan.GameID)
	return nil
}

// maybeAutoComplete resolves a plan in place when its handler opts into
// auto-completion (AutoCompleter) and no sub-step remains owed (CanComplete).
// It is called at a plan's terminal transitions — the make/mar choice and any
// post-choice sub-step route — so a plan whose ending view holds no decision or
// information (Exchange Courtiers) resolves itself instead of stranding the
// preparer on a no-op "Complete" click. Returns true if it resolved the plan;
// a no-op (false, nil) for handlers that don't opt in or still owe a step.
func maybeAutoComplete(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager,
	h PlanHandler, plan *dbgen.Plan, resData *ResolutionData, resultStr string,
) (bool, error) {
	ac, ok := h.(AutoCompleter)
	if !ok || !ac.AutoCompleteAfterChoice(plan, resData) {
		return false, nil
	}
	if resultStr == "" {
		return false, nil
	}
	// A still-owed sub-step (CanComplete error) just means "not ready to resolve
	// yet", not a failure — the actor finishes via the plan's sub-step route,
	// which calls back here once it lands on the terminal phase.
	if h.CanComplete(plan, resData) != nil {
		return false, nil //nolint:nilerr // not-ready sub-step, not an error
	}
	if err := finalizePlanResolution(ctx, q, manager, plan, resultStr); err != nil {
		return false, err
	}
	return true, nil
}

// applyAutoChoiceOnRoll records a resolved roll's outcome automatically for
// plans that opt in via AutoApplyChoiceOnRoll — those whose post-roll
// make-choice carries no decision (the outcome is fixed by the dice). It mirrors
// the MakeChoice handler's ApplyChoice → save → maybe-auto-complete tail, minus
// the option-pick budget (there are no options). It is a no-op for rolls not
// tied to such a plan, and — because the handler's ApplyChoice is idempotent —
// safe to call even if the outcome was already applied.
//
// Called from finalizeRoll so the result is recorded server-side the moment the
// dice land, rather than waiting for the actor to open the page and click "pass"
// (which would stall the async play-by-post sub-flow on a no-op gate).
func applyAutoChoiceOnRoll(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager, roll *dbgen.DiceRoll,
) error {
	if roll.PlanID == nil || roll.Outcome == nil {
		return nil
	}
	plan, err := q.GetPlanByID(ctx, *roll.PlanID)
	if err != nil {
		return err
	}
	if plan.Status != model.PlanResolving {
		return nil
	}
	h, ok := GetHandler(plan.PlanType)
	if !ok {
		return nil
	}
	applier, ok := h.(AutoApplyChoiceOnRoll)
	if !ok || !applier.AutoApplyChoiceOnRoll() {
		return nil
	}

	resData := loadResolutionData(plan.ResolutionData)
	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: manager}
	if err := h.ApplyChoice(ctx, deps, &plan, &resData, nil, *roll.Outcome); err != nil {
		return err
	}
	if err := saveResolutionData(ctx, q, plan.ID, resData); err != nil {
		return err
	}
	// A decision-free outcome can leave the plan terminally finished (e.g. a plan
	// with no post-choice sub-flow); auto-complete if so. Plans with sub-steps
	// (Propose Decree) fail CanComplete here, so this is a safe no-op for them.
	if _, err := maybeAutoComplete(ctx, q, manager, h, &plan, &resData, *roll.Outcome); err != nil {
		return err
	}
	return nil
}
