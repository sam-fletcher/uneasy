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

// TestRunRankingUpdate_TokenHolderClimbsTokensNotCleared pins that a power-track
// token holder climbs one rank (swapping with the player above), and that the
// power sheet's tokens are NOT cleared when only one of its four plans was
// prepared. (Clearing happens only when every plan on the sheet has a holder —
// see TestRunRankingUpdate_FullSheetClears.)
func TestRunRankingUpdate_TokenHolderClimbsTokensNotCleared(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Default power ranks: p0=1, p1=2, p2=3.
	tg := newTestGame(t, q, 3)
	gameID := tg.Game.ID

	// p2 (rank 3) holds a token on one power-sheet plan (Exchange Courtiers).
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

	// Only one of the four power-sheet plans was prepared → tokens stay put.
	remaining, err := q.ListPlanTokensByGame(ctx, gameID)
	require.NoError(t, err)
	assert.Len(t, remaining, 1, "a partially-prepared sheet keeps its tokens")

	// The narration diff describes the climb and the resulting standing.
	require.NotNil(t, diff)
	var power *rankingCategoryDiff
	for i := range diff.Categories {
		if diff.Categories[i].Category == model.CategoryPower {
			power = &diff.Categories[i]
		}
	}
	require.NotNil(t, power, "diff includes the power category")
	require.False(t, power.Cleared, "a partially-prepared sheet is not cleared")

	require.Len(t, power.Plans, 1, "one prepared plan on the power track")
	require.Equal(t, model.PlanExchangeCourtiers, power.Plans[0].PlanType)
	require.Len(t, power.Plans[0].Movers, 1)
	assert.Equal(t, tg.Players[2].DisplayName, power.Plans[0].Movers[0].Name)
	assert.Equal(t, "up", power.Plans[0].Movers[0].Glyph, "the climbing holder shows an up-arrow")

	// Standing lists occupied ranks only — three real players, no dummy fillers.
	require.Len(t, power.Final, 3)
	assert.Equal(t, rankStanding{Rank: 1, Name: tg.Players[0].DisplayName}, power.Final[0])
	assert.Equal(t, rankStanding{Rank: 2, Name: tg.Players[2].DisplayName}, power.Final[1])
	assert.Equal(t, rankStanding{Rank: 3, Name: tg.Players[1].DisplayName}, power.Final[2])
}

// TestRunRankingUpdate_MultiTokenHolderClimbsPerToken pins the fix for the
// per-player de-dup bug: a player who prepared two plans on one sheet climbs
// once per token, resolved bottom-most plan first. p2 (rank 3) holds Make War
// (bottom of the power sheet) and Exchange Courtiers, so it climbs 3 → 2 → 1.
func TestRunRankingUpdate_MultiTokenHolderClimbsPerToken(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	// Default power ranks: p0=1, p1=2, p2=3.
	tg := newTestGame(t, q, 3)
	gameID := tg.Game.ID

	for _, pt := range []model.PlanType{model.PlanMakeWar, model.PlanExchangeCourtiers} {
		plan := createPlanOnRow(t, q, &tg.Game, &tg.Players[2], pt, model.CategoryPower, 4)
		_, err := q.CreatePlanToken(ctx, dbgen.CreatePlanTokenParams{
			GameID:   gameID,
			PlanType: pt,
			PlayerID: tg.Players[2].ID,
			PlanID:   plan.ID,
		})
		require.NoError(t, err)
	}

	updated, diff, err := runRankingUpdate(ctx, q, gameID)
	require.NoError(t, err)

	powerRank := map[int64]int16{}
	for _, r := range updated {
		if r.Category == model.CategoryPower && r.PlayerID != nil {
			powerRank[*r.PlayerID] = r.Rank
		}
	}
	assert.EqualValues(t, 1, powerRank[tg.Players[2].ID], "two tokens → climbs 3 → 2 → 1")
	assert.EqualValues(t, 2, powerRank[tg.Players[0].ID], "displaced to rank 2")
	assert.EqualValues(t, 3, powerRank[tg.Players[1].ID], "displaced to rank 3")

	// Only two of the four power-sheet plans were prepared → tokens stay put.
	remaining, err := q.ListPlanTokensByGame(ctx, gameID)
	require.NoError(t, err)
	assert.Len(t, remaining, 2, "partially-prepared sheet keeps its tokens")

	// The diff lists both plans bottom-to-top: Make War before Exchange Courtiers.
	var power *rankingCategoryDiff
	for i := range diff.Categories {
		if diff.Categories[i].Category == model.CategoryPower {
			power = &diff.Categories[i]
		}
	}
	require.NotNil(t, power)
	require.Len(t, power.Plans, 2)
	assert.Equal(t, model.PlanMakeWar, power.Plans[0].PlanType, "bottom-most plan narrated first")
	assert.Equal(t, model.PlanExchangeCourtiers, power.Plans[1].PlanType)
}

// TestRunRankingUpdate_FullSheetClears pins that once every plan on a sheet has
// at least one token, the whole sheet's tokens are cleared back to their
// players after the update.
func TestRunRankingUpdate_FullSheetClears(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	tg := newTestGame(t, q, 3)
	gameID := tg.Game.ID

	// p0 (rank 1) prepares all four power-sheet plans, so every plan on the
	// sheet has a token. (Rank 1 can't climb, but that's irrelevant to clearing.)
	for _, pt := range categorySheetPlans[model.CategoryPower] {
		plan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0], pt, model.CategoryPower, 4)
		_, err := q.CreatePlanToken(ctx, dbgen.CreatePlanTokenParams{
			GameID:   gameID,
			PlanType: pt,
			PlayerID: tg.Players[0].ID,
			PlanID:   plan.ID,
		})
		require.NoError(t, err)
	}

	_, diff, err := runRankingUpdate(ctx, q, gameID)
	require.NoError(t, err)

	remaining, err := q.ListPlanTokensByGame(ctx, gameID)
	require.NoError(t, err)
	assert.Empty(t, remaining, "a fully-prepared sheet clears its tokens after the update")

	var power *rankingCategoryDiff
	for i := range diff.Categories {
		if diff.Categories[i].Category == model.CategoryPower {
			power = &diff.Categories[i]
		}
	}
	require.NotNil(t, power)
	assert.True(t, power.Cleared, "fully-prepared sheet is marked cleared")
}
