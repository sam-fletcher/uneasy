//go:build integration

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

// TestCurrentShakeUpActor_reverseRankAndAdvance verifies that the spending
// turn starts with the lowest-status player and advances in reverse-rank
// order after each committed spend, skipping players who are out of tokens.
func TestCurrentShakeUpActor_reverseRankAndAdvance(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	esteem := string(model.CategoryEsteem)

	// SeedShakeUp gives esteem ranks in seat order (p0=1 … p2=3) and grants
	// each player 5 tokens at the spending step — the board this test needs
	// without the hand-rolled game/player/ranking/token setup. Reverse-rank
	// turn order is therefore p2, p1, p0.
	seeded := newShakeUpGame(t, q, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending),
		gametest.WithShakeUpTokens(5),
	)
	game := seeded.Game
	players := seeded.Players

	commitSpend := func(playerID int64) {
		spend, err := q.CreateShakeUpSpend(ctx, dbgen.CreateShakeUpSpendParams{
			GameID: game.ID, PlayerID: playerID, Category: esteem,
			OptionKey: gamepkg.ShakeUpOptBumpKnowledge, BaseCost: 1,
		})
		require.NoError(t, err)
		fc := int16(1)
		require.NoError(t, q.CommitShakeUpSpend(ctx, dbgen.CommitShakeUpSpendParams{
			ID: spend.ID, FinalCost: &fc,
		}))
	}

	// No spends yet → lowest-status player (p3) acts first.
	actor, err := currentShakeUpActor(ctx, q, game.ID, esteem)
	require.NoError(t, err)
	assert.Equal(t, players[2].ID, actor, "lowest-status player acts first")

	// p3 commits → turn passes to p2, then to p1.
	commitSpend(players[2].ID)
	actor, err = currentShakeUpActor(ctx, q, game.ID, esteem)
	require.NoError(t, err)
	assert.Equal(t, players[1].ID, actor)

	commitSpend(players[1].ID)
	actor, err = currentShakeUpActor(ctx, q, game.ID, esteem)
	require.NoError(t, err)
	assert.Equal(t, players[0].ID, actor)

	// p1 commits → order loops back to p3 (still holds tokens).
	commitSpend(players[0].ID)
	actor, err = currentShakeUpActor(ctx, q, game.ID, esteem)
	require.NoError(t, err)
	assert.Equal(t, players[2].ID, actor, "turn order loops back to the front")

	// Drain p3 and p1 of tokens; after p3 commits, p2 is the only holder and
	// the turn must skip the empty p1 to land on p2.
	require.NoError(t, q.ZeroShakeUpTokens(ctx, game.ID))
	_, err = q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{ID: players[1].ID, ShakeUpTokens: 3})
	require.NoError(t, err)
	commitSpend(players[2].ID)
	actor, err = currentShakeUpActor(ctx, q, game.ID, esteem)
	require.NoError(t, err)
	assert.Equal(t, players[1].ID, actor, "turn skips token-less players")
}
