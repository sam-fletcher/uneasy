package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeTrackRankingFromCommitments_specificHeartsContribute(t *testing.T) {
	// p1: 1 club. p2: 0 clubs but commits a specific heart (K♥) to power.
	cards := []PlayerCard{
		{1, SuitClubs, "5"},
		{2, SuitHearts, "K"},
		{2, SuitHearts, "3"}, // held but not committed
	}
	committed := []CommittedHeart{
		{PlayerID: 2, Track: PrologueTrackPower, CardID: 100, Value: "K"},
	}
	ranked, setAside, err := ComputeTrackRankingFromCommitments(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 1}, ranked, "p2's committed K beats p1's 5")
	assert.Empty(t, setAside)
}

func TestComputeTrackRankingFromCommitments_uncommittedHeartsDoNotCount(t *testing.T) {
	// p1: 1 club. p2: 1 heart held but not committed.
	cards := []PlayerCard{
		{1, SuitClubs, "5"},
		{2, SuitHearts, "K"},
	}
	ranked, setAside, err := ComputeTrackRankingFromCommitments(PrologueTrackPower, []int64{1, 2}, cards, nil)
	require.NoError(t, err)
	assert.Equal(t, []int64{1}, ranked)
	assert.Equal(t, []int64{2}, setAside)
}

func TestComputeTrackRankingFromCommitments_rejectsCardCommittedTwice(t *testing.T) {
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 7, Value: "K"},
		{PlayerID: 1, Track: PrologueTrackKnowledge, CardID: 7, Value: "K"},
	}
	_, _, err := ComputeTrackRankingFromCommitments(PrologueTrackPower, []int64{1}, nil, committed)
	assert.Error(t, err)
}

func TestComputeBrightHearts_unnecessaryHeartIsGrey(t *testing.T) {
	// p1: 4 clubs (already wins). p1 commits 1 heart anyway. Wasted.
	cards := []PlayerCard{
		{1, SuitClubs, "K"}, {1, SuitClubs, "Q"}, {1, SuitClubs, "5"}, {1, SuitClubs, "2"},
		{1, SuitHearts, "9"},
		{2, SuitClubs, "3"},
	}
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 50, Value: "9"},
	}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	assert.False(t, bright[1][50], "uncontested heart should be grey")
}

func TestComputeBrightHearts_necessaryHeartIsBright(t *testing.T) {
	// p1: 2 clubs. p2: 3 clubs. p1 commits 2 hearts to overtake p2.
	cards := []PlayerCard{
		{1, SuitClubs, "K"}, {1, SuitClubs, "Q"},
		{1, SuitHearts, "9"}, {1, SuitHearts, "8"},
		{2, SuitClubs, "A"}, {2, SuitClubs, "5"}, {2, SuitClubs, "3"},
	}
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 70, Value: "9"},
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 71, Value: "8"},
	}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	// With both hearts: p1 count=4 vs p2 count=3 → p1 rank 1.
	// Drop one heart: p1 count=3 ties p2 count=3, tiebreak by high card.
	//   p1 highest: K (clubs). p2 highest: A (clubs). p2 wins → p1 drops to rank 2.
	// So both hearts are needed. Both bright.
	assert.True(t, bright[1][70], "high heart needed for the count → bright")
	assert.True(t, bright[1][71], "low heart needed for the count → bright")
}

func TestComputeBrightHearts_highValueGreyedFirst(t *testing.T) {
	// p1: 4 clubs (already wins by count alone). p1 commits 2 hearts of
	// different values; both wasted. Greedy greys highest first; both end
	// up grey, but the order matters for the algorithm's correctness.
	cards := []PlayerCard{
		{1, SuitClubs, "K"}, {1, SuitClubs, "Q"}, {1, SuitClubs, "J"}, {1, SuitClubs, "10"},
		{1, SuitHearts, "A"}, {1, SuitHearts, "2"},
		{2, SuitClubs, "3"},
	}
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 80, Value: "A"},
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 81, Value: "2"},
	}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	assert.False(t, bright[1][80], "high heart wasted → grey")
	assert.False(t, bright[1][81], "low heart wasted → grey")
}

func TestComputeBrightHearts_minimumSetKeepsLowestValue(t *testing.T) {
	// p1: 2 clubs. p2: 3 clubs. p1 commits A♥ and 2♥ — either alone
	// would suffice (count=3 ties; tiebreak depends on which heart).
	//
	// p1 clubs: K, Q. p2 clubs: A, 5, 3.
	// With A♥ alone: p1 count=3 (K,Q,A♥), p2 count=3 (A,5,3). Tied, walk
	//   sorted-desc card lists. p1: A,K,Q (A is heart). p2: A,5,3 (none
	//   hearts). Position 0 both A; one is heart → heart loses → p1 loses
	//   tiebreak → rank 2.
	// With 2♥ alone: p1 count=3 (K,Q,2♥), p2 count=3 (A,5,3). Position 0
	//   K vs A → A wins → p1 rank 2.
	// Both lose with one heart. With both hearts: p1 count=4 vs p2
	//   count=3 → p1 rank 1.
	// So both hearts are individually insufficient but together necessary.
	// Both should be bright.
	cards := []PlayerCard{
		{1, SuitClubs, "K"}, {1, SuitClubs, "Q"},
		{1, SuitHearts, "A"}, {1, SuitHearts, "2"},
		{2, SuitClubs, "A"}, {2, SuitClubs, "5"}, {2, SuitClubs, "3"},
	}
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 90, Value: "A"},
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 91, Value: "2"},
	}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	assert.True(t, bright[1][90])
	assert.True(t, bright[1][91])
}

func TestComputeBrightHearts_emptyThreatRefunded(t *testing.T) {
	// The "bluffer" scenario from the plan doc.
	// Alice (p1): 4 clubs. Bob (p2): 2 clubs.
	// Bob commits 3♥ to threaten. Alice commits 2♥ to defend.
	// Alice's effective count: 4+2=6. Bob's: 2+3=5. Alice rank 1.
	// Test removing Alice's 2♥: she'd have 4 vs Bob's 5 → drops to rank 2.
	//   So Alice's 2♥ are bright.
	// Test removing Bob's 3♥: he'd have 2 vs Alice's 6 → still rank 2.
	//   So Bob's 3♥ are all grey (refunded).
	cards := []PlayerCard{
		{1, SuitClubs, "K"}, {1, SuitClubs, "Q"}, {1, SuitClubs, "J"}, {1, SuitClubs, "10"},
		{1, SuitHearts, "9"}, {1, SuitHearts, "8"},
		{2, SuitClubs, "A"}, {2, SuitClubs, "5"},
		{2, SuitHearts, "K"}, {2, SuitHearts, "7"}, {2, SuitHearts, "3"},
	}
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 100, Value: "9"},
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 101, Value: "8"},
		{PlayerID: 2, Track: PrologueTrackPower, CardID: 200, Value: "K"},
		{PlayerID: 2, Track: PrologueTrackPower, CardID: 201, Value: "7"},
		{PlayerID: 2, Track: PrologueTrackPower, CardID: 202, Value: "3"},
	}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	assert.True(t, bright[1][100], "Alice's high heart bright (defending)")
	assert.True(t, bright[1][101], "Alice's low heart bright (defending)")
	assert.False(t, bright[2][200], "Bob's hearts all grey (empty threat)")
	assert.False(t, bright[2][201])
	assert.False(t, bright[2][202])
}

func TestComputeBrightHearts_setAsideToRankedTransition(t *testing.T) {
	// p1: 0 clubs, commits 1 heart → ranked. Removing it → set-aside.
	// Heart is bright (rank changes from 0 to set-aside sentinel).
	cards := []PlayerCard{
		{1, SuitHearts, "5"},
		{2, SuitClubs, "3"},
	}
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackPower, CardID: 300, Value: "5"},
	}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	assert.True(t, bright[1][300], "heart that promotes from set-aside to ranked is bright")
}

func TestComputeBrightHearts_noCommitmentsEmptyResult(t *testing.T) {
	cards := []PlayerCard{{1, SuitClubs, "5"}, {2, SuitClubs, "3"}}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, nil)
	require.NoError(t, err)
	assert.Empty(t, bright)
}

func TestComputeBrightHearts_invalidTrack(t *testing.T) {
	_, err := ComputeBrightHearts("bogus", nil, nil, nil)
	assert.Error(t, err)
}

func TestComputeBrightHearts_otherTrackCommitmentsIgnored(t *testing.T) {
	// p1's heart committed to Knowledge shouldn't appear in Power's
	// bright/grey computation.
	cards := []PlayerCard{
		{1, SuitClubs, "5"},
		{1, SuitHearts, "K"},
		{2, SuitClubs, "3"},
	}
	committed := []CommittedHeart{
		{PlayerID: 1, Track: PrologueTrackKnowledge, CardID: 400, Value: "K"},
	}
	bright, err := ComputeBrightHearts(PrologueTrackPower, []int64{1, 2}, cards, committed)
	require.NoError(t, err)
	assert.Empty(t, bright[1], "no power commitments → no bright entries for p1")
}
