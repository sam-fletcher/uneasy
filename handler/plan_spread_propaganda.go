package handler

// handler/plan_spread_propaganda.go — Spread Propaganda plan handler (Phase 3b).
//
// Spread Propaganda (esteem, delay 3): The preparer spreads a message.
// Difficulty = preparer's rank on the esteem track.
//
// Make options: "wide", "targeted", "incite" (narrative).
// Mar options:
//   (a) "backfire"  — rumor reflects back at preparer (narrative)
//   (b) "censured"  — esteem lockout: preparer's next plan cannot be an esteem plan
//   (c) "dismissed" — no one believes it (narrative)
//   (d) "co-opt"    — top interferer spreads their own propaganda immediately
//
// Esteem lockout (b): sets ResData.EsteemLockout = true. Checked in
// validatePlanPreparation for all esteem-category plan types.
//
// Recursive resolve (d): finds the top interferer by dice count (ties broken
// by best esteem rank), creates a new SP plan at current_row with status
// 'resolving', and creates a dice roll. Depth capped at 1.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSpreadPropaganda, spHandler{})
}

type spHandler struct{}

func (spHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 3}
}

func (spHandler) ValidatePreparation(_ context.Context, _ *ValidationContext) (int16, string) {
	return 0, "" // no plan-specific prerequisites; fixed delay
}

func (spHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer ranking: %w", err)
	}
	return gamepkg.SpreadPropagandaDifficulty(preparerRank), nil
}

// OnResolve creates the dice roll immediately (no pre-roll step).
func (spHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResolutionData(plan.ResolutionData)
	difficulty, err := spHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

// ApplyChoice handles SP mar options (b) "censured" and (d) "co-opt".
func (spHandler) ApplyChoice(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	choices []string,
	result string,
) error {
	if result != marOutcome {
		return nil // make options are purely narrative
	}

	for _, choice := range choices {
		switch choice {
		case "censured":
			resData.EsteemLockout = true

		case "co-opt":
			if err := applyCoOpt(ctx, deps, plan, resData); err != nil {
				return err
			}
		}
	}
	return nil
}

func (spHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return nil
}

func (spHandler) ExtraRoutes(_ *PlanDeps) map[string]http.HandlerFunc {
	return nil
}

// ── Co-opt (recursive propaganda) ────────────────────────────────────────────

// applyCoOpt implements SP mar option (d): the top interferer spreads their
// own propaganda at the current row, resolving immediately.
func applyCoOpt(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
) error {
	// Depth cap: recursive plans cannot co-opt again.
	if resData.OriginalPlanID != nil {
		return errors.New("co-opt is not available on a recursive propaganda plan")
	}

	// Find the resolved roll for this plan.
	roll, err := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
	if err != nil {
		return fmt.Errorf("could not find dice roll for plan: %w", err)
	}

	// Find top interferer(s).
	interferers, err := deps.Q.ListInterferenceDiceByRoll(ctx, roll.ID)
	if err != nil || len(interferers) == 0 {
		return errors.New("co-opt is not available: no interference dice were committed to this roll")
	}

	topCount := interferers[0].DiceCount
	topPlayerID, err := pickBestEsteemRanked(ctx, deps.Q, plan.GameID, interferers, topCount)
	if err != nil {
		return fmt.Errorf("could not determine top interferer: %w", err)
	}

	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return fmt.Errorf("could not load game: %w", err)
	}

	count, err := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    game.ID,
		RowNumber: new(game.CurrentRow),
	})
	if err != nil {
		count = 0
	}

	// Create the recursive SP plan.
	recursivePlan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:           game.ID,
		PlanType:         model.PlanSpreadPropaganda,
		Category:         model.CategoryEsteem,
		PreparerID:       topPlayerID,
		TargetPlayerID:   nil,
		TargetAssetID:    nil,
		RowNumber:        new(game.CurrentRow),
		RowOrder:         int16(count),
		PreparedAtRow:    game.CurrentRow,
		PreparationNotes: nil,
	})
	if err != nil {
		return fmt.Errorf("could not create recursive propaganda plan: %w", err)
	}

	// Mark it as resolving immediately (skips the pending phase).
	err = deps.Q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID:     recursivePlan.ID,
		Status: model.PlanResolving,
	})
	if err != nil {
		return fmt.Errorf("could not mark recursive plan as resolving: %w", err)
	}

	// Tag it in ResData so its own co-opt option is blocked.
	parentID := plan.ID
	recursiveResData := ResolutionData{OriginalPlanID: &parentID}
	if err = saveResolutionData(ctx, deps.Q, recursivePlan.ID, recursiveResData); err != nil {
		return fmt.Errorf("could not save recursive plan data: %w", err)
	}

	// Compute difficulty for the recursive plan (top interferer's esteem rank).
	difficulty, err := spHandler{}.ComputeDifficulty(ctx, deps.Q, &recursivePlan, &recursiveResData)
	if err != nil {
		return fmt.Errorf("could not compute recursive plan difficulty: %w", err)
	}

	// Create the dice roll. The normal leverage window then opens.
	if _, err = createPlanRoll(ctx, deps.Q, deps.Manager, &game, &recursivePlan, difficulty, topPlayerID); err != nil {
		return fmt.Errorf("could not create recursive dice roll: %w", err)
	}

	// Record the recursive plan ID in the parent's ResData.
	resData.RecursivePlanID = &recursivePlan.ID

	broadcastEvent(deps.Manager, game.ID, model.EventSPRecursivePlan, model.SPRecursivePlanPayload{
		ParentPlanID:    plan.ID,
		RecursivePlanID: recursivePlan.ID,
		PreparerID:      topPlayerID,
	})

	return nil
}

// pickBestEsteemRanked selects the player with the lowest esteem rank
// (= highest status) among those tied at topCount interference dice.
func pickBestEsteemRanked(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	interferers []dbgen.ListInterferenceDiceByRollRow,
	topCount int64,
) (int64, error) {
	bestRank := int16(999)
	bestPlayerID := int64(0)

	for _, row := range interferers {
		if row.DiceCount < topCount {
			break // rows are ordered by dice_count DESC
		}
		rank, err := playerRankInCategory(ctx, q, gameID, row.PlayerID, model.CategoryEsteem)
		if err != nil {
			continue
		}
		if rank < bestRank {
			bestRank = rank
			bestPlayerID = row.PlayerID
		}
	}

	if bestPlayerID == 0 {
		if len(interferers) > 0 {
			return interferers[0].PlayerID, nil // fallback
		}
		return 0, errors.New("no interferers found")
	}
	return bestPlayerID, nil
}

// hasEsteemLockout is defined in the game package and aliased in
// plan_registry.go as `hasEsteemLockout`.
