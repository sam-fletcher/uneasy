// Package game — Make Demands pure helpers.
//
// Draft option keys are the four mechanical sub-choices a demand's make-roll
// drafts between the demander and the target plan's preparer. They are
// persisted on the *demand* plan row in demand_option_winners (JSONB) so the
// target plan's resolution path can consult them without re-walking the
// demand plan.
package game

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

// DemandPlacement returns the (row_number, row_order) a demand should land
// on. Make Demands is an exception to the normal "rows below current" delay
// rule: the demand goes on the *same* row as its target and slots in
// immediately *before* it, so the demand resolves first within the row.
// Callers must shift the row_order of the target (and anything after it on
// that row) by +1 to make room — see ShiftRowOrderAtOrAfter.
func DemandPlacement(targetRow, targetRowOrder int16) (int16, int16) {
	return targetRow, targetRowOrder
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
