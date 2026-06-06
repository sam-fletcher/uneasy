//go:build integration

package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
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

	game, err := q.CreateGame(ctx, "SHAKE"+randSuffix())
	require.NoError(t, err)

	// p1 esteem rank 1 (highest status) … p3 rank 3 (lowest). Reverse-rank
	// turn order is therefore p3, p2, p1.
	players := make([]dbgen.Player, 3)
	for i := range players {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: fmt.Sprintf("su-p%d-%s", i+1, randSuffix()), CodeHash: "x",
		})
		require.NoError(t, err)
		p, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID: game.ID, DisplayName: fmt.Sprintf("P%d", i+1), AccountID: acct.ID,
		})
		require.NoError(t, err)
		players[i] = p
		require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: game.ID, PlayerID: &players[i].ID, Category: model.CategoryEsteem, Rank: int16(i + 1),
		}))
		_, err = q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{ID: p.ID, ShakeUpTokens: 5})
		require.NoError(t, err)
	}

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
