package handler

// handler/plan_resolution.go — Plan resolution: get, resolve, make-choice,
// and complete — the lifecycle a prepared plan goes through once
// current_row reaches it. See plans.go for creation/listing/eligibility and
// its doc comment for the full lifecycle narrative.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/go-chi/chi/v5"

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
// whenever the table enters kind=plan_pending. This endpoint remains as a retry
// for the rare case where OnResolve fails — the row state stays pending and the
// plan's PREPARER can re-trigger it.
//
// Not the focus player: per the rules each plan is resolved by whoever prepared
// it, and the focus player only sets scenes and prepares plans. requirePlanPreparer
// (plan_access.go) is the single guard for that split — the caller is retrying
// their own plan, not pushing someone else's along.
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

		roll, rollErr := s.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		var rollPtr *dbgen.DiceRoll
		if rollErr == nil {
			rollPtr = &roll
		}
		if !checkChoiceMatchesRoll(w, rollPtr, body.Result) {
			return
		}

		h, supported := GetHandler(plan.PlanType)
		if !supported {
			respondErr(w, http.StatusInternalServerError, "no handler for this plan type")
			return
		}

		if !enforceChoiceBudget(w, h, rollPtr, body.Result, body.Choices) {
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		if !checkChoiceGate(w, r, s.Q, h, plan, player, &resData, rollPtr, body.Result) {
			return
		}
		resData.ChoiceSubmitted = true
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

// checkChoiceMatchesRoll refuses a make-choice the plan's roll cannot back: the
// roll is still in the air (deciding, voting, leveraging — it has no outcome yet),
// or it settled the other way. roll is nil for a plan with no roll, which is
// unconstrained here; its own ChoiceGate owns that case.
//
// The still-open refusal is for every plan, not an opt-in: with no outcome to
// compare against the mismatch check was silently skipped, so a stale or
// hand-built client could commit a mar (and its effects) — or a make — ahead of
// the dice.
func checkChoiceMatchesRoll(w http.ResponseWriter, roll *dbgen.DiceRoll, result string) bool {
	if roll == nil {
		return true
	}
	if roll.Outcome == nil {
		respondErr(w, http.StatusConflict, errDiceNotSettled.Error())
		return false
	}
	if *roll.Outcome != result {
		respondErr(w, http.StatusConflict,
			fmt.Sprintf("result '%s' does not match roll outcome '%s'", result, *roll.Outcome))
		return false
	}
	return true
}

// checkChoiceGate consults the plan's optional ChoiceGate precondition and writes
// the refusal — its own status when it is a *httpError, else 409 — returning false.
func checkChoiceGate(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	h PlanHandler,
	plan *dbgen.Plan,
	player *dbgen.Player,
	resData *ResolutionData,
	roll *dbgen.DiceRoll,
	result string,
) bool {
	// A forced outcome applied itself when the dice landed (ForcedChoicer); a
	// second submission would apply its effects again.
	if resData.ChoiceAutoApplied {
		respondErr(w, http.StatusConflict, "this outcome was applied automatically — it has nothing left to choose")
		return false
	}
	gate, ok := h.(ChoiceGate)
	if !ok {
		return true
	}
	gErr := gate.CheckChoice(r.Context(), q, plan, player, resData, roll, result)
	if gErr == nil {
		return true
	}
	if _, isHTTP := errors.AsType[*httpError](gErr); !isHTTP {
		gErr = httpErr(http.StatusConflict, gErr.Error())
	}
	respondHTTPErr(w, r, gErr)
	return false
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
	if err := resolvePlan(ctx, q, manager, plan, resultStr); err != nil {
		return err
	}
	broadcastRowState(ctx, q, manager, plan.GameID)
	return nil
}

// resolvePlan is the one way a plan closes with a result: it stores the result,
// reloads the plan into *plan, and announces the resolution. Every plan that
// resolves itself — the shared completion path, Make War's one-step resolve,
// the liaison, the festivity, a fizzled re-aim — goes through it, so the
// stored result, the WebSocket event and the action-log line can't drift apart.
//
// result is "make", "mar", or fizzledOutcome. A fizzled plan keeps a NULL
// result column (the column only admits make/mar, and no roll was made).
func resolvePlan(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan *dbgen.Plan, result string,
) error {
	var stored *string
	if result != fizzledOutcome {
		stored = &result
	}
	if err := q.SetPlanResult(ctx, dbgen.SetPlanResultParams{ID: plan.ID, Result: stored}); err != nil {
		return err
	}
	// The result is stored, so the plan is resolved whatever happens next: a
	// failed reload must not skip the announcement, or the table would hold a
	// resolved plan nobody was told about. Announce with the plan as we have it.
	refreshed, err := q.GetPlanByID(ctx, plan.ID)
	if err != nil {
		loggerFromContext(ctx).WarnContext(ctx,
			"resolved plan could not be reloaded; announcing the stale copy",
			"plan_id", plan.ID, "err", err)
		refreshed = *plan
	}
	*plan = refreshed
	announcePlanResolved(ctx, q, manager, refreshed, result)
	return nil
}

// announcePlanResolved tells the clients and the action log that a plan has
// resolved (or, for a fall-through, been cancelled). resolvePlan is the usual
// way in; callers that store the result themselves — Exchange Courtiers,
// which logs its two asset transfers first — or never set one (a cancelled
// plan) call this directly. The client event carries "" for a fizzled plan, to
// match its NULL result column, so a live client and a reload agree.
func announcePlanResolved(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan dbgen.Plan, result string,
) {
	eventResult := result
	if result == fizzledOutcome {
		eventResult = ""
	}
	broadcastEvent(manager, plan.GameID, model.EventPlanResolved, model.PlanResolvedPayload{
		PlanID: plan.ID,
		Result: eventResult,
	})
	EmitPlanResolved(ctx, q, manager, plan, result)
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

// autoCompleteIfDone is the tail of a plan's terminal sub-step route: it reloads
// the plan (the route has just saved resolution_data) and resolves it when its
// handler opts into auto-completion and nothing is still owed. A no-op for a plan
// that isn't resolving, has no roll outcome or stored result yet, or still owes a
// step.
func autoCompleteIfDone(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, planID int64) error {
	plan, err := q.GetPlanByID(ctx, planID)
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
	resData := loadResolutionData(plan.ResolutionData)
	// A plan that has not had its make/mar choice made owes more than its sub-steps
	// (CanComplete passes vacuously before any pick is committed), so never resolve
	// it on the strength of a pre-roll or pre-choice route.
	// (A plan that records its outcome itself when the dice land has no choice to
	// wait for; its own CanComplete is the whole gate.)
	if _, selfRecording := h.(AutoApplyChoiceOnRoll); !selfRecording && !resData.ChoicesCommitted() {
		return nil
	}
	_, err = maybeAutoComplete(ctx, q, manager, h, &plan, &resData, planResultString(ctx, q, &plan))
	return err
}

// withAutoComplete wraps a plan's extra routes so that a route which succeeded and
// left nothing owed resolves the plan on the spot — for the plans (Spread
// Propaganda, Spread Rumors, Seek Answers) whose every post-choice step is a
// sub-flow route, so "the last step landed" is not one place but any of them. The
// route's own response is written first; the resolution reaches everyone by the
// resolved broadcast and the plan's "resolved" line. Plans with an embedded scene
// do NOT use this: they stay open until the players have finished playing it out.
func withAutoComplete(deps *PlanDeps, routes map[string]http.HandlerFunc) map[string]http.HandlerFunc {
	out := make(map[string]http.HandlerFunc, len(routes))
	for name, next := range routes {
		out[name] = func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next(rec, r)
			if rec.status >= http.StatusMultipleChoices {
				return
			}
			planID, err := strconv.ParseInt(chi.URLParam(r, "planId"), 10, 64)
			if err != nil {
				return
			}
			if err := autoCompleteIfDone(r.Context(), deps.Q, deps.Manager, planID); err != nil {
				loggerFromContext(r.Context()).ErrorContext(r.Context(), "plan auto-complete failed",
					"plan_id", planID, "err", err)
			}
		}
	}
	return out
}

// statusRecorder remembers the status a handler wrote, so a wrapper can tell a
// route that succeeded from one that was refused.
type statusRecorder struct {
	http.ResponseWriter

	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// applyForcedChoice applies a ForcedChoicer plan's only legal outcome. It
// reports applied=false (nothing done) when the plan doesn't opt in or the actor
// has a real decision at this margin. The picks are recorded as the committed
// choices, exactly as make-choice would have, plus the ChoiceAutoApplied marker;
// resData is the caller's copy and is persisted by the caller.
func applyForcedChoice(
	ctx context.Context, deps *PlanDeps, h PlanHandler, plan *dbgen.Plan, resData *ResolutionData, roll *dbgen.DiceRoll,
) (applied bool, err error) {
	fc, ok := h.(ForcedChoicer)
	if !ok || roll.Result == nil || roll.Outcome == nil {
		return false, nil
	}
	difficulty := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		difficulty = *roll.AdjustedDifficulty
	}
	choices, forced := fc.ForcedChoice(*roll.Outcome, *roll.Result, difficulty)
	if !forced {
		return false, nil
	}
	resData.ChoiceAutoApplied = true
	resData.MakeMarChoices = make([]Choice, len(choices))
	for i, c := range choices {
		resData.MakeMarChoices[i] = Choice{Option: c}
	}
	return true, h.ApplyChoice(ctx, deps, plan, resData, choices, *roll.Outcome)
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
	resData := loadResolutionData(plan.ResolutionData)
	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: manager}

	if applier, ok := h.(AutoApplyChoiceOnRoll); ok && applier.AutoApplyChoiceOnRoll() {
		if err := h.ApplyChoice(ctx, deps, &plan, &resData, nil, *roll.Outcome); err != nil {
			return err
		}
	} else if applied, err := applyForcedChoice(ctx, deps, h, &plan, &resData, roll); err != nil {
		return err
	} else if !applied {
		return nil
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
