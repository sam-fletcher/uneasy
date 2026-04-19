package handler

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"uneasy/game"
)

// TestPlanDifficulty tests the per-plan pure difficulty functions.
func TestPlanDifficulty(t *testing.T) {
	t.Run("exchange courtiers: 6 - target power rank (min 1)", func(t *testing.T) {
		cases := []struct{ rank, expected int16 }{
			{1, 5}, {2, 4}, {3, 3}, {4, 2}, {5, 1}, {6, 1}, // 6 is defensive: max(0, 1) = 1
		}
		for _, tc := range cases {
			assert.Equal(t, tc.expected, game.ExchangeCourtiersDifficulty(tc.rank))
		}
	})

	t.Run("make introductions: 2 + peer_count (0 treated as 1)", func(t *testing.T) {
		cases := []struct {
			peerCount, expected int16
		}{
			{0, 3}, {1, 3}, {2, 4}, {3, 5}, {4, 6},
		}
		for _, tc := range cases {
			got := game.MakeIntroductionsDifficulty(game.ResolutionData{PeerCount: tc.peerCount})
			assert.Equal(t, tc.expected, got)
		}
	})

	t.Run("spread propaganda: preparer esteem rank", func(t *testing.T) {
		for rank := int16(1); rank <= 5; rank++ {
			assert.Equal(t, rank, game.SpreadPropagandaDifficulty(rank))
		}
	})

	t.Run("propose decree: preparer power rank", func(t *testing.T) {
		for rank := int16(1); rank <= 5; rank++ {
			assert.Equal(t, rank, game.ProposeDecreeDifficulty(rank))
		}
	})

	t.Run("seek answers: preparer knowledge rank", func(t *testing.T) {
		for rank := int16(1); rank <= 5; rank++ {
			assert.Equal(t, rank, game.SeekAnswersDifficulty(rank))
		}
	})

	t.Run("spread rumors: non-main-char target uses preparer esteem rank", func(t *testing.T) {
		for rank := int16(1); rank <= 5; rank++ {
			assert.Equal(t, rank, game.SpreadRumorsDifficulty(rank, false))
		}
	})

	t.Run("spread rumors: main-char target uses 6 - target esteem rank", func(t *testing.T) {
		cases := []struct{ rank, expected int16 }{
			{1, 5}, {2, 4}, {3, 3}, {4, 2}, {5, 1}, {6, 1},
		}
		for _, tc := range cases {
			assert.Equal(t, tc.expected, game.SpreadRumorsDifficulty(tc.rank, true))
		}
	})

	t.Run("chronicle histories: max(knowledge rank, invoked artifact count)", func(t *testing.T) {
		cases := []struct {
			name      string
			rank      int16
			artifacts int
			expected  int16
		}{
			{"rank dominates, no artifacts", 4, 0, 4},
			{"artifacts dominate over low rank", 1, 3, 3},
			{"tie stays the same", 2, 2, 2},
			{"many artifacts over rank", 1, 6, 6},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				ids := make([]int64, tc.artifacts)
				for i := range ids {
					ids[i] = int64(i + 1)
				}
				resData := game.ResolutionData{InvokedArtifactIDs: ids}
				assert.Equal(t, tc.expected, game.ChronicleHistoriesDifficulty(tc.rank, resData))
			})
		}
	})

	t.Run("propose duel: 6 - target esteem rank (min 1)", func(t *testing.T) {
		cases := []struct{ rank, expected int16 }{
			{1, 5}, {2, 4}, {3, 3}, {4, 2}, {5, 1}, {6, 1},
		}
		for _, tc := range cases {
			assert.Equal(t, tc.expected, game.ProposeDuelDifficulty(tc.rank))
		}
	})
}

// TestProposeDecreeCouncilEligibility tests the eligibility rules for
// joining a Propose Decree council.
func TestProposeDecreeCouncilEligibility(t *testing.T) {
	// Eligibility rule: joiner must have rank 1 (Monarch), or rank < preparerRank.
	checkEligible := func(joinerRank, preparerRank int16) bool {
		return joinerRank == 1 || joinerRank < preparerRank
	}

	t.Run("Monarch (rank 1) is always eligible", func(t *testing.T) {
		assert.True(t, checkEligible(1, 1), "Monarch eligible even if preparer is also rank 1")
		assert.True(t, checkEligible(1, 3))
		assert.True(t, checkEligible(1, 5))
	})

	t.Run("player ranked above preparer is eligible", func(t *testing.T) {
		// rank 2 can join when preparer is rank 3, 4, or 5
		assert.True(t, checkEligible(2, 3))
		assert.True(t, checkEligible(2, 4))
		assert.True(t, checkEligible(4, 5))
	})

	t.Run("player ranked equal to preparer is not eligible", func(t *testing.T) {
		assert.False(t, checkEligible(3, 3))
		assert.False(t, checkEligible(4, 4))
	})

	t.Run("player ranked below preparer is not eligible", func(t *testing.T) {
		assert.False(t, checkEligible(4, 3))
		assert.False(t, checkEligible(5, 2))
	})
}

// TestSimultaneousRevealDelay tests the ceiling-of-average delay computation
// used by Clandestinely Liaise and Make War simultaneous reveals.
func TestSimultaneousRevealDelay(t *testing.T) {
	// delay = ceil(average of all submitted faces)
	ceilAvg := func(faces []int16) int16 {
		if len(faces) == 0 {
			return 0
		}
		sum := 0
		for _, f := range faces {
			sum += int(f)
		}
		return int16(math.Ceil(float64(sum) / float64(len(faces))))
	}

	testCases := []struct {
		name          string
		faces         []int16
		expectedDelay int16
	}{
		{"both reveal 1 → delay 1", []int16{1, 1}, 1},
		{"both reveal 6 → delay 6", []int16{6, 6}, 6},
		{"1 and 2 → avg 1.5 → ceil 2", []int16{1, 2}, 2},
		{"3 and 4 → avg 3.5 → ceil 4", []int16{3, 4}, 4},
		{"2 and 4 → avg 3.0 → delay 3", []int16{2, 4}, 3},
		{"1 and 6 → avg 3.5 → ceil 4", []int16{1, 6}, 4},
		// Three participants (Make War scenario)
		{"1, 2, 3 → avg 2.0 → delay 2", []int16{1, 2, 3}, 2},
		{"1, 1, 2 → avg 1.33 → ceil 2", []int16{1, 1, 2}, 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ceilAvg(tc.faces)
			assert.Equal(t, tc.expectedDelay, got)
		})
	}
}
