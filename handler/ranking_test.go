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
			require.GreaterOrEqual(t, rank, int16(1))
			require.LessOrEqual(t, rank, int16(5))
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
		assert.True(t, exists, "player %d not in rank map for category %s", playerID, cat)
		assert.Equal(t, expectedRank, actual, "player %d rank in %s", playerID, cat)
	}

	// Helper to assert token-clearing decision.
	assertTokenClear := func(result map[model.RankingCategory]bool, cat model.RankingCategory, shouldClear bool) {
		actual, exists := result[cat]
		assert.True(t, exists, "category %s not in result map", cat)
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

		result, _ := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

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

		result, _ := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Player 1 should still be at rank 1.
		assertRank(pr, 1, model.CategoryPower, 1)
		assertRank(pr, 2, model.CategoryPower, 2)
		// Having one token in Phase 2 means all plan types have tokens → should clear.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("token holder skips past nil to reach next real player", func(t *testing.T) {
		// Player 1 at rank 3, rank 2 is unoccupied (nil).
		// Give player 1 a token; should skip past rank 2 and swap with player 2 at rank 1.
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

		result, _ := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Player 1 should now be at rank 1 (skipped rank 2), player 2 at rank 3.
		assertRank(pr, 1, model.CategoryPower, 1)
		assertRank(pr, 2, model.CategoryPower, 3)
		// Having one token in Phase 2 means all plan types have tokens → should clear.
		assertTokenClear(result, model.CategoryPower, true)
	})

	t.Run("two adjacent token holders on one plan cancel each other", func(t *testing.T) {
		// Player 1 at rank 3, player 2 at rank 2 (adjacent), both stacked on the
		// same plan. Tokens resolve bottom-of-stack first; the bottom token is the
		// first preparer, which (by the eligibility rules) is the worse-ranked one
		// — here player 1 (id 1), then player 2 (id 2). Live ranks mean:
		//   - process player 1 (rank 3): 3 → 2, player 2 displaced 2 → 3.
		//   - process player 2 (now rank 3): 3 → 2, player 1 displaced 2 → 3.
		// Net: they swap back to their original positions.
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				3: intPtr(1),
				2: intPtr(2),
				1: intPtr(3),
			})
		pr := buildPlayerRank(slots)

		tokens := []dbgen.PlanToken{
			{ID: 1, PlayerID: 1, PlanType: model.PlanExchangeCourtiers},
			{ID: 2, PlayerID: 2, PlanType: model.PlanExchangeCourtiers},
		}
		catPlanTypes := map[model.RankingCategory][]model.PlanType{
			model.CategoryPower: {model.PlanExchangeCourtiers},
		}

		result, _ := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// After two adjacent token holders swap, they should return to original positions.
		assertRank(pr, 1, model.CategoryPower, 3)
		assertRank(pr, 2, model.CategoryPower, 2)
		assertRank(pr, 3, model.CategoryPower, 1)
		// The sole listed plan type has a token → sheet is fully prepared → clear.
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

		result, _ := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

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

		result, _ := applyRankingSwaps(slots, pr, tokens, catPlanTypes)

		// Power category: has one token of one plan type → clear.
		// Knowledge category: has no tokens → don't clear.
		// Esteem category: has no tokens → don't clear.
		assertTokenClear(result, model.CategoryPower, true)
		assertTokenClear(result, model.CategoryKnowledge, false)
		assertTokenClear(result, model.CategoryEsteem, false)
	})

	t.Run("one player holding two plans in a category climbs once per token", func(t *testing.T) {
		// Player 3 (rank 3) holds tokens on two different power-sheet plans.
		// Each token is its own swap, so player 3 climbs twice: 3 → 2 → 1. This is
		// the case the old per-player de-dup got wrong.
		slots := mkSlots(model.CategoryPower,
			map[int16]*int64{
				1: intPtr(1),
				2: intPtr(2),
				3: intPtr(3),
			})
		pr := buildPlayerRank(slots)

		// Bottom-most plan (Make War) resolves first, then Exchange Courtiers.
		tokens := []dbgen.PlanToken{
			{ID: 1, PlayerID: 3, PlanType: model.PlanMakeWar},
			{ID: 2, PlayerID: 3, PlanType: model.PlanExchangeCourtiers},
		}

		result, _ := applyRankingSwaps(slots, pr, tokens, categorySheetPlans)

		assertRank(pr, 3, model.CategoryPower, 1)
		assertRank(pr, 1, model.CategoryPower, 2)
		assertRank(pr, 2, model.CategoryPower, 3)
		// Only 2 of the 4 power-sheet plans had tokens → sheet not cleared.
		assertTokenClear(result, model.CategoryPower, false)
	})
}

func Test_swapTokenPlayerWithAbove(t *testing.T) {
	intPtr := func(v int64) *int64 { return &v }

	tests := []struct {
		name              string
		pid               int64
		cat               model.RankingCategory
		s                 *categorySlots
		playerRank        map[int64]map[model.RankingCategory]int16
		expectedPIDRank   int16 // expected rank of pid after swap
		expectedAboveRank int16 // expected rank of player who was above
	}{
		{
			name: "basic swap advances player one rank",
			pid:  1,
			cat:  model.CategoryPower,
			s: &categorySlots{
				intPtr(3), // rank 1: player 3
				intPtr(2), // rank 2: player 2
				intPtr(1), // rank 3: player 1
				nil,       // rank 4: nil
				nil,       // rank 5: nil
			},
			playerRank: map[int64]map[model.RankingCategory]int16{
				1: {model.CategoryPower: 3},
				2: {model.CategoryPower: 2},
				3: {model.CategoryPower: 1},
			},
			expectedPIDRank:   2, // player 1 moves to rank 2
			expectedAboveRank: 3, // player 2 moves to rank 3
		},
		{
			name: "player at rank 1 does not move",
			pid:  1,
			cat:  model.CategoryPower,
			s: &categorySlots{
				intPtr(1), // rank 1: player 1
				intPtr(2), // rank 2: player 2
				nil,       // rank 3
				nil,       // rank 4
				nil,       // rank 5
			},
			playerRank: map[int64]map[model.RankingCategory]int16{
				1: {model.CategoryPower: 1},
				2: {model.CategoryPower: 2},
			},
			expectedPIDRank:   1, // player 1 stays at rank 1
			expectedAboveRank: 0, // no change to player above
		},
		{
			name: "skips past nil slots to find next real player",
			pid:  1,
			cat:  model.CategoryPower,
			s: &categorySlots{
				intPtr(2), // rank 1: player 2 (real player)
				nil,       // rank 2: nil (dummy)
				nil,       // rank 3: nil (dummy)
				intPtr(1), // rank 4: player 1
				nil,       // rank 5: nil
			},
			playerRank: map[int64]map[model.RankingCategory]int16{
				1: {model.CategoryPower: 4},
				2: {model.CategoryPower: 1},
			},
			expectedPIDRank:   1, // player 1 swaps with player 2, moves to rank 1
			expectedAboveRank: 4, // player 2 moves to rank 4
		},
		{
			name: "cannot advance when all ranks above are nil",
			pid:  1,
			cat:  model.CategoryPower,
			s: &categorySlots{
				nil,       // rank 1: nil (no one above)
				nil,       // rank 2: nil
				intPtr(1), // rank 3: player 1
				nil,       // rank 4
				nil,       // rank 5
			},
			playerRank: map[int64]map[model.RankingCategory]int16{
				1: {model.CategoryPower: 3},
			},
			expectedPIDRank:   3, // player 1 stays at rank 3 (no real player above)
			expectedAboveRank: 0, // no change
		},
		{
			name: "swap with live rank updates",
			pid:  2,
			cat:  model.CategoryKnowledge,
			s: &categorySlots{
				intPtr(4), // rank 1: player 4
				intPtr(2), // rank 2: player 2
				intPtr(3), // rank 3: player 3
				nil,
				nil,
			},
			playerRank: map[int64]map[model.RankingCategory]int16{
				2: {model.CategoryKnowledge: 2},
				3: {model.CategoryKnowledge: 3},
				4: {model.CategoryKnowledge: 1},
			},
			expectedPIDRank:   1, // player 2 moves to rank 1
			expectedAboveRank: 2, // player 4 moves to rank 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find which player currently occupies the higher rank we're swapping into.
			var playerAbove int64
			for i := range 5 {
				if tt.s[i] != nil && int16(i+1) == tt.expectedPIDRank {
					playerAbove = *tt.s[i]
					break
				}
			}

			swapTokenPlayerWithAbove(tt.pid, tt.cat, tt.s, tt.playerRank)

			// Verify player's rank after swap
			if tt.expectedPIDRank > 0 {
				assert.Equal(t, tt.expectedPIDRank, tt.playerRank[tt.pid][tt.cat],
					"player %d should be at rank %d", tt.pid, tt.expectedPIDRank)
			}

			// Verify player above's rank after swap (if applicable)
			if tt.expectedAboveRank > 0 && playerAbove > 0 {
				assert.Equal(t, tt.expectedAboveRank, tt.playerRank[playerAbove][tt.cat],
					"player %d should be at rank %d", playerAbove, tt.expectedAboveRank)
			}
		})
	}
}

// TestApplyRankingSwaps_SwapFlags pins the per-holder "did an up-swap happen"
// signal that drives the chat glyph: true → up-arrow, false → crown (no real
// player above to overtake). The key case is the adjacent cancel, where two
// holders both swap (both true) yet the board nets back to where it started —
// the arrows are the operations, not the net result.
func TestApplyRankingSwaps_SwapFlags(t *testing.T) {
	intPtr := func(v int64) *int64 { return &v }
	cat := model.CategoryPower
	catPlanTypes := map[model.RankingCategory][]model.PlanType{
		cat: {model.PlanExchangeCourtiers},
	}
	// tok builds a token with an explicit id so the stack resolves in a
	// deterministic placement order (lower id = bottom of stack = resolved first).
	tok := func(id, pid int64) dbgen.PlanToken {
		return dbgen.PlanToken{ID: id, PlayerID: pid, PlanType: model.PlanExchangeCourtiers}
	}
	// setup builds the slots + live rank map for the power track from a
	// rank→playerID layout, then runs the swaps and returns the per-holder flags
	// for the (single) Exchange Courtiers plan.
	setup := func(layout map[int16]int64, tokens []dbgen.PlanToken) (
		map[model.RankingCategory]*categorySlots, map[int64]bool,
	) {
		slots := map[model.RankingCategory]*categorySlots{
			model.CategoryPower:     new(categorySlots),
			model.CategoryKnowledge: new(categorySlots),
			model.CategoryEsteem:    new(categorySlots),
		}
		pr := make(map[int64]map[model.RankingCategory]int16)
		for rank, pid := range layout {
			slots[cat][rank-1] = intPtr(pid)
			pr[pid] = map[model.RankingCategory]int16{cat: rank}
		}
		_, swapped := applyRankingSwaps(slots, pr, tokens, catPlanTypes)
		return slots, swapped[cat][model.PlanExchangeCourtiers]
	}
	// rankOf reads a player's final rank out of the slot array.
	rankOf := func(s *categorySlots, pid int64) int16 {
		for i := range s {
			if s[i] != nil && *s[i] == pid {
				return int16(i + 1)
			}
		}
		return 0
	}

	t.Run("holder with a real rival above swaps (up)", func(t *testing.T) {
		_, flags := setup(map[int16]int64{1: 10, 2: 20, 3: 30}, []dbgen.PlanToken{tok(1, 30)})
		assert.True(t, flags[30], "rank-3 holder overtakes rank-2 player")
	})

	t.Run("holder at the top does not swap (crown)", func(t *testing.T) {
		_, flags := setup(map[int16]int64{1: 10, 2: 20}, []dbgen.PlanToken{tok(1, 10)})
		assert.False(t, flags[10], "rank-1 holder has no one to overtake")
	})

	t.Run("holder above only dummies does not swap (crown)", func(t *testing.T) {
		// Ranks 1-2 empty (dummies); the rank-3 holder has no real rival above.
		_, flags := setup(map[int16]int64{3: 30, 4: 40}, []dbgen.PlanToken{tok(1, 30)})
		assert.False(t, flags[30], "top real player can't climb past dummies")
	})

	t.Run("adjacent holders both swap and net to no movement", func(t *testing.T) {
		// 1:10 (no token), 2:20 (token), 3:30 (token), both stacked on the same
		// plan. The bottom token (first preparer, worse rank → player 30, id 1)
		// resolves first, then player 20 (id 2). Both swap; the board returns to
		// its starting order, but both flags are true.
		s, flags := setup(map[int16]int64{1: 10, 2: 20, 3: 30},
			[]dbgen.PlanToken{tok(1, 30), tok(2, 20)})
		assert.True(t, flags[20], "preparer 20 performed an up-swap")
		assert.True(t, flags[30], "preparer 30 performed an up-swap")
		assert.EqualValues(t, 2, rankOf(s[cat], 20), "board nets back to the start")
		assert.EqualValues(t, 3, rankOf(s[cat], 30), "board nets back to the start")
	})
}

// TestFindFirstFocusPlayer pins the underdog selection: per PROLOGUE_RULES.md
// the first focus player is the LOWEST-status player, and since rank 1 is the
// highest status, that means the highest sum of ranks across the three tracks.
// Ties go to the lowest-status-on-power player (highest power rank number).
func TestFindFirstFocusPlayer(t *testing.T) {
	intPtr := func(v int64) *int64 { return &v }
	rk := func(pid int64, cat model.RankingCategory, rank int16) dbgen.Ranking {
		return dbgen.Ranking{PlayerID: intPtr(pid), Category: cat, Rank: rank}
	}

	t.Run("highest rank sum (underdog) is chosen, not the leader", func(t *testing.T) {
		players := []dbgen.Player{{ID: 1}, {ID: 2}}
		rankings := []dbgen.Ranking{
			// p1 is the leader (sum 3), p2 is the underdog (sum 12).
			rk(1, model.CategoryPower, 1), rk(1, model.CategoryKnowledge, 1), rk(1, model.CategoryEsteem, 1),
			rk(2, model.CategoryPower, 4), rk(2, model.CategoryKnowledge, 4), rk(2, model.CategoryEsteem, 4),
		}
		got := findFirstFocusPlayer(&dbgen.Game{}, players, rankings)
		require.NotNil(t, got)
		assert.Equal(t, int64(2), got.ID, "underdog (highest rank sum) takes the marker")
	})

	t.Run("tie on total broken by lowest power status", func(t *testing.T) {
		players := []dbgen.Player{{ID: 1}, {ID: 2}}
		rankings := []dbgen.Ranking{
			// Both total 9. p1 lower-status on power (rank 5) → p1 wins.
			rk(1, model.CategoryPower, 5), rk(1, model.CategoryKnowledge, 2), rk(1, model.CategoryEsteem, 2),
			rk(2, model.CategoryPower, 1), rk(2, model.CategoryKnowledge, 4), rk(2, model.CategoryEsteem, 4),
		}
		got := findFirstFocusPlayer(&dbgen.Game{}, players, rankings)
		require.NotNil(t, got)
		assert.Equal(t, int64(1), got.ID, "lowest-status-on-power wins the tie")
	})

	t.Run("explicit focus player overrides computation", func(t *testing.T) {
		players := []dbgen.Player{{ID: 1}, {ID: 2}}
		got := findFirstFocusPlayer(&dbgen.Game{FocusPlayerID: intPtr(2)}, players, nil)
		require.NotNil(t, got)
		assert.Equal(t, int64(2), got.ID)
	})
}

// TestCategorySheetPlansMatchRegistry guards that categorySheetPlans (the
// hardcoded composition of each ranking sheet) stays in lockstep with the plan
// registry: every registered plan appears exactly once, filed under the
// category its handler reports. This is the canary for the original bug, where
// the sheet map listed only one plan per category — so untracked plans were
// silently ignored by the ranking update.
func TestCategorySheetPlansMatchRegistry(t *testing.T) {
	seen := map[model.PlanType]bool{}
	for cat, plans := range categorySheetPlans {
		for _, pt := range plans {
			require.False(t, seen[pt], "plan %s listed more than once in categorySheetPlans", pt)
			seen[pt] = true
			h, ok := GetHandler(pt)
			require.True(t, ok, "plan %s in categorySheetPlans has no registered handler", pt)
			assert.Equal(t, cat, h.Metadata().Category, "plan %s filed under the wrong category", pt)
		}
	}
	for pt := range AllHandlers() {
		assert.True(t, seen[pt], "registered plan %s is missing from categorySheetPlans", pt)
	}
}
