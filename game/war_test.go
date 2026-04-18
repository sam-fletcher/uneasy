package game

import (
	"reflect"
	"testing"
)

func TestMakeWarDelay(t *testing.T) {
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
			if got := MakeWarDelay(tc.faces); got != tc.want {
				t.Errorf("MakeWarDelay(%v) = %d, want %d", tc.faces, got, tc.want)
			}
		})
	}
}

func TestOpposingSide(t *testing.T) {
	if OpposingSide(WarSideDeclarer) != WarSideEnemy {
		t.Errorf("declarer's opposite should be enemy")
	}
	if OpposingSide(WarSideEnemy) != WarSideDeclarer {
		t.Errorf("enemy's opposite should be declarer")
	}
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
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveOpponents(1) = %v, want %v", got, want)
	}

	got = ActiveOpponents(3, sides, surrendered)
	want = []int64{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveOpponents(3) = %v, want %v", got, want)
	}

	if out := ActiveOpponents(99, sides, surrendered); out != nil {
		t.Errorf("unknown payer should return nil, got %v", out)
	}
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
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReversePowerOrder = %v, want %v", got, want)
	}

	// Unranked players go last; tie-break by player_id ascending.
	ranksPartial := map[int64]int16{10: 2, 20: 2}
	got = ReversePowerOrder([]int64{40, 10, 30, 20}, ranksPartial)
	want = []int64{10, 20, 30, 40} // ranked tied at 2 first by id, then unranked by id
	if !reflect.DeepEqual(got, want) {
		t.Errorf("partial ranks = %v, want %v", got, want)
	}
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
	got := MissingBattleCosts(active, sides, ranks, map[BattleCostKey]bool{})
	want := []BattleCostKey{
		{PayerID: 2, OpponentID: 1}, // player 2 owes opponent 1
		{PayerID: 3, OpponentID: 1}, // player 3 owes opponent 1
		{PayerID: 1, OpponentID: 2}, // player 1 owes opponents 2 and 3
		{PayerID: 1, OpponentID: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unpaid order = %v, want %v", got, want)
	}

	// After player 2 pays opponent 1: that entry is gone, rest preserved.
	paid := map[BattleCostKey]bool{{PayerID: 2, OpponentID: 1}: true}
	got = MissingBattleCosts(active, sides, ranks, paid)
	want = []BattleCostKey{
		{PayerID: 3, OpponentID: 1},
		{PayerID: 1, OpponentID: 2},
		{PayerID: 1, OpponentID: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after one payment = %v, want %v", got, want)
	}
}

func TestIsValidBattleCostChoice(t *testing.T) {
	if !IsValidBattleCostChoice(WarCostBreakAsset) {
		t.Error("break_asset should be valid")
	}
	if !IsValidBattleCostChoice(WarCostLeverageTwo) {
		t.Error("leverage_two should be valid")
	}
	if IsValidBattleCostChoice("") || IsValidBattleCostChoice("surrender") {
		t.Error("unknown choices should be invalid")
	}
}
