package handler

// handler/plan_access.go — Access and authorization helpers shared by the
// common plan lifecycle handlers (plans.go) and the per-plan extra-route
// handlers (plan_*.go). These resolve the plan from the URL, verify the caller
// belongs to the game / is the preparer, and guard plan type and status.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// requirePlanAccess parses planId and verifies the caller belongs to the plan's game.
func requirePlanAccess(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
) (*dbgen.Plan, *dbgen.Player, bool) {
	planID, err := strconv.ParseInt(chi.URLParam(r, "planId"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid plan id")
		return nil, nil, false
	}
	plan, err := q.GetPlanByID(r.Context(), planID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "plan not found")
		return nil, nil, false
	}
	player, ok := requirePlayerInGame(w, r, q, plan.GameID)
	if !ok {
		return nil, nil, false
	}
	return &plan, player, true
}

// requirePlanPreparer returns the game and plan, verifying the caller is the
// plan's preparer and the game is in main_event phase. Per the rules each plan
// is resolved by its own preparer (the "actor"), not the current focus player —
// the focus player only sets scenes and prepares plans.
func requirePlanPreparer(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
) (*dbgen.Game, *dbgen.Plan, bool) {
	plan, player, ok := requirePlanAccess(w, r, q)
	if !ok {
		return nil, nil, false
	}
	game, err := q.GetGameByID(r.Context(), plan.GameID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "table not found")
		return nil, nil, false
	}
	if game.Phase != model.PhaseMainEvent {
		respondErr(w, http.StatusConflict, "game is not in the main event phase")
		return nil, nil, false
	}
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the plan's preparer can resolve it")
		return nil, nil, false
	}
	return &game, plan, true
}

// requirePlanType writes a 400 and returns false if the plan isn't of the
// expected type. Used by plan-specific extra-route handlers to guard against
// a route being called with the wrong plan ID.
func requirePlanType(w http.ResponseWriter, plan *dbgen.Plan, want model.PlanType) bool {
	if plan.PlanType != want {
		respondErr(w, http.StatusBadRequest, "route is only for "+string(want)+" plans")
		return false
	}
	return true
}

// requirePlanResolving writes a 409 and returns false if the plan isn't in
// the resolving status. Several plans' extra routes fire only during the
// resolving phase.
func requirePlanResolving(w http.ResponseWriter, plan *dbgen.Plan) bool {
	if plan.Status != model.PlanResolving {
		respondErr(w, http.StatusConflict, "plan is not in resolving status")
		return false
	}
	return true
}

// actsForPreparer reports whether playerID may drive plan's make/mar resolution
// steps as the preparer-equivalent. Authority is the preparer's by default, but
// a resolved, made Make Demands "perform_steps" win TRANSFERS it to the winner:
// when the demander wins, they perform the steps and the preparer is locked out;
// when the preparer wins (or there's no demand) the preparer keeps it. This is
// the single source of truth for "who resolves this plan", shared by the
// standard make-choice path and the per-plan make-resolution routes.
func actsForPreparer(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, playerID int64) bool {
	if winner := performStepsWinner(ctx, q, plan); winner != 0 {
		return playerID == winner
	}
	return playerID == plan.PreparerID
}

// requireResolutionActor is the route guard form of actsForPreparer: it writes a
// 403 and returns false when the caller may not drive this plan's resolution.
// Per-plan resolution routes use it in place of a bare preparer check so a
// perform_steps demand winner is honored uniformly.
func requireResolutionActor(
	w http.ResponseWriter,
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	playerID int64,
) bool {
	if !actsForPreparer(ctx, q, plan, playerID) {
		respondErr(w, http.StatusForbidden, "only the plan's preparer (or a demand's perform-steps winner) can do this")
		return false
	}
	return true
}

// marChoiceTargetRole returns true if the caller is a non-preparer who the plan
// text hands a make-choice on a *mar* outcome — independent of any demand. The
// mar of these plans is target-driven by design, so the target/victim drives it
// (not the preparer, and not a perform_steps winner):
//
//   - Propose Duel target — picks which staked assets to claim from the preparer.
//   - Exchange Courtiers target — chooses fair_trade / riposte / forfeit.
//   - Spread Rumors target-asset owner — drives the counter-rumor options.
func marChoiceTargetRole(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	player *dbgen.Player,
) bool {
	if plan.PlanType == model.PlanProposeDuel &&
		plan.TargetPlayerID != nil && *plan.TargetPlayerID == player.ID &&
		planRollIsMar(ctx, q, plan) {
		return true
	}
	if plan.PlanType == model.PlanExchangeCourtiers &&
		plan.TargetPlayerID != nil && *plan.TargetPlayerID == player.ID &&
		planRollIsMar(ctx, q, plan) {
		return true
	}
	if plan.PlanType == model.PlanSpreadRumors && plan.TargetAssetID != nil {
		if asset, err := q.GetAssetByID(ctx, *plan.TargetAssetID); err == nil &&
			asset.OwnerID == player.ID && planRollIsMar(ctx, q, plan) {
			return true
		}
	}
	return false
}

// enforceChoiceBudget writes a 422 and returns false if the submitted choices
// violate the plan's per-result option rules. Handlers opt in via ChoiceLimiter
// (a simple count cap) or ChoiceValidator (full validation — level cap +
// single-select, e.g. Exchange Courtiers); ChoiceValidator wins if both exist.
// Rules are only applied when the roll's result is internally consistent with
// the claimed outcome (make ⇒ result ≥ difficulty), so a forced/legacy roll
// carrying a placeholder result never trips a spurious limit. Returns true
// (proceed) when there's no limiter/validator, no usable roll, or the rules pass.
func enforceChoiceBudget(
	w http.ResponseWriter,
	h PlanHandler,
	roll *dbgen.DiceRoll,
	result string,
	choices []string,
) bool {
	if roll == nil || roll.Result == nil {
		return true
	}
	diff := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		diff = *roll.AdjustedDifficulty
	}
	res := *roll.Result
	consistent := (result == makeOutcome && res >= diff) || (result == marOutcome && res < diff)
	if !consistent {
		return true
	}
	// A full validator (level cap + single-select for Exchange Courtiers) takes
	// precedence over the simple count cap; handlers implement one or the other.
	if v, ok := h.(ChoiceValidator); ok {
		if err := v.ValidateChoices(result, res, diff, choices); err != nil {
			respondErr(w, http.StatusUnprocessableEntity, err.Error())
			return false
		}
		return true
	}
	lim, ok := h.(ChoiceLimiter)
	if !ok {
		return true
	}
	maxChoices := lim.MaxChoices(result, res, diff)
	if maxChoices >= 0 && len(choices) > maxChoices {
		respondErr(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("you may choose at most %d option(s) for this %s result", maxChoices, result))
		return false
	}
	return true
}

// planRollIsMar returns true if plan has a resolved dice roll whose outcome
// is mar.
func planRollIsMar(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) bool {
	roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil {
		return false
	}
	return roll.Outcome != nil && *roll.Outcome == marOutcome
}

// requirePlanForExtraRoute combines requirePlanAccess + requirePlanType +
// requirePlanResolving — the preamble of most plan extra-route handlers.
// Returns the plan and caller's player on success; writes the appropriate
// HTTP error and returns false otherwise.
func requirePlanForExtraRoute(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	wantType model.PlanType,
) (*dbgen.Plan, *dbgen.Player, bool) {
	plan, player, ok := requirePlanAccess(w, r, q)
	if !ok {
		return nil, nil, false
	}
	if !requirePlanType(w, plan, wantType) {
		return nil, nil, false
	}
	if !requirePlanResolving(w, plan) {
		return nil, nil, false
	}
	return plan, player, true
}
