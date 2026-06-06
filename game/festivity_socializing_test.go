package game

// Exhaustive tests for NextSocializingTurn — the combinatorial turn-order rule
// for the Host Festivity socializing phase (esteem ordering, host-last,
// skip-resolved). Untested before; this is the Host Festivity pressure-test.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// rankFn builds an esteem-rank lookup from a map, returning `missing` for any
// player not present (mirrors how the handler supplies a sentinel).
func rankFn(m map[int64]int16, missing int16) func(int64) int16 {
	return func(id int64) int16 {
		if r, ok := m[id]; ok {
			return r
		}
		return missing
	}
}

// fest builds a minimal FestivityResolutionData with the given guests and
// recorded outcomes (any non-empty string per player counts as "acted").
func fest(guests []int64, acted ...int64) *FestivityResolutionData {
	s := &FestivityResolutionData{Guests: guests, Outcomes: map[string]string{}}
	for _, id := range acted {
		s.Outcomes[int64ToKey(id)] = FestivityOutcomeMake
	}
	return s
}

func TestNextSocializingTurn(t *testing.T) {
	const host int64 = 4
	// Esteem ranks: lower number = higher esteem. Guest 3 has the lowest esteem
	// (rank 5) so acts first; the host acts last regardless of its own rank.
	ranks := rankFn(map[int64]int16{1: 1, 2: 3, 3: 5, 4: 2}, 0)

	t.Run("lowest esteem first, host last", func(t *testing.T) {
		s := fest([]int64{1, 2, 3, host})
		assert.EqualValues(t, 3, s.NextSocializingTurn(host, ranks))
	})

	t.Run("skips resolved guests in order", func(t *testing.T) {
		assert.EqualValues(t, 2, fest([]int64{1, 2, 3, host}, 3).NextSocializingTurn(host, ranks))
		assert.EqualValues(t, 1, fest([]int64{1, 2, 3, host}, 3, 2).NextSocializingTurn(host, ranks))
		assert.Equal(t, host, fest([]int64{1, 2, 3, host}, 3, 2, 1).NextSocializingTurn(host, ranks))
	})

	t.Run("all resolved returns 0", func(t *testing.T) {
		s := fest([]int64{1, 2, 3, host}, 1, 2, 3, host)
		assert.EqualValues(t, 0, s.NextSocializingTurn(host, ranks))
	})

	t.Run("host not a guest is never returned", func(t *testing.T) {
		s := fest([]int64{1, 2, 3}) // host (4) did not join
		assert.EqualValues(t, 3, s.NextSocializingTurn(host, ranks))
		// Once all three guests act, no one remains — the host is not appended.
		assert.EqualValues(t, 0, fest([]int64{1, 2, 3}, 1, 2, 3).NextSocializingTurn(host, ranks))
	})

	t.Run("host resolved early still yields pending guests first", func(t *testing.T) {
		// Host acted, guest 2 hasn't → guest 2 (who sorts before the host) is next.
		s := fest([]int64{1, 2, 3, host}, 3, 1, host)
		assert.EqualValues(t, 2, s.NextSocializingTurn(host, ranks))
	})

	t.Run("empty guest list returns 0", func(t *testing.T) {
		s := &FestivityResolutionData{Outcomes: map[string]string{}}
		assert.EqualValues(t, 0, s.NextSocializingTurn(host, ranks))
	})
}

// TestNextSocializingTurn_MissingRankSortsLast pins the missing-esteem
// convention: a guest with no esteem rank must act LAST among the guests, per
// the function's documented intent. The caller therefore must map "missing" to a
// LOW sentinel (below any real rank), since the sort is descending by rank — a
// high sentinel (the previous bug) would sort the unranked guest FIRST.
func TestNextSocializingTurn_MissingRankSortsLast(t *testing.T) {
	const unranked int64 = 99
	ranks := rankFn(map[int64]int16{1: 1, 2: 3}, 0) // 99 missing → sentinel 0

	s := fest([]int64{1, 2, unranked})
	// Descending by rank: 2 (rank 3), 1 (rank 1), then the unranked guest last.
	assert.EqualValues(t, 2, s.NextSocializingTurn(0, ranks))
	assert.EqualValues(t, 1, fest([]int64{1, 2, unranked}, 2).NextSocializingTurn(0, ranks))
	assert.Equal(t, unranked, fest([]int64{1, 2, unranked}, 2, 1).NextSocializingTurn(0, ranks))
}
