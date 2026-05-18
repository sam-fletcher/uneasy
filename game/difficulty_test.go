package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExchangeCourtiersDifficulty tests the difficulty formula: 6 - rank (min 1).
func TestExchangeCourtiersDifficulty(t *testing.T) {
	tests := []struct {
		name         string
		targetRank   int16
		expectedDiff int16
	}{
		{"rank 1 (highest status)", 1, 5},
		{"rank 2", 2, 4},
		{"rank 3", 3, 3},
		{"rank 4", 4, 2},
		{"rank 5", 5, 1},
		{"rank 6 (lowest status)", 6, 1}, // clamped to 1
		{"rank 0 (edge)", 0, 6},
		{"rank 7 (beyond normal)", 7, 1}, // clamped to 1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExchangeCourtiersDifficulty(tt.targetRank)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}

// TestMakeIntroductionsDifficulty tests the formula: 2 + peer_count (min peer_count 1).
func TestMakeIntroductionsDifficulty(t *testing.T) {
	tests := []struct {
		name         string
		peerCount    int16
		expectedDiff int16
	}{
		{"0 peers (treated as 1)", 0, 3},
		{"1 peer", 1, 3},
		{"2 peers", 2, 4},
		{"3 peers", 3, 5},
		{"4 peers (max)", 4, 6},
		{"5 peers (exceeds)", 5, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resData := ResolutionData{
				MakeIntroductions: &MakeIntroductionsResolutionData{PeerCount: tt.peerCount},
			}
			got := MakeIntroductionsDifficulty(resData)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}

// TestSpreadPropagandaDifficulty tests the formula: difficulty = rank.
func TestSpreadPropagandaDifficulty(t *testing.T) {
	tests := []struct {
		name         string
		preparerRank int16
		expectedDiff int16
	}{
		{"rank 1", 1, 1},
		{"rank 2", 2, 2},
		{"rank 3", 3, 3},
		{"rank 4", 4, 4},
		{"rank 5 (max esteem)", 5, 5},
		{"rank 0 (edge)", 0, 0},
		{"rank 6", 6, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpreadPropagandaDifficulty(tt.preparerRank)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}

// TestSeekAnswersDifficulty tests the formula: difficulty = rank.
func TestSeekAnswersDifficulty(t *testing.T) {
	tests := []struct {
		name          string
		knowledgeRank int16
		expectedDiff  int16
	}{
		{"rank 1", 1, 1},
		{"rank 2", 2, 2},
		{"rank 3", 3, 3},
		{"rank 4", 4, 4},
		{"rank 5 (max knowledge)", 5, 5},
		{"rank 0 (edge)", 0, 0},
		{"rank 6", 6, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SeekAnswersDifficulty(tt.knowledgeRank)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}

// TestSpreadRumorsDifficulty tests the formula:
// - target is main char: 6 - rank (min 1)
// - target not main char: rank
func TestSpreadRumorsDifficulty(t *testing.T) {
	tests := []struct {
		name             string
		relevantRank     int16
		targetIsMainChar bool
		expectedDiff     int16
	}{
		// Main character (target): 6 - rank (min 1)
		{"MC target, rank 1 (highest status)", 1, true, 5},
		{"MC target, rank 2", 2, true, 4},
		{"MC target, rank 3", 3, true, 3},
		{"MC target, rank 4", 4, true, 2},
		{"MC target, rank 5", 5, true, 1},
		{"MC target, rank 6 (lowest status)", 6, true, 1}, // clamped
		{"MC target, rank 0 (edge)", 0, true, 6},

		// Non-main character (preparer): rank
		{"non-MC preparer, rank 1", 1, false, 1},
		{"non-MC preparer, rank 2", 2, false, 2},
		{"non-MC preparer, rank 3", 3, false, 3},
		{"non-MC preparer, rank 4", 4, false, 4},
		{"non-MC preparer, rank 5", 5, false, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpreadRumorsDifficulty(tt.relevantRank, tt.targetIsMainChar)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}

// TestChronicleHistoriesDifficulty tests the formula:
// max(preparerKnowledgeRank, len(InvokedArtifactIDs))
func TestChronicleHistoriesDifficulty(t *testing.T) {
	tests := []struct {
		name          string
		knowledgeRank int16
		artifactCount int
		expectedDiff  int16
	}{
		{"knowledge rank 1, 0 artifacts", 1, 0, 1},
		{"knowledge rank 1, 3 artifacts", 1, 3, 3},
		{"knowledge rank 5, 1 artifact", 5, 1, 5},
		{"knowledge rank 3, 3 artifacts (equal)", 3, 3, 3},
		{"knowledge rank 2, 5 artifacts", 2, 5, 5},
		{"knowledge rank 0, 0 artifacts", 0, 0, 0},
		{"knowledge rank 0, 4 artifacts", 0, 4, 4},
		{"knowledge rank 6, 2 artifacts", 6, 2, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resData := ResolutionData{
				ChronicleHistories: &ChronicleHistoriesResolutionData{
					InvokedArtifactIDs: make([]int64, tt.artifactCount),
				},
			}
			got := ChronicleHistoriesDifficulty(tt.knowledgeRank, resData)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}

// TestProposeDecreeDifficulty tests the formula: difficulty = rank.
func TestProposeDecreeDifficulty(t *testing.T) {
	tests := []struct {
		name         string
		preparerRank int16
		expectedDiff int16
	}{
		{"rank 1", 1, 1},
		{"rank 2", 2, 2},
		{"rank 3", 3, 3},
		{"rank 4", 4, 4},
		{"rank 5 (max power)", 5, 5},
		{"rank 0 (edge)", 0, 0},
		{"rank 6", 6, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProposeDecreeDifficulty(tt.preparerRank)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}

// TestProposeDuelDifficulty tests the formula: 6 - rank (min 1).
func TestProposeDuelDifficulty(t *testing.T) {
	tests := []struct {
		name         string
		targetRank   int16
		expectedDiff int16
	}{
		{"rank 1 (highest esteem)", 1, 5},
		{"rank 2", 2, 4},
		{"rank 3", 3, 3},
		{"rank 4", 4, 2},
		{"rank 5 (lowest esteem)", 5, 1},
		{"rank 6 (beyond normal)", 6, 1}, // clamped to 1
		{"rank 0 (edge)", 0, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProposeDuelDifficulty(tt.targetRank)
			assert.Equal(t, tt.expectedDiff, got)
		})
	}
}
