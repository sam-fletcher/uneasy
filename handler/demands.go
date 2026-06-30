package handler

// handler/demands.go — DB-backed Make Demands lookups.
//
// These query Postgres (GetPlansTargeting) so they live in the imperative
// shell, not game/. The pure Make Demands helpers (difficulty, placement,
// draft pickers, the DemandOptionWinners type and option-key constants) stay
// in game/demands.go. Relocated here to keep game/ free of dbgen.

import (
	"context"
	"encoding/json"
	"fmt"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// AssetRecipientForPlan returns the player who should receive an asset that
// would otherwise be awarded to plan.PreparerID during this plan's
// resolution. If a resolved, made Make Demands plan targets this plan and
// its keep_assets winner is set, that winner is returned; otherwise the
// plan's own preparer.
//
// Safe to call for any plan — returns plan.PreparerID for plans with no
// outstanding demand against them.
func AssetRecipientForPlan(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
) (int64, error) {
	_, winners, err := DemandWinnersForTargetPlan(ctx, q, plan)
	if err != nil {
		return 0, err
	}
	if winner, ok := winners[game.DemandOptionKeepAssets]; ok && winner != 0 {
		return winner, nil
	}
	return plan.PreparerID, nil
}

// performStepsWinner returns the player who holds the perform_steps option of a
// resolved, made Make Demands against plan — i.e. who drives this plan's make/mar
// resolution steps in the preparer's stead. Returns 0 when there is no such
// demand (so the preparer performs their own steps as usual). The winner may
// itself be the preparer (if they won the draft), in which case authority is
// unchanged.
func performStepsWinner(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) int64 {
	_, winners, err := DemandWinnersForTargetPlan(ctx, q, plan)
	if err != nil || winners == nil {
		return 0
	}
	return winners[game.DemandOptionPerformSteps]
}

// pendingPerformStepsChooser returns the "perform_steps" demand winner who owes
// the make/mar choice on a plan targeted by a made Make Demands, while that
// choice is still outstanding — and ok=false otherwise.
//
// When a demand's perform_steps option is won by someone other than the target
// plan's preparer, that winner (not the preparer) drives the target plan's
// make-choice. During the post-roll window before they submit, the generic
// plan_resolving case would otherwise name the preparer; this lets it name the
// actual chooser instead. Once they submit (MakeMarChoices populated) the
// preparer completes, so this returns ok=false and the bar falls back to them.
func pendingPerformStepsChooser(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (int64, bool) {
	winnerID := performStepsWinner(ctx, q, plan)
	if winnerID == 0 || winnerID == plan.PreparerID {
		return 0, false
	}
	// Only once the roll has resolved (the chooser can't act before then) and
	// only while the choice is still outstanding.
	roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil || roll.Outcome == nil {
		return 0, false
	}
	if len(loadResolutionData(plan.ResolutionData).MakeMarChoices) > 0 {
		return 0, false
	}
	return winnerID, true
}

// pendingControlLeverageChooser returns the "control_leverage" demand winner who
// still owes the leverage decision on a plan targeted by a made Make Demands,
// while that decision is still outstanding — and 0 otherwise.
//
// When a demand's control_leverage option is won by someone other than the
// target plan's preparer, that winner (not the preparer) decides how many of the
// preparer's own assets are leveraged onto the target plan's roll — including
// none, to guarantee the roll's failure. Because "leverage none" leaves zero
// demand-leveraged dice (indistinguishable from "hasn't acted yet"), the winner
// must explicitly finalize; until they do, they block the roll. This names them
// so the pre-roll wait isn't mis-attributed to the preparer, mirroring the
// post-roll pendingPerformStepsChooser handoff.
//
// Returns 0 when: there is no such demand; the winner is the preparer (no
// handoff — they leverage their own assets directly); they have already
// finalized; or the target plan's roll is no longer open (the window has
// closed).
func pendingControlLeverageChooser(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) int64 {
	_, winners, err := DemandWinnersForTargetPlan(ctx, q, plan)
	if err != nil || winners == nil {
		return 0
	}
	winnerID := winners[game.DemandOptionControlLeverage]
	if winnerID == 0 || winnerID == plan.PreparerID {
		return 0
	}
	if loadResolutionData(plan.ResolutionData).DemandLeverageFinalized {
		return 0
	}
	// Only while the target plan's roll is still open: the winner can act only
	// during the leverage window, and once the roll resolves the gate is moot.
	roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil || !rollIsOpen(&roll) {
		return 0
	}
	return winnerID
}

// DemandWinnersForTargetPlan returns the resolved made demand (if any) that
// targets the given plan, along with its decoded option-winners map. Returns
// (nil, nil, nil) if no such demand exists. Used by target-plan integration
// paths (leverage, retarget, perform-steps) to check who — if anyone — has
// won a given demand option against this plan.
func DemandWinnersForTargetPlan(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
) (*dbgen.Plan, game.DemandOptionWinners, error) {
	demands, err := q.GetPlansTargeting(ctx, &plan.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("look up demands targeting plan %d: %w", plan.ID, err)
	}
	for i := range demands {
		d := demands[i]
		if d.Status != model.PlanResolved {
			continue
		}
		if d.Result == nil || *d.Result != makeOutcome {
			continue
		}
		if len(d.DemandOptionWinners) == 0 {
			continue
		}
		var winners game.DemandOptionWinners
		if err := json.Unmarshal(d.DemandOptionWinners, &winners); err != nil {
			return nil, nil, fmt.Errorf("decode demand_option_winners for plan %d: %w", d.ID, err)
		}
		return &d, winners, nil
	}
	return nil, nil, nil
}
