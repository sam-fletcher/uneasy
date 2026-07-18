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
	// highest status, player 0 lowest), and drop a Make War plan on row 9.
	// For 3 players the open ranks are 2,3,4 (dummies sit at 1 and 5), so the
	// reversed track maps player2→2, player1→3, player0→4.
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

	// Power track was reversed: order [2,1,0] over open ranks [2,3,4] →
	// player2=rank2, player1=rank3, player0=rank4.
	assert.EqualValues(t, 2, byPlayer[model.CategoryPower][tg.Players[2].ID])
	assert.EqualValues(t, 3, byPlayer[model.CategoryPower][tg.Players[1].ID])
	assert.EqualValues(t, 4, byPlayer[model.CategoryPower][tg.Players[0].ID])
	// Untouched tracks keep seat order over the open ranks (player i → open[i]).
	assert.EqualValues(t, 2, byPlayer[model.CategoryEsteem][tg.Players[0].ID])
	assert.EqualValues(t, 4, byPlayer[model.CategoryEsteem][tg.Players[2].ID])

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

func TestSeedPrologueClosing_ParksAtClosingWithNoBoard(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	names := []string{"pc-a-" + randSuffix(), "pc-b-" + randSuffix(), "pc-c-" + randSuffix()}
	seeded, err := gametest.SeedPrologueClosing(ctx, q, names)
	require.NoError(t, err)

	// Parked at the closing step of the prologue.
	assert.Equal(t, model.PhasePrologue, seeded.Game.Phase)
	require.NotNil(t, seeded.Game.PrologueRankingStep)
	assert.Equal(t, gamepkg.PrologueStepClosing, *seeded.Game.PrologueRankingStep)

	// Rankings seeded for all three tracks (drives the recap's final standings).
	rankings, err := q.ListRankingsByGame(ctx, seeded.Game.ID)
	require.NoError(t, err)
	byPlayer := rankByPlayer(rankings)
	assert.Len(t, byPlayer, 3, "power/knowledge/esteem all seeded")

	// Every player holds their four starting assets, one of them the MC.
	assets, err := q.ListAssetsByGame(ctx, seeded.Game.ID)
	require.NoError(t, err)
	assert.Len(t, assets, 4*len(names))
	mcs := 0
	for _, a := range assets {
		if a.IsMainCharacter {
			mcs++
		}
	}
	assert.Equal(t, len(names), mcs, "one main character per player")

	// No Public Record board yet — this is the invariant that lets a later
	// all-ready advance run advanceToMainEvent without a dup-key insert.
	rows, err := q.ListPublicRecordRows(ctx, seeded.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, rows, "prologue game must have no record rows")
}

func TestSeed_WithLawsAndRumors(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	names := []string{"lr-a-" + randSuffix(), "lr-b-" + randSuffix(), "lr-c-" + randSuffix()}
	seeded, err := gametest.SeedMainEvent(ctx, q, names,
		gametest.WithLaw("No dueling within the palace walls."),
		gametest.WithLaw("Grain tithes are halved in a lean year."),
		gametest.WithRumor("The chancellor keeps two sets of books."),
	)
	require.NoError(t, err)

	// Laws are signed by the first player, in insertion order.
	laws, err := q.ListLaws(ctx, seeded.Game.ID)
	require.NoError(t, err)
	require.Len(t, laws, 2)
	assert.Equal(t, "No dueling within the palace walls.", laws[0].Text)
	assert.Equal(t, "Grain tithes are halved in a lean year.", laws[1].Text)
	for _, law := range laws {
		require.NotNil(t, law.SignatoryID)
		assert.Equal(t, seeded.Players[0].ID, *law.SignatoryID)
		assert.Nil(t, law.OriginPlanID, "seeded law has no origin plan")
	}

	// The rumor is sourced to the last player.
	rumors, err := q.ListRumors(ctx, seeded.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1)
	assert.Equal(t, "The chancellor keeps two sets of books.", rumors[0].Text)
	require.NotNil(t, rumors[0].SourcePlayerID)
	assert.Equal(t, seeded.Players[len(names)-1].ID, *rumors[0].SourcePlayerID)

	// No options => no seeded laws/rumors (the fixture layer stays opt-in; the
	// dev handler is what adds the samples by default).
	plain, err := gametest.SeedMainEvent(ctx, q,
		[]string{"lr-x-" + randSuffix(), "lr-y-" + randSuffix()})
	require.NoError(t, err)
	plainLaws, err := q.ListLaws(ctx, plain.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, plainLaws)
	plainRumors, err := q.ListRumors(ctx, plain.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, plainRumors)
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
