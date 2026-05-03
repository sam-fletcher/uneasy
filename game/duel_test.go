package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBout_HighDeclarerWinsWhenHigher(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclHigh, 5, 3)
	require.NoError(t, err)
	assert.False(t, out.Match, "expected non-match")
	assert.Equal(t, DuelSidePreparer, out.WinnerSide)
	assert.Equal(t, DuelSideTarget, out.NextDeclarer, "initiative swaps")
}

func TestResolveBout_HighDeclarerLosesWhenLower(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclHigh, 2, 6)
	require.NoError(t, err)
	assert.Equal(t, DuelSideTarget, out.WinnerSide)
}

func TestResolveBout_LowDeclarerWinsWhenLower(t *testing.T) {
	out, err := ResolveBout(DuelSideTarget, DeclLow, 1, 4)
	require.NoError(t, err)
	assert.Equal(t, DuelSideTarget, out.WinnerSide)
	assert.Equal(t, DuelSidePreparer, out.NextDeclarer, "initiative should swap")
}

func TestResolveBout_LowDeclarerLosesWhenHigher(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclLow, 5, 2)
	require.NoError(t, err)
	assert.Equal(t, DuelSideTarget, out.WinnerSide)
}

func TestResolveBout_MatchSetsAside(t *testing.T) {
	out, err := ResolveBout(DuelSidePreparer, DeclHigh, 3, 3)
	require.NoError(t, err)
	assert.True(t, out.Match, "expected match")
	assert.Equal(t, DuelSideTarget, out.NextDeclarer, "initiative should swap after match")
}

func TestResolveBout_InvalidDice(t *testing.T) {
	_, err := ResolveBout(DuelSidePreparer, DeclHigh, 0, 3)
	require.Error(t, err, "expected error for die=0")
	_, err = ResolveBout(DuelSidePreparer, DeclHigh, 3, 7)
	require.Error(t, err, "expected error for die=7")
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
		got := c.t.BoutsComplete()
		assert.Equal(t, c.want, got, c.name)
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
		got := MaxStakes(c.rank)
		assert.Equal(t, c.want, got)
	}
}
