package handler

// handler/finale.go — Explosive Finale mechanics
// (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §3–§6).
//
// Under explosive_finale each player gets exactly ONE plan whose row is clamped
// to 13 instead of its natural row — prepared normally in every other respect
// (token, category, rank, notes). The slot is spent in one of two ways:
//
//  1. explicitly, by preparing a fixed-delay plan that would overflow: it is
//     clamped onto row 13 and created with is_finale_bonus set
//     (planOverflowOutcome below → validatePlanPreparation → CreatePlan); or
//  2. implicitly, when a Make War / Clandestinely Liaise delay reveal lands past
//     row 13 and collapses onto it instead of falling through
//     (delayRevealOverflow below, called from the two reveal sites).
//
// The second is the only place in the game a slot is spent without an explicit
// choice, which is why both reveal sites must state the outcome in the log.
//
// The overflow decision itself lives here in ONE function so the prep grid
// (planIneligibilityReason) and prepare-time validation (validatePlanPreparation)
// cannot drift: the grid exists to grey out precisely what a prepare call would
// reject, and keeping the two "in lockstep" by convention is exactly the kind of
// pairing that rots. They now read the same answer.

import (
	"context"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// finaleSlotSpent reports whether this player has already used their one
// Explosive Finale plan. Derived from plans (is_finale_bonus), never from a
// flag on the player, so the spend is always attributable to a plan.
func finaleSlotSpent(ctx context.Context, q *dbgen.Queries, gameID, playerID int64) (bool, error) {
	n, err := q.CountFinaleBonusPlans(ctx, dbgen.CountFinaleBonusPlansParams{
		GameID:     gameID,
		PreparerID: playerID,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// finaleCollapseSpendsSlot reports whether an overflowing delay reveal should
// collapse onto row 13 (spending the preparer's Finale slot) rather than let the
// plan fall through. True only under explosive_finale with the slot still free.
//
// Settled ruling (2026-07-27): a dice-delay plan that overflows SPENDS the
// bonus slot; if the slot is already spent the plan falls through exactly as it
// does today. Clamping without spending was rejected — it would make the
// one-per-player rule unenforceable for precisely the two plans whose delay is
// chosen rather than fixed, so a table could deliberately aim past 13 for a free
// extra plan.
func finaleCollapseSpendsSlot(
	ctx context.Context,
	q *dbgen.Queries,
	game *dbgen.Game,
	preparerID int64,
) (bool, error) {
	if game.EndingMode == nil || *game.EndingMode != EndingModeExplosiveFinale {
		return false, nil
	}
	spent, err := finaleSlotSpent(ctx, q, game.ID, preparerID)
	if err != nil {
		return false, err
	}
	return !spent, nil
}

// delayRevealOverflow decides what happens to a Make War / Clandestinely Liaise
// plan whose chosen delay lands past row 13, and carries out the half that is
// the same at both call sites. Returns true when the plan collapses onto row 13
// (the caller places it there), false when it falls through (the caller has
// already been given the fall-through by planFellThrough).
//
// The collapse SPENDS the preparer's Explosive Finale slot, and this is the one
// place in the game a slot is spent without an explicit choice — so it is stated
// plainly in the log rather than left to be inferred from the grid greying out
// later. Both outcomes get a post: today's silent cancel is a bug either way.
func delayRevealOverflow(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	game *dbgen.Game,
	plan *dbgen.Plan,
) bool {
	collapse, err := finaleCollapseSpendsSlot(ctx, q, game, plan.PreparerID)
	if err != nil {
		// Conservative: a slot we could not read is treated as spent, so the
		// plan falls through rather than silently handing out a second bonus.
		loggerFromContext(ctx).WarnContext(ctx,
			"delay reveal overflow: could not read the Explosive Finale slot; falling through",
			"plan_id", plan.ID, "err", err)
		collapse = false
	}
	if !collapse {
		return false
	}

	if err := q.SetPlanFinaleBonus(ctx, plan.ID); err != nil {
		loggerFromContext(ctx).WarnContext(ctx,
			"delay reveal overflow: could not spend the Explosive Finale slot",
			"plan_id", plan.ID, "err", err)
	}
	plan.IsFinaleBonus = true

	planID := plan.ID
	EmitSystemPost(ctx, q, manager, plan.GameID, "plan.finale_collapse",
		model.SeverityImportant,
		planLabel(plan.PlanType)+" would have landed past row 13. Under an Explosive Finale it "+
			"collapses onto row 13 instead of falling through — and that spends "+
			playerDisplayName(ctx, q, plan.PreparerID)+"'s one Explosive Finale plan.",
		logRow(*game), &planID, nil,
		map[string]any{"plan_id": plan.ID, "preparer_id": plan.PreparerID})
	return true
}

// planFellThrough is the shared "the plan never came together" path: mark it
// cancelled, take its token back off the shield, broadcast, and log.
//
// The token removal is the owner's ruling of 2026-07-28 — "there should be no
// shield for a plan that wasn't actually prepared". It used to stay, which
// accidentally produced the right outcome for the preparer (they could not
// re-pick) through the wrong mechanism, while also blocking every lower-ranked
// player from that plan type until the next engrailed line, for a plan that
// never happened. The preparer's own block is now derived in checkPlanEligible.
func planFellThrough(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	plan dbgen.Plan,
) {
	if err := q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID:     plan.ID,
		Status: model.PlanCancelled,
	}); err != nil {
		loggerFromContext(ctx).WarnContext(ctx,
			"fall-through: could not set the plan status", "plan_id", plan.ID, "err", err)
	}
	if err := q.DeletePlanTokenByPlan(ctx, plan.ID); err != nil {
		loggerFromContext(ctx).WarnContext(ctx,
			"fall-through: could not clear the plan token", "plan_id", plan.ID, "err", err)
	}
	broadcastEvent(manager, plan.GameID, model.EventPlanResolved, model.PlanResolvedPayload{
		PlanID: plan.ID,
		Result: "cancelled",
	})
	EmitPlanResolved(ctx, q, manager, plan, "cancelled")
}

// planPrepOverflow is what the ending mode says about a preparation whose row
// would land past row 13. Exactly one of the three states is set:
//
//   - Reason non-empty          → refuse (grid greys the tile out with this text)
//   - ModeUnsettled             → refuse, and the caller answers with the
//     structured endgame_choice_required body
//   - ClampToFinalRow / Bonus   → allow, on row 13, spending the Finale slot
//
// The all-clear (no overflow at all) is the zero value.
type planPrepOverflow struct {
	Reason          string
	ModeUnsettled   bool
	ClampToFinalRow bool
	FinaleBonus     bool
}

// planOverflowOutcome decides what happens to a preparation that would land
// past row 13, per the ending mode and the player's Finale slot.
//
// deferredRow marks the dice-delay plans (Make War, Clandestinely Liaise), whose
// row is decided by a post-prep simultaneous reveal. They are asymmetric on
// purpose and cannot be bonus plans at prep time — whether they overflow is not
// known until the reveal:
//
//   - with the slot free, an overflow is no longer futile (the reveal collapses
//     it onto row 13 and spends the slot there), so the declaration is allowed
//     through even when every possible delay overflows;
//   - with the slot spent, every outcome is a guaranteed fall-through, so it is
//     refused here rather than let through to be cancelled after the fact.
func planOverflowOutcome(
	ctx context.Context,
	q *dbgen.Queries,
	game *dbgen.Game,
	playerID int64,
	deferredRow bool,
) (planPrepOverflow, error) {
	switch {
	case game.EndingMode == nil:
		// Unreachable in normal play: the table vote settles the mode at the
		// row 7 → 8 boundary, and nothing can overflow before row 8 (max fixed
		// delay is 6, and 7 + 6 = 13). Reachable via DevAdvanceRow, which skips
		// the play-state gates by design.
		return planPrepOverflow{ModeUnsettled: true}, nil

	case *game.EndingMode == EndingModeSmoothLanding:
		return planPrepOverflow{
			Reason: "under a Smooth Landing no plan may land past row 13 — " +
				"choose a different plan, or don't prepare anything",
		}, nil

	case *game.EndingMode == EndingModeExplosiveFinale:
		spent, err := finaleSlotSpent(ctx, q, game.ID, playerID)
		if err != nil {
			return planPrepOverflow{}, err
		}
		if spent {
			return planPrepOverflow{
				Reason: "you have already used your one Explosive Finale plan",
			}, nil
		}
		if deferredRow {
			// Allowed, but not marked: the reveal decides, and the collapse
			// (with the slot spend and its log post) happens there.
			return planPrepOverflow{}, nil
		}
		return planPrepOverflow{ClampToFinalRow: true, FinaleBonus: true}, nil

	default:
		return planPrepOverflow{
			Reason: "endgame mode " + *game.EndingMode + " does not allow new plans past row 13",
		}, nil
	}
}
