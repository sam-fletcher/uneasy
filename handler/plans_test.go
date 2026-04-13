package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// TestComputeDifficultyPure tests the pure difficulty computation logic.
func TestComputeDifficultyPure(t *testing.T) {
	t.Run("exchange courtiers: difficulty equals target status (6 - rank)", func(t *testing.T) {
		plan := &dbgen.Plan{
			PlanType: model.PlanExchangeCourtiers,
		}

		testCases := []struct {
			name               string
			targetRank         int16
			expectedDifficulty int16
		}{
			{"rank 1 → status 5, difficulty 5", 1, 5},
			{"rank 2 → status 4, difficulty 4", 2, 4},
			{"rank 3 → status 3, difficulty 3", 3, 3},
			{"rank 4 → status 2, difficulty 2", 4, 2},
			{"rank 5 → status 1, difficulty 1", 5, 1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				diff, err := computeDifficultyPure(plan.PlanType, planResData{}, tc.targetRank)
				require.NoError(t, err, "should not error")
				assert.Equal(t, tc.expectedDifficulty, diff, "difficulty should be 6 - rank, with safety min of 1")
			})
		}
	})

	t.Run("make introductions: difficulty equals 2 + peer_count", func(t *testing.T) {
		testCases := []struct {
			name               string
			peerCount          int16
			expectedDifficulty int16
		}{
			{"0 peers defaults to 1 → difficulty 3", 0, 3},
			{"1 peer → difficulty 3", 1, 3},
			{"2 peers → difficulty 4", 2, 4},
			{"3 peers → difficulty 5", 3, 5},
			{"4 peers → difficulty 6", 4, 6},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resData := planResData{PeerCount: tc.peerCount}
				diff, err := computeDifficultyPure(model.PlanMakeIntroductions, resData, 0)
				require.NoError(t, err, "should not error (rank param ignored for this plan type)")
				assert.Equal(t, tc.expectedDifficulty, diff, "difficulty should be 2 + peer_count, treating 0 as 1")
			})
		}
	})

	t.Run("spread propaganda: difficulty equals preparer rank", func(t *testing.T) {
		testCases := []struct {
			name               string
			preparerRank       int16
			expectedDifficulty int16
		}{
			{"rank 1 → difficulty 1", 1, 1},
			{"rank 2 → difficulty 2", 2, 2},
			{"rank 3 → difficulty 3", 3, 3},
			{"rank 4 → difficulty 4", 4, 4},
			{"rank 5 → difficulty 5", 5, 5},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				diff, err := computeDifficultyPure(model.PlanSpreadPropaganda, planResData{}, tc.preparerRank)
				require.NoError(t, err, "should not error")
				assert.Equal(t, tc.expectedDifficulty, diff, "difficulty should equal preparer rank")
			})
		}
	})

	t.Run("unsupported plan type returns error", func(t *testing.T) {
		diff, err := computeDifficultyPure(model.PlanMakeDemands, planResData{}, 1)
		require.Error(t, err, "unsupported plan type should error")
		assert.Equal(t, int16(0), diff, "unsupported plan should return 0 difficulty")
	})

	t.Run("make introductions with peer_count 0 treats as 1", func(t *testing.T) {
		// Verify that peer_count: 0 is treated as 1 (using max(pc, 1)),
		// resulting in difficulty 2 + 1 = 3.
		resData := planResData{PeerCount: 0}
		diff, err := computeDifficultyPure(model.PlanMakeIntroductions, resData, 0)
		require.NoError(t, err)
		assert.Equal(t, int16(3), diff)
	})

	t.Run("rank boundary: min safety check on exchange courtiers", func(t *testing.T) {
		// The implementation uses max(6 - rank, 1) to ensure difficulty >= 1.
		// This test verifies that even with rank == 0 (shouldn't happen but test robustness),
		// we get a minimum of 1. Actually, rank should be 1-5, so this is defensive.
		diff, err := computeDifficultyPure(model.PlanExchangeCourtiers, planResData{}, 6)
		require.NoError(t, err, "should handle edge case gracefully")
		// 6 - 6 = 0, but max(0, 1) = 1
		assert.Equal(t, int16(1), diff, "should have minimum safety check")
	})
}
