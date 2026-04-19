// Package game — Make Demands pure helpers.
//
// Draft option keys are the four mechanical sub-choices a demand's make-roll
// drafts between the demander and the target plan's preparer. They are
// persisted on the *demand* plan row in demand_option_winners (JSONB) so the
// target plan's resolution path can consult them without re-walking the
// demand plan.
package game

import (
	"context"
	"encoding/json"
	"fmt"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// Draft option keys for Make Demands.
const (
	DemandOptionControlLeverage    = "control_leverage"
	DemandOptionKeepOrChangeTarget = "keep_or_change_target"
	DemandOptionKeepAssets         = "keep_assets"
	DemandOptionPerformSteps       = "perform_steps"
)

// DemandOptionWinners maps each draft option key to the player ID that won it.
// Persisted as JSONB on plans.demand_option_winners for the demand plan row.
type DemandOptionWinners map[string]int64

// MakeDemandsDifficulty returns a demand plan's final roll difficulty.
// targetDiff is the target plan's own (already computed) difficulty.
// When the target plan's preparer outranks the demander (targetRank lower
// than demanderRank in power), the gap is added; otherwise no bonus.
func MakeDemandsDifficulty(targetDiff, demanderRank, targetRank int16) int16 {
	if targetRank < demanderRank {
		return targetDiff + (demanderRank - targetRank)
	}
	return targetDiff
}

// DemandRowPlacement returns the row a demand should land on: one row before
// the target plan's row, but never earlier than the game's current row
// (demands on the current row resolve immediately).
func DemandRowPlacement(targetRow, currentRow int16) int16 {
	return max(targetRow-1, currentRow)
}

// DemandDraftPickers returns (firstPicker, secondPicker) for the post-make
// draft between the demander and target plan's preparer. The higher-ranked
// player (lower rank number) picks first; ranks are unique per (game,
// category) so no tiebreaker is needed.
func DemandDraftPickers(
	demanderID, targetPreparerID int64,
	demanderRank, targetRank int16,
) (int64, int64) {
	if demanderRank < targetRank {
		return demanderID, targetPreparerID
	}
	return targetPreparerID, demanderID
}

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
	demands, err := q.GetPlansTargeting(ctx, &plan.ID)
	if err != nil {
		return 0, fmt.Errorf("look up demands targeting plan %d: %w", plan.ID, err)
	}
	for _, d := range demands {
		if d.Status != model.PlanResolved {
			continue
		}
		if d.Result == nil || *d.Result != "make" {
			continue
		}
		if len(d.DemandOptionWinners) == 0 {
			continue
		}
		var winners DemandOptionWinners
		if err := json.Unmarshal(d.DemandOptionWinners, &winners); err != nil {
			return 0, fmt.Errorf("decode demand_option_winners for plan %d: %w", d.ID, err)
		}
		if winner, ok := winners[DemandOptionKeepAssets]; ok && winner != 0 {
			return winner, nil
		}
	}
	return plan.PreparerID, nil
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
) (*dbgen.Plan, DemandOptionWinners, error) {
	demands, err := q.GetPlansTargeting(ctx, &plan.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("look up demands targeting plan %d: %w", plan.ID, err)
	}
	for i := range demands {
		d := demands[i]
		if d.Status != model.PlanResolved {
			continue
		}
		if d.Result == nil || *d.Result != "make" {
			continue
		}
		if len(d.DemandOptionWinners) == 0 {
			continue
		}
		var winners DemandOptionWinners
		if err := json.Unmarshal(d.DemandOptionWinners, &winners); err != nil {
			return nil, nil, fmt.Errorf("decode demand_option_winners for plan %d: %w", d.ID, err)
		}
		return &d, winners, nil
	}
	return nil, nil, nil
}
