package handler

// handler/table_title.go — which plan a table's title names.
//
// A table has one title, shown in its title bar and on the profile page's row
// for it: "Row 6 · Seek Answers", "Prologue · Choosing", "Lobby", "Game over".
// The words are built client-side from a handful of fields
// (frontend/src/lib/tableTitle.ts); this file picks the one that is not a plain
// column of the games row, the plan a Main Event row is about.
//
// It is decided here, once, and attached to the RowState (RowState.TitlePlanType),
// which the table page already receives live and which the profile list already
// computes per table. There is deliberately no client-side twin: the pick needs
// the row's scenes (the follow-scene gate below), which the client does not hold
// as a list, and two implementations of a rule that reads scenes were a drift
// waiting to happen.

import (
	"slices"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// titlePlan returns the type of the plan the Main Event row game.CurrentRow is
// about, or nil when the row has none. plans is the game's plans in the
// order ListPlansByGame returns them (row_number, row_order); recent is the plan
// that most recently resolved on this row (nil if none); scenes are the row's
// scenes. All three come from the RowState snapshot (rowStateReads).
//
// The rule follows the order the table actually plays a row in, so the title
// names the plan the table is on rather than the one it will be on next:
//
//  1. The plan being resolved, wherever it sits. A Host Festivity paused under
//     the duel it spawned gives way to the duel (planSuspendedByNested, the rule
//     the waiting-on set and the play area use). The first in order if several.
//  2. Otherwise, while a resolved plan's follow-scene turn is still being played
//     (followSceneGate, ComputeRowState's step 4.5, which runs before "plan
//     pending"), that resolved plan. With two plans on a row the table stays on
//     the first until its setter has passed focus, and only then moves to the second.
//  3. Otherwise the next plan waiting on the row: the lowest row_order
//     (topPendingPlanOnRow, ComputeRowState's pre-kickoff step).
//  4. Otherwise the plan that last resolved on the row, so a row keeps its name
//     through its scene and the next player's turn instead of dropping to a
//     bare "Row 6" the moment its plan finishes.
//
// A plan still waiting on its delay reveal (row_number NULL: Make War,
// Clandestinely Liaise) is on no row yet and is never named; a plan that fell
// through (cancelled) never is either.
func titlePlan(game *dbgen.Game, plans []dbgen.Plan, recent *dbgen.Plan, scenes []dbgen.Scene) *model.PlanType {
	var resolving []dbgen.Plan
	for _, p := range plans {
		if p.Status == model.PlanResolving {
			resolving = append(resolving, p)
		}
	}
	if len(resolving) > 0 {
		pick := resolving[0]
		if i := slices.IndexFunc(resolving, func(p dbgen.Plan) bool {
			return !planSuspendedByNested(&p, resolving)
		}); i >= 0 {
			pick = resolving[i]
		}
		return &pick.PlanType
	}
	if _, onFollowScene := followSceneGate(game, recent, scenes); onFollowScene {
		return &recent.PlanType
	}
	if top := topPendingPlanOnRow(plans, game.CurrentRow); top != nil {
		return &top.PlanType
	}
	if recent != nil {
		return &recent.PlanType
	}
	return nil
}
