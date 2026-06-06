package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShakeUpTurnOrder_reverseRankSkipsDummies(t *testing.T) {
	pid := func(v int64) *int64 { return &v }
	rankings := []RankingRow{
		{PlayerID: pid(10), Category: "esteem", Rank: 1}, // top status
		{PlayerID: nil, Category: "esteem", Rank: 2},     // dummy
		{PlayerID: pid(20), Category: "esteem", Rank: 3},
		{PlayerID: pid(30), Category: "esteem", Rank: 5}, // lowest status
		// a different category must not leak in
		{PlayerID: pid(99), Category: "power", Rank: 5},
	}
	// Reverse rank: lowest status (rank 5) first, dummies skipped.
	assert.Equal(t, []int64{30, 20, 10}, ShakeUpTurnOrder("esteem", rankings))
}

func TestNextShakeUpActor(t *testing.T) {
	order := []int64{30, 20, 10} // reverse-rank: 30 lowest status, acts first
	pid := func(v int64) *int64 { return &v }

	all := map[int64]bool{30: true, 20: true, 10: true}

	t.Run("no spends yet → first in order", func(t *testing.T) {
		assert.Equal(t, int64(30), NextShakeUpActor(order, all, nil))
	})

	t.Run("advances to the next holder after last actor", func(t *testing.T) {
		assert.Equal(t, int64(20), NextShakeUpActor(order, all, pid(30)))
		assert.Equal(t, int64(10), NextShakeUpActor(order, all, pid(20)))
	})

	t.Run("loops back to the front", func(t *testing.T) {
		assert.Equal(t, int64(30), NextShakeUpActor(order, all, pid(10)))
	})

	t.Run("skips players with no tokens", func(t *testing.T) {
		// 20 is out of tokens; after 30 the turn skips to 10.
		some := map[int64]bool{30: true, 20: false, 10: true}
		assert.Equal(t, int64(10), NextShakeUpActor(order, some, pid(30)))
	})

	t.Run("sole token-holder keeps the turn", func(t *testing.T) {
		only30 := map[int64]bool{30: true, 20: false, 10: false}
		// After 30 acts, wrapping past the empty players returns to 30.
		assert.Equal(t, int64(30), NextShakeUpActor(order, only30, pid(30)))
	})

	t.Run("nobody holds tokens → 0", func(t *testing.T) {
		none := map[int64]bool{30: false, 20: false, 10: false}
		assert.Equal(t, int64(0), NextShakeUpActor(order, none, pid(10)))
	})
}
