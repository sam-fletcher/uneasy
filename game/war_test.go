package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCeilAverage(t *testing.T) {
	cases := []struct {
		name  string
		faces []int16
		want  int16
	}{
		{"single die 3", []int16{3}, 3},
		{"pair avg 4.5 → 5", []int16{4, 5}, 5},
		{"pair avg 4 → 4", []int16{3, 5}, 4},
		{"four dice avg 2.5 → 3", []int16{1, 2, 3, 4}, 3},
		{"five dice sum 14 → ceil 2.8 = 3", []int16{1, 2, 3, 4, 4}, 3},
		{"all sixes", []int16{6, 6, 6}, 6},
		{"empty returns 0", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CeilAverage(tc.faces)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOpposingSide(t *testing.T) {
	assert.Equal(t, WarSideEnemy, OpposingSide(WarSideDeclarer), "declarer's opposite should be enemy")
	assert.Equal(t, WarSideDeclarer, OpposingSide(WarSideEnemy), "enemy's opposite should be declarer")
}

func TestActiveOpponents(t *testing.T) {
	sides := map[int64]int16{
		1: WarSideDeclarer,
		2: WarSideDeclarer,
		3: WarSideEnemy,
		4: WarSideEnemy,
		5: WarSideEnemy,
	}
	surrendered := map[int64]bool{4: true}

	got := ActiveOpponents(1, sides, surrendered)
	want := []int64{3, 5}
	assert.Equal(t, want, got)

	got = ActiveOpponents(3, sides, surrendered)
	want = []int64{1, 2}
	assert.Equal(t, want, got)

	out := ActiveOpponents(99, sides, surrendered)
	assert.Nil(t, out, "unknown payer should return nil")
}

func TestReversePowerOrder(t *testing.T) {
	// Rank 5 = lowest power → pays first.
	ranks := map[int64]int16{
		10: 1,
		20: 3,
		30: 5,
		40: 2,
	}
	got := ReversePowerOrder([]int64{10, 20, 30, 40}, ranks)
	want := []int64{30, 20, 40, 10} // 5, 3, 2, 1
	assert.Equal(t, want, got)

	// Unranked players go last; tie-break by player_id ascending.
	ranksPartial := map[int64]int16{10: 2, 20: 2}
	got = ReversePowerOrder([]int64{40, 10, 30, 20}, ranksPartial)
	want = []int64{10, 20, 30, 40} // ranked tied at 2 first by id, then unranked by id
	assert.Equal(t, want, got)
}

func TestMissingBattleCosts(t *testing.T) {
	sides := map[int64]int16{
		1: WarSideDeclarer,
		2: WarSideEnemy,
		3: WarSideEnemy,
	}
	ranks := map[int64]int16{1: 1, 2: 5, 3: 3}
	active := []int64{1, 2, 3}

	// Nothing paid yet. Order: rank 5 (player 2) first, then rank 3 (player 3),
	// then rank 1 (player 1). Each pays per opposing opponent.
	got := MissingBattleCosts(active, sides, ranks, nil, map[BattleCostKey]bool{})
	want := []BattleCostKey{
		{PayerID: 2, OpponentID: 1}, // player 2 owes opponent 1
		{PayerID: 3, OpponentID: 1}, // player 3 owes opponent 1
		{PayerID: 1, OpponentID: 2}, // player 1 owes opponents 2 and 3
		{PayerID: 1, OpponentID: 3},
	}
	assert.Equal(t, want, got)

	// After player 2 pays opponent 1: that entry is gone, rest preserved.
	paid := map[BattleCostKey]bool{{PayerID: 2, OpponentID: 1}: true}
	got = MissingBattleCosts(active, sides, ranks, nil, paid)
	want = []BattleCostKey{
		{PayerID: 3, OpponentID: 1},
		{PayerID: 1, OpponentID: 2},
		{PayerID: 1, OpponentID: 3},
	}
	assert.Equal(t, want, got)
}

// TestMissingBattleCosts_ExcludesSurrenderedOpponent pins that no active player
// owes the cost of battle against an opponent who has already surrendered, in a
// war that continues after a partial surrender (2v2, one enemy surrenders).
func TestMissingBattleCosts_ExcludesSurrenderedOpponent(t *testing.T) {
	// Players 1,2 on the declarer side; 3,4 on the enemy side. Player 4 has
	// surrendered, but player 3 is still active so the war continues.
	sides := map[int64]int16{
		1: WarSideDeclarer, 2: WarSideDeclarer,
		3: WarSideEnemy, 4: WarSideEnemy,
	}
	ranks := map[int64]int16{1: 1, 2: 2, 3: 3, 4: 4}
	active := []int64{1, 2, 3} // 4 surrendered → not an active payer
	surrendered := map[int64]bool{4: true}

	got := MissingBattleCosts(active, sides, ranks, surrendered, map[BattleCostKey]bool{})
	for _, k := range got {
		assert.NotEqualf(t, int64(4), k.OpponentID,
			"no active player should owe a cost against surrendered player 4 (got %+v)", k)
	}
	// The live opponents should still be charged: 1→3, 2→3, 3→{1,2}.
	want := []BattleCostKey{
		{PayerID: 1, OpponentID: 3},
		{PayerID: 2, OpponentID: 3},
		{PayerID: 3, OpponentID: 1},
		{PayerID: 3, OpponentID: 2},
	}
	assert.ElementsMatch(t, want, got)
}

func TestIsValidBattleCostChoice(t *testing.T) {
	assert.True(t, IsValidBattleCostChoice(WarCostBreakAsset), "break_asset should be valid")
	assert.True(t, IsValidBattleCostChoice(WarCostLeverageTwo), "leverage_two should be valid")
	assert.False(t, IsValidBattleCostChoice(""), "empty choice should be invalid")
	assert.False(t, IsValidBattleCostChoice("surrender"), "surrender should be invalid")
}

func TestSurrenderOutcome(t *testing.T) {
	cases := []struct {
		name        string
		sides       map[int64]int16
		surrendered map[int64]bool
		payer       int64
		wantEnded   bool
		wantReason  string
	}{
		{
			name:        "opponent remains on each side — war continues",
			sides:       map[int64]int16{1: WarSideDeclarer, 2: WarSideDeclarer, 3: WarSideEnemy, 4: WarSideEnemy},
			surrendered: map[int64]bool{},
			payer:       1,
			wantEnded:   false,
		},
		{
			name:        "payer was last on their side — war ends, surrender",
			sides:       map[int64]int16{1: WarSideDeclarer, 2: WarSideEnemy, 3: WarSideEnemy},
			surrendered: map[int64]bool{},
			payer:       1,
			wantEnded:   true,
			wantReason:  WarEndSurrender,
		},
		{
			name:        "both sides empty after this surrender — all-surrendered",
			sides:       map[int64]int16{1: WarSideDeclarer, 2: WarSideEnemy},
			surrendered: map[int64]bool{2: true},
			payer:       1,
			wantEnded:   true,
			wantReason:  WarEndAllSurrendered,
		},
		{
			name:        "prior surrenders ignored on payer's own side",
			sides:       map[int64]int16{1: WarSideDeclarer, 2: WarSideDeclarer, 3: WarSideEnemy},
			surrendered: map[int64]bool{2: true},
			payer:       1,
			wantEnded:   true,
			wantReason:  WarEndSurrender,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ended, reason := SurrenderOutcome(tc.sides, tc.surrendered, tc.payer)
			assert.Equal(t, tc.wantEnded, ended)
			if ended {
				assert.Equal(t, tc.wantReason, reason)
			}
		})
	}
}

func TestPeaceTally(t *testing.T) {
	t.Run("unanimous accept", func(t *testing.T) {
		unanimous, awaiting := PeaceTally(
			[]int64{1, 2, 3},
			map[int64]bool{1: true, 2: true, 3: true},
		)
		assert.True(t, unanimous)
		assert.Equal(t, int64(0), awaiting)
	})
	t.Run("one missing vote is awaited", func(t *testing.T) {
		unanimous, awaiting := PeaceTally(
			[]int64{1, 2, 3},
			map[int64]bool{1: true, 3: true},
		)
		assert.False(t, unanimous)
		assert.Equal(t, int64(2), awaiting)
	})
	t.Run("explicit false counts as missing", func(t *testing.T) {
		unanimous, awaiting := PeaceTally(
			[]int64{1, 2},
			map[int64]bool{1: true, 2: false},
		)
		assert.False(t, unanimous)
		assert.Equal(t, int64(2), awaiting)
	})
	t.Run("first missing in active-order is returned", func(t *testing.T) {
		unanimous, awaiting := PeaceTally(
			[]int64{5, 3, 7},
			map[int64]bool{5: true},
		)
		assert.False(t, unanimous)
		assert.Equal(t, int64(3), awaiting)
	})
	t.Run("empty active list is vacuously unanimous", func(t *testing.T) {
		unanimous, awaiting := PeaceTally(nil, map[int64]bool{})
		assert.True(t, unanimous)
		assert.Equal(t, int64(0), awaiting)
	})
}
