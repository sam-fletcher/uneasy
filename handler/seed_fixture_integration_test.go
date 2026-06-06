//go:build integration

// handler/seed_fixture_integration_test.go — self-tests for the parameterized
// gametest fixtures (Step 1 / "thin A"). These prove the seed options produce
// the board they claim to, so the characterization tests built on top of them
// can trust their starting state.
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/gametest"
	"uneasy/model"
)

// rankByPlayer indexes rankings as [category][playerID] = rank for assertions.
func rankByPlayer(rows []dbgen.Ranking) map[model.RankingCategory]map[int64]int16 {
	out := map[model.RankingCategory]map[int64]int16{}
	for _, r := range rows {
		if r.PlayerID == nil {
			continue
		}
		if out[r.Category] == nil {
			out[r.Category] = map[int64]int16{}
		}
		out[r.Category][*r.PlayerID] = r.Rank
	}
	return out
}

func TestSeedMainEvent_Options_ShapeTheBoard(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Three players: bump current_row to 9, reverse the power track (player 2
	// at rank 1, player 0 at rank 3), and drop a Make War plan on row 9.
	tg := newTestGame(t, q, 3,
		gametest.WithCurrentRow(9),
		gametest.WithRankings(model.CategoryPower, []int{2, 1, 0}),
		gametest.WithPlan(gametest.SeedPlan{
			PreparerIdx: 0, PlanType: model.PlanMakeWar,
			Category: model.CategoryPower, Row: 9, RowOrder: 0,
		}),
	)

	assert.Equal(t, model.PhaseMainEvent, tg.Game.Phase)
	assert.EqualValues(t, 9, tg.Game.CurrentRow)

	rankings, err := q.ListRankingsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	byPlayer := rankByPlayer(rankings)

	// Power track was reversed: order [2,1,0] → player2=rank1, player0=rank3.
	assert.EqualValues(t, 1, byPlayer[model.CategoryPower][tg.Players[2].ID])
	assert.EqualValues(t, 2, byPlayer[model.CategoryPower][tg.Players[1].ID])
	assert.EqualValues(t, 3, byPlayer[model.CategoryPower][tg.Players[0].ID])
	// Untouched tracks keep seat order (player i → rank i+1).
	assert.EqualValues(t, 1, byPlayer[model.CategoryEsteem][tg.Players[0].ID])
	assert.EqualValues(t, 3, byPlayer[model.CategoryEsteem][tg.Players[2].ID])

	plans, err := q.ListPlansByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, model.PlanMakeWar, plans[0].PlanType)
	assert.Equal(t, tg.Players[0].ID, plans[0].PreparerID)
	require.NotNil(t, plans[0].RowNumber)
	assert.EqualValues(t, 9, *plans[0].RowNumber)
}

func TestSeedMainEvent_RejectsBadRankings(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	// [0,0,1] is not a permutation of 0..2 — must be rejected before any writes.
	_, err := gametest.SeedMainEvent(context.Background(), q,
		[]string{"a-" + randSuffix(), "b-" + randSuffix(), "c-" + randSuffix()},
		gametest.WithRankings(model.CategoryPower, []int{0, 0, 1}),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rankings[power]")
}

func TestSeedShakeUp_Default_MirrorsBeginShakeUp(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	seeded, err := gametest.SeedShakeUp(ctx, q,
		[]string{"su-a-" + randSuffix(), "su-b-" + randSuffix()})
	require.NoError(t, err)

	assert.Equal(t, model.PhaseShakeUp, seeded.Game.Phase)
	require.NotNil(t, seeded.Game.ShakeUpCategory)
	assert.Equal(t, gamepkg.ShakeUpCategoryEsteem, *seeded.Game.ShakeUpCategory)
	require.NotNil(t, seeded.Game.ShakeUpStep)
	assert.Equal(t, gamepkg.ShakeUpStepRolling, *seeded.Game.ShakeUpStep)

	// Freshly-entered shake-up: every player's token pool is zero.
	for _, p := range seeded.Players {
		n, err := q.GetShakeUpTokens(ctx, p.ID)
		require.NoError(t, err)
		assert.EqualValues(t, 0, n, "fresh shake-up should have zero tokens")
	}
}

func TestSeedShakeUp_SpendingStepWithTokens(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	seeded, err := gametest.SeedShakeUp(ctx, q,
		[]string{"sus-a-" + randSuffix(), "sus-b-" + randSuffix(), "sus-c-" + randSuffix()},
		gametest.WithShakeUpTokens(5),
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending),
	)
	require.NoError(t, err)

	require.NotNil(t, seeded.Game.ShakeUpStep)
	assert.Equal(t, gamepkg.ShakeUpStepSpending, *seeded.Game.ShakeUpStep)
	for _, p := range seeded.Players {
		n, err := q.GetShakeUpTokens(ctx, p.ID)
		require.NoError(t, err)
		assert.EqualValues(t, 5, n)
	}
}
