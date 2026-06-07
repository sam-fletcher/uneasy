package game

import "uneasy/model"

// PlanLockoutView is the decoupled snapshot of one prepared plan that
// EsteemLockoutActive needs: its ranking category, its type, and (for Spread
// Propaganda plans) whether its resolution data carried the esteem-lockout
// flag. The shell builds these from query rows so this rule stays pure.
type PlanLockoutView struct {
	Category model.RankingCategory
	PlanType model.PlanType
	// EsteemLockout is the parsed SpreadPropaganda.EsteemLockout flag. It is
	// only meaningful on Spread Propaganda plans; false otherwise.
	EsteemLockout bool
}

// EsteemLockoutActive reports whether a player has an active esteem lockout
// from a Spread Propaganda mar option (b) "censured".
//
// plans MUST be ordered newest-first (most recently prepared first) — the rule
// is "the player's most recent plan decides". Walking newest-first: the first
// non-esteem plan proves the lockout has cleared (that plan became the most
// recent); the first Spread Propaganda plan carrying the lockout flag (with no
// non-esteem plan seen yet) proves it is still active.
func EsteemLockoutActive(newestFirst []PlanLockoutView) bool {
	for _, p := range newestFirst {
		if p.Category != model.CategoryEsteem {
			// A non-esteem plan newer than any lockout → cleared.
			return false
		}
		if p.PlanType == model.PlanSpreadPropaganda && p.EsteemLockout {
			return true
		}
	}
	return false
}
