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
// finalized; the target plan's roll is no longer open (the window has closed);
// or the target plan type has no preparer-owned roll at all, where the option
// is inert and must not touch someone else's roll (mdTargetHasPreparerRoll).
func pendingControlLeverageChooser(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) int64 {
	if !mdTargetHasPreparerRoll(plan.PlanType) {
		return 0
	}
	return pendingDemandChooser(ctx, q, plan, game.DemandOptionControlLeverage,
		loadResolutionData(plan.ResolutionData).DemandLeverageFinalized)
}

// pendingDemandRetargetChooser is pendingControlLeverageChooser's
// keep_or_change_target twin: the winner who still owes their keep-or-re-aim
// call on a plan targeted by a made Make Demands, or 0.
//
// Same shape, same reason. "Keep the current target" is a legitimate choice the
// rules hand the winner, and it leaves the plan's target columns untouched —
// indistinguishable from having never acted. So the winner must explicitly
// finalize, and until they do they block the roll (seeded unready, excluded from
// the auto-ready sweeps). Without that, a target whose roll opens at kickoff
// could resolve the window shut before they ever saw it (audit D5).
//
// Returns 0 in the same four cases: no such demand; the winner is the preparer
// (no handoff — they re-aim their own plan directly); they have already
// finalized; or the target plan's roll is no longer open. Plus the D7 case —
// a target with no preparer-owned roll, where there is nothing to hold open.
func pendingDemandRetargetChooser(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) int64 {
	if !mdTargetHasPreparerRoll(plan.PlanType) {
		return 0
	}
	return pendingDemandChooser(ctx, q, plan, game.DemandOptionKeepOrChangeTarget,
		loadResolutionData(plan.ResolutionData).DemandRetargetFinalized)
}

// pendingDemandChooser is the body shared by the two pre-roll demand-option
// gates above: the winner of option holds up plan's roll until finalized flips.
// Returns 0 when there is no demand, no winner, the winner is the plan's own
// preparer (no handoff), they have already finalized, or the roll is closed.
func pendingDemandChooser(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	option string,
	finalized bool,
) int64 {
	_, winners, err := DemandWinnersForTargetPlan(ctx, q, plan)
	if err != nil || winners == nil {
		return 0
	}
	winnerID := winners[option]
	if winnerID == 0 || winnerID == plan.PreparerID {
		return 0
	}
	if finalized {
		return 0
	}
	// Only while the target plan's roll is still open: the winner can act only
	// during the pre-roll window, and once the roll resolves the gate is moot.
	roll, err := q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil || !rollIsOpen(&roll) {
		return 0
	}
	return winnerID
}

// pendingDemandRollBlockers returns every Make Demands option winner who still
// owes a pre-roll decision on this (target) plan's open roll, and therefore
// keeps it from resolving: the control_leverage winner and the
// keep_or_change_target winner. Empty when nobody is holding the roll.
//
// The roll's stage machine only needs the set, not which option each blocker
// holds — seedRollParticipants seeds them unready, runAutoReadySweep refuses to
// auto-ready them, and advanceToLeverage refuses to short-circuit past them.
func pendingDemandRollBlockers(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) map[int64]bool {
	blockers := map[int64]bool{}
	if id := pendingControlLeverageChooser(ctx, q, plan); id != 0 {
		blockers[id] = true
	}
	if id := pendingDemandRetargetChooser(ctx, q, plan); id != 0 {
		blockers[id] = true
	}
	return blockers
}

// mdTargetHasPreparerRoll reports whether a demand target of this plan type
// resolves through a dice roll of the target *preparer's* own — the thing the
// two pre-roll demand options (control_leverage, keep_or_change_target) attach
// to. Three target types have none, and the option that draws them is inert
// (audit D7):
//
//   - Host Festivity: the host never rolls. Every roll carrying a festivity
//     plan_id belongs to a *guest* (plan_host_festivity.go). GetDiceRollByPlanID
//     is ORDER BY created_at DESC LIMIT 1 over that plan_id, so without this
//     guard a control_leverage winner would block the first guest's roll and
//     /demand-leverage would spend the host's assets into it as interference.
//   - Propose Duel: the final roll is built straight from the accumulated bout
//     dice (plan_propose_duel_bouts.go), bypassing createPlanRoll and therefore
//     seedRollParticipants — so there are no participant rows to ready, and
//     leveraging into a duel result is wrong under the rules anyway.
//   - Clandestinely Liaise: no roll at all, ever.
//
// The rules already expect the four options to land unevenly ("They'll each
// have a different impact depending on which plan is being targeted… at least
// two of them should be juicy"), so an inert option is a legitimate outcome.
// An option that misfires or 500s is not.
func mdTargetHasPreparerRoll(t model.PlanType) bool {
	return t != model.PlanHostFestivity &&
		t != model.PlanProposeDuel &&
		t != model.PlanClandestinelyLiaise
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
