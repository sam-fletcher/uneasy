//go:build integration

// handler/shake_up_effects_integration_test.go — characterization tests for the
// Shake-Up spend *effects* (Step 2 of the testability roadmap, see
// adr/TESTABILITY_AND_ENGINE_DECOUPLING_PLAN.md).
//
// These pin the current behavior of the DB-coupled engine logic that Option E
// will later move out of package handler: the rank-bump swap, asset take/break,
// and category→category→ended progression. They are deliberately
// characterization tests — they assert what the code does today so the E
// refactor can prove it preserved behavior. They lean on the new SeedShakeUp
// fixture, which replaces ~50 lines of hand-rolled setup per case.
//
// The effect functions read plain fields off a *dbgen.ShakeUpSpend and mutate
// the DB directly, so we drive them with constructed spend literals rather than
// the full announce→adjust→commit HTTP flow (token accounting is exercised
// separately by the currentShakeUpActor ordering test).
package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/gametest"
	"uneasy/hub"
	"uneasy/model"
)

// newShakeUpGame seeds a fresh shake_up game with n players via the gametest
// fixture and fresh usernames. Default board: esteem/rolling, zero tokens, all
// three tracks in seat order (player i → rank i+1).
func newShakeUpGame(t *testing.T, q *dbgen.Queries, n int, opts ...gametest.Option) gametest.SeededGame {
	t.Helper()
	usernames := make([]string, n)
	for i := range usernames {
		usernames[i] = fmt.Sprintf("su%d-%s", i+1, randSuffix())
	}
	seeded, err := gametest.SeedShakeUp(context.Background(), q, usernames, opts...)
	require.NoError(t, err)
	return seeded
}

// committedPosts returns the bodies of all shake_up.committed action-log posts
// for the game, in insertion order.
func committedPosts(t *testing.T, q *dbgen.Queries, gameID int64) []string {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "shake_up.committed" {
			out = append(out, p.Body)
		}
	}
	return out
}

// rankOf returns player's rank on the given category.
func rankOf(t *testing.T, q *dbgen.Queries, gameID int64, cat model.RankingCategory, playerID int64) int16 {
	t.Helper()
	rankings, err := q.ListRankingsByGame(context.Background(), gameID)
	require.NoError(t, err)
	for _, r := range rankings {
		if r.Category == cat && r.PlayerID != nil && *r.PlayerID == playerID {
			return r.Rank
		}
	}
	t.Fatalf("no %s rank for player %d", cat, playerID)
	return 0
}

// TestShakeUpBumpRank_SwapsWithDisplaced pins the core ranking mechanic: a
// bump moves the spender up one rank and pushes the player who held that rank
// down into the spender's old slot. bump_knowledge targets the knowledge track.
func TestShakeUpBumpRank_SwapsWithDisplaced(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	// Default knowledge ranks: p0=1, p1=2, p2=3.
	seeded := newShakeUpGame(t, q, 3)
	gameID := seeded.Game.ID

	// p2 (rank 3) bumps knowledge → climbs to rank 2; p1 is displaced to rank 3.
	spend := &dbgen.ShakeUpSpend{
		OptionKey: gamepkg.ShakeUpOptBumpKnowledge,
		PlayerID:  seeded.Players[2].ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	assert.EqualValues(t, 2, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[2].ID),
		"spender climbs one rank")
	assert.EqualValues(t, 3, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[1].ID),
		"displaced player drops into the vacated slot")
	assert.EqualValues(t, 1, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[0].ID),
		"untouched player keeps their rank")

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "rise to rank 2 on Knowledge")
	assert.Contains(t, posts[0], "displacing")
}

// TestShakeUpBumpRank_TopRankIsNoOp pins that bumping from rank 1 does nothing.
func TestShakeUpBumpRank_TopRankIsNoOp(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 3)
	gameID := seeded.Game.ID

	// p0 is already knowledge rank 1.
	spend := &dbgen.ShakeUpSpend{
		OptionKey: gamepkg.ShakeUpOptBumpKnowledge,
		PlayerID:  seeded.Players[0].ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	assert.EqualValues(t, 1, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[0].ID))
	assert.EqualValues(t, 2, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[1].ID))
	assert.EqualValues(t, 3, rankOf(t, q, gameID, model.CategoryKnowledge, seeded.Players[2].ID))

	// Even a no-op bump is logged — the rules dwell on spends that change nothing.
	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], "already at the top")
}

// TestShakeUpEffect_TakeAsset_TransfersOwnership pins that a take_* option
// reassigns the target asset to the spender.
func TestShakeUpEffect_TakeAsset_TransfersOwnership(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID

	peer, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: gameID, OwnerID: seeded.Players[1].ID, CreatorID: seeded.Players[1].ID,
		AssetType: model.AssetPeer, Name: "Loyal guard",
	})
	require.NoError(t, err)

	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptTakePeer,
		PlayerID:      seeded.Players[0].ID,
		TargetAssetID: &peer.ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	got, err := q.GetAssetByID(ctx, peer.ID)
	require.NoError(t, err)
	assert.Equal(t, seeded.Players[0].ID, got.OwnerID, "asset transfers to the spender")
	assert.False(t, got.IsDestroyed)

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], `to take **Loyal guard** (peer)`)
}

// TestShakeUpEffect_BreakAsset_Destroys pins that a break_* option destroys the
// target asset.
func TestShakeUpEffect_BreakAsset_Destroys(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID

	res, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: gameID, OwnerID: seeded.Players[1].ID, CreatorID: seeded.Players[1].ID,
		AssetType: model.AssetResource, Name: "Granary",
	})
	require.NoError(t, err)

	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptBreakResource,
		PlayerID:      seeded.Players[0].ID,
		TargetAssetID: &res.ID,
	}
	require.NoError(t, applyShakeUpEffect(ctx, q, manager, gameID, spend, 1))

	got, err := q.GetAssetByID(ctx, res.ID)
	require.NoError(t, err)
	assert.True(t, got.IsDestroyed, "broken asset is destroyed")

	posts := committedPosts(t, q, gameID)
	require.Len(t, posts, 1)
	assert.Contains(t, posts[0], `to break`)
	assert.Contains(t, posts[0], `**Granary** (resource)`)
}

// TestMaybeAdvanceShakeUpCategory_Progression pins the category machine: with
// every pool empty, the spending step advances esteem → knowledge → power →
// ended.
func TestMaybeAdvanceShakeUpCategory_Progression(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	// Seed at the spending step with zero tokens (every pool already empty).
	seeded := newShakeUpGame(t, q, 2, gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending))
	gameID := seeded.Game.ID

	requireCategory := func(wantCat string, wantStep int16) {
		g, err := q.GetGameByID(ctx, gameID)
		require.NoError(t, err)
		require.NotNil(t, g.ShakeUpCategory)
		require.NotNil(t, g.ShakeUpStep)
		assert.Equal(t, wantCat, *g.ShakeUpCategory)
		assert.Equal(t, wantStep, *g.ShakeUpStep)
	}

	requireCategory(gamepkg.ShakeUpCategoryEsteem, gamepkg.ShakeUpStepSpending)

	// esteem → knowledge (back to the rolling step).
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, q, manager, gameID))
	requireCategory(gamepkg.ShakeUpCategoryKnowledge, gamepkg.ShakeUpStepRolling)

	// knowledge → power.
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, q, manager, gameID))
	requireCategory(gamepkg.ShakeUpCategoryPower, gamepkg.ShakeUpStepRolling)

	// power → ended.
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, q, manager, gameID))
	g, err := q.GetGameByID(ctx, gameID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseEnded, g.Phase, "game ends after the power category")
}
