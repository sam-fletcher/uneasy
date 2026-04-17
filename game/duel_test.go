package game

import "testing"

func TestResolveBout_HighDeclarerWinsWhenHigher(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclHigh, 5, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Match {
		t.Fatalf("expected non-match")
	}
	if out.WinnerSide != DuelSidePreparer {
		t.Errorf("winner = %v, want preparer", out.WinnerSide)
	}
	if out.NextDeclarer != DuelSideTarget {
		t.Errorf("next declarer = %v, want target (initiative swaps)", out.NextDeclarer)
	}
}

func TestResolveBout_HighDeclarerLosesWhenLower(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclHigh, 2, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.WinnerSide != DuelSideTarget {
		t.Errorf("winner = %v, want target", out.WinnerSide)
	}
}

func TestResolveBout_LowDeclarerWinsWhenLower(t *testing.T) {
	out, err := ResolveBout(DuelSideTarget, DeclLow, 1, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.WinnerSide != DuelSideTarget {
		t.Errorf("winner = %v, want target", out.WinnerSide)
	}
	if out.NextDeclarer != DuelSidePreparer {
		t.Errorf("initiative should swap")
	}
}

func TestResolveBout_LowDeclarerLosesWhenHigher(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclLow, 5, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.WinnerSide != DuelSideTarget {
		t.Errorf("winner = %v, want target", out.WinnerSide)
	}
}

func TestResolveBout_MatchSetsAside(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclHigh, 3, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Match {
		t.Fatalf("expected match")
	}
	if out.NextDeclarer != DuelSideTarget {
		t.Errorf("initiative should swap after match")
	}
}

func TestResolveBout_InvalidDice(t *testing.T) {
	if _, err := ResolveBout(DuelSidePreparer, DeclHigh, 0, 3); err == nil {
		t.Error("expected error for die=0")
	}
	if _, err := ResolveBout(DuelSidePreparer, DeclHigh, 3, 7); err == nil {
		t.Error("expected error for die=7")
	}
}

func TestResolveBout_InvalidDeclaration(t *testing.T) {
	if _, err := ResolveBout(DuelSidePreparer, "sideways", 3, 4); err == nil {
		t.Error("expected error for invalid declaration")
	}
}

func TestBoutsComplete(t *testing.T) {
	cases := []struct {
		name string
		t    DuelTallies
		want bool
	}{
		{"both have stakes", DuelTallies{PreparerRemaining: 2, TargetRemaining: 3}, false},
		{"preparer out", DuelTallies{PreparerRemaining: 0, TargetRemaining: 3}, true},
		{"target out", DuelTallies{PreparerRemaining: 2, TargetRemaining: 0}, true},
		{"both out", DuelTallies{}, true},
	}
	for _, c := range cases {
		if got := c.t.BoutsComplete(); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestMaxStakes(t *testing.T) {
	cases := []struct {
		rank int16
		want int16
	}{
		{1, 6}, // status 5 → 1+5
		{2, 5},
		{3, 4},
		{4, 3},
		{5, 2},
		{6, 1}, // status 0 → 1+0
	}
	for _, c := range cases {
		if got := MaxStakes(c.rank); got != c.want {
			t.Errorf("MaxStakes(%d) = %d, want %d", c.rank, got, c.want)
		}
	}
}
