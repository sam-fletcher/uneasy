package handler

// handler/plan_make_demands.go — Make Demands plan handler (Phase 3d).
//
// Make Demands (power, variable delay) targets another unresolved plan. The
// demand lands on the *same* row as its target and slots in immediately
// before it (taking the target's row_order; the target and later plans on
// that row shift up by one), so the demand resolves first within the row.
// Difficulty is the target plan's baseline plus the demander's power-rank
// deficit vs. the target's preparer.
//
// On a made roll, the demander and target's preparer alternate drafting the
// four demand options — control_leverage, keep_or_change_target, keep_assets,
// perform_steps — in power-rank order (higher-ranked = lower rank number
// picks first). Winners are persisted on the demand plan's
// demand_option_winners column so the target plan's resolution can consult
// them without re-walking the demand.
//
// On a marred roll, the target of the demand may prepare a free counter-
// demand (Stage 5). Until that counter lands (or the target waives it) the
// demand plan is not marked complete.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

const (
	demandEventPrepared       = "demand.prepared"
	demandEventResolved       = "demand.resolved"
	demandEventDraftPick      = "demand.draft_pick"
	demandEventCounterPending = "demand.counter_pending"
	demandEventLeverageSet    = "demand.leverage_set"
	demandEventRetargeted     = "demand.retargeted"
	demandEventCounterPlaced  = "demand.counter_placed"
)

func init() {
	RegisterPlan(model.PlanMakeDemands, mdHandler{})
}

type mdHandler struct{}

func (mdHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryPower, Delay: -1}
}

func (mdHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (*int16, string) {
	if v.TargetPlanID == nil {
		return nil, "make_demands requires target_plan_id"
	}
	target, err := v.Q.GetPlanByID(ctx, *v.TargetPlanID)
	if err != nil {
		return nil, "target plan not found"
	}
	if target.GameID != v.Game.ID {
		return nil, "target plan is not in this game"
	}
	if target.Status == model.PlanResolved || target.Status == model.PlanCancelled {
		return nil, "target plan is already resolved or cancelled"
	}
	if target.Status == model.PlanResolving {
		return nil, "target plan is already resolving — a demand cannot slot in before it"
	}
	if target.PlanType == model.PlanMakeWar {
		return nil, "Make War cannot be the target of a demand"
	}
	if target.PreparerID == v.Player.ID {
		return nil, "you cannot demand against your own plan"
	}
	existing, err := v.Q.GetPlansTargeting(ctx, &target.ID)
	if err != nil {
		return nil, "could not check existing demands"
	}
	for _, d := range existing {
		if d.Status != model.PlanResolved && d.Status != model.PlanCancelled {
			return nil, "another demand already targets that plan"
		}
	}
	if target.RowNumber == nil {
		return nil, "target plan has not been assigned a row yet (its delay reveal is still open)"
	}
	row, _ := gamepkg.DemandPlacement(*target.RowNumber, target.RowOrder)
	return &row, ""
}

func (mdHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	if plan.TargetedPlanID == nil {
		return 0, errors.New("make_demands plan has no targeted plan")
	}
	target, err := q.GetPlanByID(ctx, *plan.TargetedPlanID)
	if err != nil {
		return 0, fmt.Errorf("load target plan: %w", err)
	}
	targetHandler, ok := GetHandler(target.PlanType)
	if !ok {
		return 0, fmt.Errorf("no handler for target plan type %s", target.PlanType)
	}
	targetRes := loadResolutionData(target.ResolutionData)
	targetDiff, err := targetHandler.ComputeDifficulty(ctx, q, &target, &targetRes)
	if err != nil {
		return 0, fmt.Errorf("compute target difficulty: %w", err)
	}
	demanderRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryPower)
	if err != nil {
		return 0, fmt.Errorf("load demander power rank: %w", err)
	}
	targetRank, err := playerRankInCategory(ctx, q, plan.GameID, target.PreparerID, model.CategoryPower)
	if err != nil {
		return 0, fmt.Errorf("load target power rank: %w", err)
	}
	return gamepkg.MakeDemandsDifficulty(targetDiff, demanderRank, targetRank), nil
}

// OnPrepare is a no-op beyond the broadcast: the targeted_plan_id column is
// populated by PreparePlan after the row is created.
func (mdHandler) OnPrepare(_ context.Context, deps *PlanDeps, plan *dbgen.Plan) error {
	broadcastEvent(deps.Manager, plan.GameID, demandEventPrepared, model.PlanPayload{Plan: *plan})
	return nil
}

func (mdHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	diff, err := (mdHandler{}).ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, fmt.Errorf("load game: %w", err)
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, diff, plan.PreparerID)
}

// ApplyChoice records the result via the standard MakeChoice endpoint; the
// draft itself flows through /draft-choice. On a marred demand, the counter-
// demand window opens and is consumed via /counter-demand (Stage 5).
func (mdHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	result string,
) error {
	if h, ok := deps.Manager.Get(plan.GameID); ok {
		h.BroadcastEvent(demandEventResolved, map[string]any{
			"plan_id": plan.ID,
			"result":  result,
		})
		if result == marOutcome {
			h.BroadcastEvent(demandEventCounterPending, map[string]any{
				"plan_id": plan.ID,
			})
		}
	}

	demander := playerDisplayName(ctx, deps.Q, plan.PreparerID)
	targetName := mdTargetPlanLabel(ctx, deps.Q, plan)
	if result == makeOutcome {
		mdLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("%s's demand against %s succeeded — they now draft control of its resolution.",
				demander, targetName))
	} else {
		mdLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("%s's demand against %s was marred — the target may counter-demand.",
				demander, targetName))
	}
	return nil
}

// mdTargetPlanLabel renders a short human label for the plan a demand targets,
// e.g. "Bob's Exchange Courtiers". Falls back gracefully on lookup failure.
func mdTargetPlanLabel(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) string {
	if plan.TargetedPlanID == nil {
		return "their target plan"
	}
	target, err := q.GetPlanByID(ctx, *plan.TargetedPlanID)
	if err != nil {
		return "their target plan"
	}
	return fmt.Sprintf("%s's %s", playerDisplayName(ctx, q, target.PreparerID), planLabel(target.PlanType))
}

// mdPersistDraftWinners builds the option→winner map from the four draft picks,
// persists it on the demand plan (so the target plan's resolution can consult
// it), and emits the draft-complete action-log entry.
func mdPersistDraftWinners(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	picks []gamepkg.DraftChoice,
) error {
	winners := gamepkg.DemandOptionWinners{}
	for _, c := range picks {
		winners[c.Option] = c.PlayerID
	}
	raw, err := json.Marshal(winners)
	if err != nil {
		return fmt.Errorf("encode option winners: %w", err)
	}
	if err := deps.Q.SetDemandOptionWinners(ctx, dbgen.SetDemandOptionWinnersParams{
		ID:                  plan.ID,
		DemandOptionWinners: raw,
	}); err != nil {
		return fmt.Errorf("save option winners: %w", err)
	}
	mdLog(ctx, deps, plan, model.SeverityImportant,
		fmt.Sprintf("Demand draft complete: %s.", mdWinnersSummary(ctx, deps.Q, winners)))
	return nil
}

// mdDemandOptionLabels gives each draft option a short human label for the log.
var mdDemandOptionLabels = map[string]string{
	gamepkg.DemandOptionControlLeverage:    "leverage control",
	gamepkg.DemandOptionKeepOrChangeTarget: "keep/change target",
	gamepkg.DemandOptionKeepAssets:         "keep created assets",
	gamepkg.DemandOptionPerformSteps:       "perform make/mar steps",
}

// mdWinnersSummary renders the drafted option winners as
// "Alice took leverage control & keep created assets; Bob took …".
func mdWinnersSummary(ctx context.Context, q *dbgen.Queries, winners gamepkg.DemandOptionWinners) string {
	byPlayer := map[int64][]string{}
	// Stable option order for readable output.
	order := []string{
		gamepkg.DemandOptionControlLeverage,
		gamepkg.DemandOptionKeepOrChangeTarget,
		gamepkg.DemandOptionKeepAssets,
		gamepkg.DemandOptionPerformSteps,
	}
	var playerOrder []int64
	for _, opt := range order {
		pid, ok := winners[opt]
		if !ok || pid == 0 {
			continue
		}
		if _, seen := byPlayer[pid]; !seen {
			playerOrder = append(playerOrder, pid)
		}
		byPlayer[pid] = append(byPlayer[pid], mdDemandOptionLabels[opt])
	}
	parts := make([]string, 0, len(playerOrder))
	for _, pid := range playerOrder {
		parts = append(parts, fmt.Sprintf("%s took %s",
			playerDisplayName(ctx, q, pid), strings.Join(byPlayer[pid], " & ")))
	}
	return strings.Join(parts, "; ")
}

// mdLog emits a Make Demands action-log entry anchored to the plan's row.
func mdLog(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, severity int32, body string) {
	planID := plan.ID
	EmitSystemPost(ctx, deps.Q, deps.Manager, plan.GameID, "plan.make_demands",
		severity, body, plan.RowNumber, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

func (mdHandler) CanComplete(plan *dbgen.Plan, resData *ResolutionData) error {
	if plan.Result == nil {
		return errors.New("demand has no result yet")
	}
	md := resData.MakeDemands
	switch *plan.Result {
	case makeOutcome:
		if md == nil || len(md.DraftChoices) < 4 {
			n := 0
			if md != nil {
				n = len(md.DraftChoices)
			}
			return fmt.Errorf("draft incomplete: %d of 4 options picked", n)
		}
	case marOutcome:
		if md == nil || !md.CounterDemandPlaced {
			return errors.New("target must place or waive the counter-demand before completing")
		}
	}
	return nil
}

func (mdHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"draft-choice":    mdDraftChoiceHandler(deps),
		"counter-demand":  mdCounterDemandHandler(deps),
		"demand-leverage": mdDemandLeverageHandler(deps),
		"demand-retarget": mdDemandRetargetHandler(deps),
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// mdRollOutcome returns the resolved dice-roll outcome ("make"/"mar") for a
// Make Demands plan, or "" if no roll exists or it hasn't resolved. Used by
// the draft-choice and counter-demand handlers to gate by roll outcome —
// plan.Result isn't written until CompletePlan (which also flips status
// out of 'resolving'), so checking plan.Result here would be unreachable.
func mdRollOutcome(ctx context.Context, q *dbgen.Queries, planID int64) string {
	roll, err := q.GetDiceRollByPlanID(ctx, &planID)
	if err != nil || roll.Outcome == nil {
		return ""
	}
	return *roll.Outcome
}

func validDemandOption(s string) bool {
	switch s {
	case gamepkg.DemandOptionControlLeverage,
		gamepkg.DemandOptionKeepOrChangeTarget,
		gamepkg.DemandOptionKeepAssets,
		gamepkg.DemandOptionPerformSteps:
		return true
	}
	return false
}

// mdDraftPickers returns (firstPicker, secondPicker) by power rank. The
// higher-ranked (lower rank number) player picks first. Power ranks are
// unique per (game, category) via DB constraint, so no tiebreaker is needed.
func mdDraftPickers(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, demanderID, targetPreparerID int64,
) (int64, int64, error) {
	demanderRank, err := playerRankInCategory(ctx, q, gameID, demanderID, model.CategoryPower)
	if err != nil {
		return 0, 0, fmt.Errorf("load demander power rank: %w", err)
	}
	targetRank, err := playerRankInCategory(ctx, q, gameID, targetPreparerID, model.CategoryPower)
	if err != nil {
		return 0, 0, fmt.Errorf("load target power rank: %w", err)
	}
	first, second := gamepkg.DemandDraftPickers(demanderID, targetPreparerID, demanderRank, targetRank)
	return first, second, nil
}
