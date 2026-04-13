package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// TestApplyRankingSwaps tests the pure ranking swap algorithm.
func TestApplyRankingSwaps(t *testing.T) {
	// Helper to create a pointer to int64.
	intPtr := func(v int64) *int64 { return &v }

	// Helper to create slots with players at specific ranks.
	mkSlots := func(cat model.RankingCategory, players map[int16]*int64) map[model.RankingCategory]*categorySlots {
		slots := make(map[model.RankingCategory]*categorySlots)
		for _, c := range []model.RankingCategory{model.CategoryPower, model.CategoryKnowledge, model.CategoryEsteem} {
			slots[c] = new(categorySlots)
		}
		for rank, pid := range players {
			if rank < 1 || rank > 5 {
				t.Fatalf("invalid rank %d", rank)
			}
			slots[cat][rank-1] = pid
		}
		return slots
	}

	// Helper to build playerRank map from slots.
	buildPlayerRank := func(slots map[model.RankingCategory]*categorySlots) map[int64]map[model.RankingCategory]int16 {
		pr := make(map[int64]map[model.RankingCategory]int16)
		for cat, s := range slots {
			for rank, pid := range s {
				if pid == nil {
					continue
				}
				if _, exists := pr[*pid]; !exists {
					pr[*pid] = make(map[model.RankingCategory]int16)
				}
				pr[*pid][cat] = int16(rank + 1)
			}
		}
		return pr
	}

	// Helper to assert a player's rank in a category.
	assertRank := func(pr map[int64]map[model.RankingCategory]int16, playerID int64, cat model.RankingCategory, expectedRank int16) {
		actual, exists := pr[playerID][cat]
		require.True(t, exists, "player %d not in rank map for category %s", playerID, cat)
		assert.Equal(t, expectedRank, actual, "player %d rank in %s", playerID, cat)
	}

	// Helper to assert token-clearing decision.
	assertTokenClear := func(result map[model.RankingCategory]bool, cat model.RankingCategory, shouldClear bool) {
		actual, exists := result[cat]
		require.True(t, exists, "category %s not in result map", cat)
		assert.Equal(t, shouldClear, actual, "token clear decision for %s", cat)
	}

	t.Run("single token holder moves up one rank", func(t *testing.T) {
		// Player 1 at rank 3, player 2 at rank 2, player 3 at rank 1.
		// Give player 1 a token; they should swap with player 2.
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				1: intPtr(3),
				2: intPtr(2),
				3: intPtr(1),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{PlayerID: 1, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Player 1 should now be at rank 2, player 2 at rank 3.
		assertRank(pr, 1, model.CategoryPower, 2)
		assertRank(pr, 2, model.CategoryPower, 3)
		assertRank(pr, 3, model.CategoryPower, 1)

		// Single token-holding plan type in Phase 2, so having one token means
		// all plan types have tokens → tokens SHOULD be cleared.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("token holder already at rank 1, no-op", func(t *testing.T) {
		// Player 1 at rank 1 (already top), player 2 at rank 2.
		// Give player 1 a token; should not move.
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				1: intPtr(1),
				2: intPtr(2),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{PlayerID: 1, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Player 1 should still be at rank 1.
		assertRank(pr, 1, model.CategoryPower, 1)
		assertRank(pr, 2, model.CategoryPower, 2)
		// Having one token in Phase 2 means all plan types have tokens → should clear.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("token holder with nil slot above, cannot advance past unoccupied position", func(t *testing.T) {
		// Player 1 at rank 3, rank 2 is unoccupied (nil).
		// Give player 1 a token; should NOT advance (can't pass dummy).
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				3: intPtr(1),
				// rank 2 is nil
				1: intPtr(2),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{PlayerID: 1, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Player 1 should still be at rank 3.
		assertRank(pr, 1, model.CategoryPower, 3)
		assertRank(pr, 2, model.CategoryPower, 1)
		// Having one token in Phase 2 means all plan types have tokens → should clear.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("two adjacent token holders cancel each other", func(t *testing.T) {
		// Player 1 at rank 3, player 2 at rank 2 (adjacent).
		// Both have tokens. Worst-to-best processing: player 1 (rank 3) swaps up to rank 2,
		// displacing player 2 to rank 3. Then player 2 (now at rank 3) might swap up,
		// but the algorithm processes worst-to-best in initial order, so player 2's
		// updated position isn't re-processed. Actually wait, let me re-read the algorithm...
		//
		// Looking at the code: playerRank is kept live, so after player 1 swaps,
		// player 2's rank is updated in the live map. But tokenPlayers list was built
		// before the swaps, so we process: player 1 first (rank 3), then player 2 (now rank 3).
		// When we process player 2 at their new rank 3, they might swap again.
		// Actually, the comment says "Two token holders at adjacent ranks will effectively
		// cancel each other", so let's test that.
		//
		// Initial: player 1 rank 3, player 2 rank 2.
		// Process player 1 (worst): rank 3 → rank 2, player 2 goes rank 2 → rank 3.
		// Process player 2 (now at rank 3): rank 3 → rank 2, player 1 goes rank 2 → rank 3.
		// Net: they swapped back to original positions.
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				3: intPtr(1),
				2: intPtr(2),
				1: intPtr(3),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{PlayerID: 1, PlanType: model.PlanExchangeCourtiers},
			{PlayerID: 2, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// After two adjacent token holders swap, they should return to original positions.
		assertRank(pr, 1, model.CategoryPower, 3)
		assertRank(pr, 2, model.CategoryPower, 2)
		assertRank(pr, 3, model.CategoryPower, 1)
		// Having tokens in Phase 2 means all plan types have tokens → should clear.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("token clearing when all plan types have tokens", func(t *testing.T) {
		// Power category: only one plan type (ExchangeCourtiers in Phase 2).
		// Give a token → all plan types have tokens → should clear.
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				1: intPtr(1),
				2: intPtr(2),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{PlayerID: 1, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// In Phase 2, only one plan type per category, so having one token means
		// all plan types have tokens.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("multiple categories processed independently", func(t *testing.T) {
		// Set up two categories: power and knowledge.
		// Give token in power only; knowledge should not clear.
		slots := map[model.RankingCategory]*categorySlots{
			model.CategoryPower: {
				intPtr(1), intPtr(2), nil, nil, nil,
			},
			model.CategoryKnowledge: {
				intPtr(3), intPtr(4), nil, nil, nil,
			},
			model.CategoryEsteem: {
				nil, nil, nil, nil, nil,
			},
		}
		pr := map[int64]map[model.RankingCategory]int16{
			1: {model.CategoryPower: 1, model.CategoryKnowledge: 0},
			2: {model.CategoryPower: 2, model.CategoryKnowledge: 0},
			3: {model.CategoryPower: 0, model.CategoryKnowledge: 1},
			4: {model.CategoryPower: 0, model.CategoryKnowledge: 2},
		}

		tokens := []dbgen.PlanToken{
			{PlayerID: 1, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower:     {model.PlanExchangeCourtiers},
			model.CategoryKnowledge: {model.PlanMakeIntroductions},
			model.CategoryEsteem:    {model.PlanSpreadPropaganda},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Power category: has one token of one plan type → clear.
		// Knowledge category: has no tokens → don't clear.
		// Esteem category: has no tokens → don't clear.
		assertTokenClear(result, model.CategoryPower, true)
		assertTokenClear(result, model.CategoryKnowledge, false)
		assertTokenClear(result, model.CategoryEsteem, false)
	})

	t.Run("three-player game with unoccupied ranks", func(t *testing.T) {
		// Three players: ranks 1, 3, 5 occupied; ranks 2, 4 unoccupied (dummy tokens).
		// Player 1 rank 3 with token: should NOT move because rank 2 above is nil (dummy).
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				1: intPtr(1),
				// 2: nil (dummy)
				3: intPtr(2),
				// 4: nil (dummy)
				5: intPtr(3),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{PlayerID: 2, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Player 2 at rank 3 cannot move past the nil (dummy) at rank 2.
		assertRank(pr, 2, model.CategoryPower, 3)
		// Having one token in Phase 2 means all plan types have tokens → should clear.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("stacked tokens on same plan type deduplicated", func(t *testing.T) {
		// Player 3 has two tokens on the same plan type (stacked).
		// Should only process player 3 once.
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				1: intPtr(2),
				2: intPtr(1),
				3: intPtr(3),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{PlayerID: 3, PlanType: model.PlanExchangeCourtiers},
			{PlayerID: 3, PlanType: model.PlanExchangeCourtiers}, // duplicate
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Player 3 at rank 3 should move to rank 2 (only processed once despite stacked tokens).
		assertRank(pr, 3, model.CategoryPower, 2)
		assertTokenClear(result, model.CategoryPower, true)
	})
}
