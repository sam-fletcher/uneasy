//go:build integration

// handler/ranking_update_integration_test.go — characterization test for the
// DB round-trip of the engrailed-line ranking update (Step 2 of the testability
// roadmap).
//
// The pure swap algorithm (applyRankingSwaps / swapTokenPlayerWithAbove) is
// already covered by ranking_test.go. The gap this fills is runRankingUpdate's
// storage-coupled wrapper: read rankings + plan tokens from the DB, apply the
// swaps, write the slots back, and clear a fully-tokened category. That wrapper
// is exactly the kind of DB-interleaved engine code Option E will move, so we
// pin its current behavior end-to-end against a real database.
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// TestRunRankingUpdate_TokenHolderClimbsAndTokensCleared pins that a power-track
// token holder climbs one rank (swapping with the player above), and that the
// power category's tokens are cleared once its only plan type has a holder.
func TestRunRankingUpdate_TokenHolderClimbsAndTokensCleared(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Default power ranks: p0=1, p1=2, p2=3.
	tg := newTestGame(t, q, 3)
	gameID := tg.Game.ID

	// p2 (rank 3) holds a token on the power category's plan sheet
	// (Exchange Courtiers, per runRankingUpdate's categoryPlanTypes).
	plan := createPlanOnRow(t, q, &tg.Game, &tg.Players[2],
		model.PlanExchangeCourtiers, model.CategoryPower, 4)
	_, err := q.CreatePlanToken(ctx, dbgen.CreatePlanTokenParams{
		GameID:   gameID,
		PlanType: model.PlanExchangeCourtiers,
		PlayerID: tg.Players[2].ID,
		PlanID:   plan.ID,
	})
	require.NoError(t, err)

	updated, diff, err := runRankingUpdate(ctx, q, gameID)
	require.NoError(t, err)

	// Read power ranks out of the returned slice.
	powerRank := map[int64]int16{}
	for _, r := range updated {
		if r.Category == model.CategoryPower && r.PlayerID != nil {
			powerRank[*r.PlayerID] = r.Rank
		}
	}
	assert.EqualValues(t, 2, powerRank[tg.Players[2].ID], "token holder climbs 3 → 2")
	assert.EqualValues(t, 3, powerRank[tg.Players[1].ID], "player above is displaced 2 → 3")
	assert.EqualValues(t, 1, powerRank[tg.Players[0].ID], "rank-1 player is untouched")

	// The power sheet's only plan type had a holder → its tokens are cleared.
	remaining, err := q.ListPlanTokensByGame(ctx, gameID)
	require.NoError(t, err)
	assert.Empty(t, remaining, "a fully-tokened category clears its tokens after the update")

	// The narration diff describes the climb and the resulting standing.
	require.NotNil(t, diff)
	var power *rankingCategoryDiff
	for i := range diff.Categories {
		if diff.Categories[i].Category == model.CategoryPower {
			power = &diff.Categories[i]
		}
	}
	require.NotNil(t, power, "diff includes the power category")
	require.True(t, power.Cleared, "fully-prepared category is marked cleared")

	require.Len(t, power.Plans, 1, "one prepared plan on the power track")
	require.Equal(t, model.PlanExchangeCourtiers, power.Plans[0].PlanType)
	require.Len(t, power.Plans[0].Movers, 1)
	assert.Equal(t, tg.Players[2].DisplayName, power.Plans[0].Movers[0].Name)
	assert.Equal(t, "up", power.Plans[0].Movers[0].Glyph, "the climbing holder shows an up-arrow")

	// Standing is rank 1→5: p0 untouched, p2 climbed over p1, then dummy fillers.
	require.Len(t, power.Final, 5)
	assert.Equal(t, tg.Players[0].DisplayName, power.Final[0], "rank 1")
	assert.Equal(t, tg.Players[2].DisplayName, power.Final[1], "rank 2")
	assert.Equal(t, tg.Players[1].DisplayName, power.Final[2], "rank 3")
	assert.Equal(t, "Dummy", power.Final[3], "empty slot labelled Dummy")
}
