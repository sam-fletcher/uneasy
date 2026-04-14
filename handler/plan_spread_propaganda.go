package handler

// handler/plan_spread_propaganda.go — Spread Propaganda plan handler.
//
// Spread Propaganda (esteem, delay 3): The preparer spreads a message across
// the game world. Difficulty = preparer's rank on the esteem track.
//
// Make options: "wide" (rumor reaches everyone), "targeted" (affects one player
// or asset), "incite" (causes a reaction from a named NPC/faction).
// Mar options: "backfire" (rumor reflects back at preparer),
// "dismissed" (no one believes it), "censured" (esteem lockout — preparer's
// next plan cannot be an esteem plan), "co-opt" (top interferer spreads their
// own propaganda immediately — Phase 2 simplification; recursive resolve is
// Phase 3b).
//
// This handler has no extra routes.

import (
	"context"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanSpreadPropaganda, spHandler{})
}

type spHandler struct{}

func (spHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 3}
}

// spreadPropagandaDifficultyPure returns the difficulty given the preparer's
// rank on the esteem track.
// Difficulty = preparer rank (rank 1–5 → difficulty 1–5).
func spreadPropagandaDifficultyPure(preparerRank int16) int16 {
	return preparerRank
}

func (spHandler) ValidatePreparation(_ context.Context, _ *ValidationContext) (int16, string) {
	return 0, "" // no plan-specific prerequisites; fixed delay
}

func (spHandler) ComputeDifficulty(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, _ *ResData) (int16, error) {
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("could not determine preparer ranking: %w", err)
	}
	return spreadPropagandaDifficultyPure(preparerRank), nil
}

// OnResolve creates the dice roll immediately (no pre-roll step).
func (spHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResData(plan.ResolutionData)
	difficulty, err := spHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
	if err != nil {
		return nil, err
	}
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

func (spHandler) ApplyChoice(_ context.Context, _ *PlanDeps, _ *dbgen.Plan, _ *ResData, _ []string, _ string) error {
	return nil // all make/mar effects are narrative
}

func (spHandler) CanComplete(_ *dbgen.Plan, _ *ResData) error {
	return nil // no extra prerequisites
}

func (spHandler) ExtraRoutes(_ *PlanDeps) map[string]http.HandlerFunc {
	return nil
}
