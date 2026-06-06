package game

// Exhaustive pure tests for the duel dice-accumulation and winner logic
// extracted from the handler (Option E pressure-test on the most complex plan).
// These hammer the tied-dice carry-over edge cases — trailing ties, runs of
// ties, interleavings — that the integration tests never exercise.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// win builds a resolved, non-tie bout with the two dice and the winning side.
func win(dDie, rDie int16, winnerIsPreparer bool) DuelBoutView {
	return DuelBoutView{DeclarerDie: new(dDie), ResponderDie: new(rDie), WinnerIsPreparer: new(winnerIsPreparer)}
}

// tie builds a tied bout (both dice equal, no winner).
func tie(die int16) DuelBoutView {
	return DuelBoutView{DeclarerDie: new(die), ResponderDie: new(die), IsMatch: true}
}

// unfinished builds a bout missing its responder die — should be skipped.
func unfinished(dDie int16, winnerIsPreparer bool) DuelBoutView {
	return DuelBoutView{DeclarerDie: new(dDie), WinnerIsPreparer: new(winnerIsPreparer)}
}

// nonTieNoWinner builds a resolved non-tie bout with no recorded winner —
// should be skipped, and its dice must NOT join the pending pool.
func nonTieNoWinner(dDie, rDie int16) DuelBoutView {
	return DuelBoutView{DeclarerDie: new(dDie), ResponderDie: new(rDie)}
}

func TestAccumulateDuelDice(t *testing.T) {
	cases := []struct {
		name     string
		bouts    []DuelBoutView
		wantPrep []int16
		wantTarg []int16
	}{
		{
			name: "empty",
		},
		{
			name:     "single preparer win takes both dice",
			bouts:    []DuelBoutView{win(5, 2, true)},
			wantPrep: []int16{5, 2},
		},
		{
			name:     "single target win takes both dice",
			bouts:    []DuelBoutView{win(3, 6, false)},
			wantTarg: []int16{3, 6},
		},
		{
			name:     "tie then preparer win: tied dice carry to the winner",
			bouts:    []DuelBoutView{tie(4), win(5, 1, true)},
			wantPrep: []int16{5, 1, 4, 4}, // current bout's dice, then the pending tie pair
		},
		{
			name:     "two ties then target win: both tie pairs carry over",
			bouts:    []DuelBoutView{tie(2), tie(6), win(3, 1, false)},
			wantTarg: []int16{3, 1, 2, 2, 6, 6},
		},
		{
			name:     "trailing tie is dropped (stays with its stakes)",
			bouts:    []DuelBoutView{win(6, 1, true), tie(3)},
			wantPrep: []int16{6, 1}, // the trailing tie's dice are NOT awarded
		},
		{
			name:     "win then tie then win: pending only spans the gap, not earlier wins",
			bouts:    []DuelBoutView{win(6, 2, true), tie(5), win(4, 1, false)},
			wantPrep: []int16{6, 2},
			wantTarg: []int16{4, 1, 5, 5},
		},
		{
			name:     "interleaved winners",
			bouts:    []DuelBoutView{win(6, 1, true), win(2, 5, false)},
			wantPrep: []int16{6, 1},
			wantTarg: []int16{2, 5},
		},
		{
			name:     "unfinished bout (missing die) is skipped",
			bouts:    []DuelBoutView{unfinished(4, true), win(5, 3, true)},
			wantPrep: []int16{5, 3},
		},
		{
			name:     "non-tie bout with no recorded winner is skipped, not pooled",
			bouts:    []DuelBoutView{nonTieNoWinner(4, 2), win(5, 3, true)},
			wantPrep: []int16{5, 3}, // the skipped bout's dice do NOT join pending
		},
		{
			name:  "all ties: nothing accumulates",
			bouts: []DuelBoutView{tie(2), tie(4)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prep, targ := AccumulateDuelDice(tc.bouts)
			assert.Equal(t, tc.wantPrep, prep, "preparer dice")
			assert.Equal(t, tc.wantTarg, targ, "target dice")
		})
	}
}

func TestBoutWinnerID(t *testing.T) {
	const declarerID, responderID int64 = 10, 20

	t.Run("tie returns nil", func(t *testing.T) {
		got := BoutWinnerID(BoutOutcome{Match: true}, DuelSidePreparer, declarerID, responderID)
		assert.Nil(t, got)
	})

	t.Run("declarer's side wins, declarer is the winner", func(t *testing.T) {
		got := BoutWinnerID(BoutOutcome{WinnerSide: DuelSidePreparer}, DuelSidePreparer, declarerID, responderID)
		if assert.NotNil(t, got) {
			assert.Equal(t, declarerID, *got)
		}
	})

	t.Run("opponent's side wins, responder is the winner", func(t *testing.T) {
		got := BoutWinnerID(BoutOutcome{WinnerSide: DuelSideTarget}, DuelSidePreparer, declarerID, responderID)
		if assert.NotNil(t, got) {
			assert.Equal(t, responderID, *got)
		}
	})

	t.Run("declarer is the target side and wins, declarer is the winner", func(t *testing.T) {
		got := BoutWinnerID(BoutOutcome{WinnerSide: DuelSideTarget}, DuelSideTarget, declarerID, responderID)
		if assert.NotNil(t, got) {
			assert.Equal(t, declarerID, *got)
		}
	})
}
