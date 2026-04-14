package handler

// handler/plan_make_introductions.go — Make Introductions plan handler.
//
// Make Introductions (knowledge, delay 3): The preparer brings 1–4 new peers
// into the game. Difficulty = 2 + peer_count.
//
// Make options (per peer): "retinue" (peer joins preparer), "independent"
// (peer is unaffiliated), "gift" (peer goes to another player).
// Mar options: "delayed" (peer arrives in d6 rows — Phase 2 simplification;
// full delayed arrival is Phase 3b), "center" (peer goes to center of table).
//
// This handler has no extra routes — all make/mar effects are narrative or
// handled through existing asset endpoints.

import (
	"context"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanMakeIntroductions, miHandler{})
}

type miHandler struct{}

func (miHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryKnowledge, Delay: 3}
}

// makeIntroductionsDifficultyPure returns the difficulty given the ResData.
// Difficulty = 2 + peer_count (peer_count 0 treated as 1; range 1–4 → 3–6).
func makeIntroductionsDifficultyPure(resData ResData) int16 {
	const baseDifficulty = int16(2)
	pc := max(resData.PeerCount, 1)
	return baseDifficulty + pc
}

func (miHandler) ValidatePreparation(ctx context.Context, v *ValidationContext) (int16, string) {
	if v.PeerCount < 1 || v.PeerCount > 4 {
		return 0, "make_introductions requires peer_count between 1 and 4"
	}
	return 0, "" // fixed delay; target row computed from Metadata().Delay
}

func (miHandler) ComputeDifficulty(
	_ context.Context,
	_ *dbgen.Queries,
	_ *dbgen.Plan,
	resData *ResData,
) (int16, error) {
	return makeIntroductionsDifficultyPure(*resData), nil
}

// OnResolve creates the dice roll immediately (no pre-roll step).
func (miHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	game, err := deps.Q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return nil, err
	}
	resData := loadResData(plan.ResolutionData)
	difficulty := makeIntroductionsDifficultyPure(resData)
	return createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
}

func (miHandler) ApplyChoice(_ context.Context, _ *PlanDeps, _ *dbgen.Plan, _ *ResData, _ []string, _ string) error {
	return nil // all make/mar effects are narrative
}

func (miHandler) CanComplete(_ *dbgen.Plan, _ *ResData) error {
	return nil // no extra prerequisites
}

func (miHandler) ExtraRoutes(_ *PlanDeps) map[string]http.HandlerFunc {
	return nil
}

// store peer_count in resolution_data during plan preparation.
func miStoreResData(ctx context.Context, q *dbgen.Queries, planID int64, peerCount int16) error {
	d := ResData{PeerCount: peerCount}
	return saveResData(ctx, q, planID, d)
}
