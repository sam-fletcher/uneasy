package game

import (
	"reflect"
	"testing"
)

func TestComputeTrackRanking_basicSort(t *testing.T) {
	// Power track = clubs.
	// p1: 3 clubs; p2: 2 clubs; p3: 1 club; p4: 0 clubs (set aside).
	cards := []PlayerCard{
		{1, SuitClubs, "K"}, {1, SuitClubs, "5"}, {1, SuitClubs, "2"},
		{2, SuitClubs, "Q"}, {2, SuitClubs, "9"},
		{3, SuitClubs, "A"},
		{4, SuitDiamonds, "K"},
	}
	ranked, setAside, err := ComputeTrackRanking(PrologueTrackPower, []int64{1, 2, 3, 4}, cards, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ranked, []int64{1, 2, 3}) {
		t.Errorf("ranked = %v, want [1 2 3]", ranked)
	}
	if !reflect.DeepEqual(setAside, []int64{4}) {
		t.Errorf("setAside = %v, want [4]", setAside)
	}
}

func TestComputeTrackRanking_tieBreakHighCard(t *testing.T) {
	// Same count → high card wins.
	cards := []PlayerCard{
		{1, SuitClubs, "K"}, {1, SuitClubs, "2"},
		{2, SuitClubs, "A"}, {2, SuitClubs, "3"},
	}
	ranked, _, _ := ComputeTrackRanking(PrologueTrackPower, []int64{1, 2}, cards, nil)
	if !reflect.DeepEqual(ranked, []int64{2, 1}) {
		t.Errorf("ranked = %v, want [2 1]", ranked)
	}
}

func TestComputeTrackRanking_heartDeclarationContributes(t *testing.T) {
	// p1: 1 club. p2: 0 clubs but declares 1 heart (king of hearts).
	cards := []PlayerCard{
		{1, SuitClubs, "5"},
		{2, SuitHearts, "K"},
	}
	decls := []HeartDeclaration{{PlayerID: 2, Track: PrologueTrackPower, Count: 1}}
	ranked, setAside, _ := ComputeTrackRanking(PrologueTrackPower, []int64{1, 2}, cards, decls)
	// p2 has K (rank 13) > p1's 5 → p2 ranks first.
	if !reflect.DeepEqual(ranked, []int64{2, 1}) {
		t.Errorf("ranked = %v, want [2 1]", ranked)
	}
	if len(setAside) != 0 {
		t.Errorf("setAside = %v, want []", setAside)
	}
}

func TestComputeTrackRanking_heartLosesFinalTie(t *testing.T) {
	// p1: club K. p2: heart K declared into power. Both have count=1, both
	// hold a K — tied at rank. Heart-as-final-loser → p1 wins.
	cards := []PlayerCard{
		{1, SuitClubs, "K"},
		{2, SuitHearts, "K"},
	}
	decls := []HeartDeclaration{{PlayerID: 2, Track: PrologueTrackPower, Count: 1}}
	ranked, _, _ := ComputeTrackRanking(PrologueTrackPower, []int64{1, 2}, cards, decls)
	if !reflect.DeepEqual(ranked, []int64{1, 2}) {
		t.Errorf("ranked = %v, want [1 2] (heart loses tie)", ranked)
	}
}

func TestComputeTrackRanking_overdeclaredHearts(t *testing.T) {
	cards := []PlayerCard{{1, SuitHearts, "5"}} // only 1 heart
	decls := []HeartDeclaration{{PlayerID: 1, Track: PrologueTrackPower, Count: 2}}
	if _, _, err := ComputeTrackRanking(PrologueTrackPower, []int64{1}, cards, decls); err == nil {
		t.Error("expected error for over-declared hearts")
	}
}

func TestComputeTrackRanking_invalidTrack(t *testing.T) {
	if _, _, err := ComputeTrackRanking("bogus", nil, nil, nil); err == nil {
		t.Error("expected error for invalid track")
	}
}

func TestComputeTrackRanking_setAsideCorrectness(t *testing.T) {
	// Three players, only p1 has any clubs.
	cards := []PlayerCard{{1, SuitClubs, "9"}}
	ranked, setAside, _ := ComputeTrackRanking(PrologueTrackPower, []int64{1, 2, 3}, cards, nil)
	if !reflect.DeepEqual(ranked, []int64{1}) {
		t.Errorf("ranked = %v, want [1]", ranked)
	}
	if len(setAside) != 2 {
		t.Errorf("setAside = %v, want 2 entries", setAside)
	}
}
