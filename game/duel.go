package game

// game/duel.go — Pure state machine for Propose Duel bouts.
//
// A duel consists of a series of "bouts" between two players. Each player
// has staked a set of assets, and each staked asset has a hidden d6 tucked
// under it (visible to the asset's owner; hidden from the opponent).
//
// Per bout:
//   1. The player with *initiative* is the declarer. They select one of their
//      unresolved stakes and declare "high" or "low".
//   2. The opponent responds by selecting one of their unresolved stakes.
//   3. Both hidden dice are revealed.
//       - If the dice match → neither wins; both stakes are set aside; swap
//         initiative; continue to a new bout.
//       - Otherwise → the high die wins if declarer said "high", the low die
//         wins if declarer said "low". The loser's die goes to the winner's
//         accumulated pool (both stakes are resolved); initiative swaps.
//
// Bouts continue until one player runs out of unresolved stakes. The
// accumulated winning dice feed into the plan's standard dice roll — the
// winner's dice become actor dice (for the preparer side) or interference
// dice (for the opponent side) depending on which player accumulated them.
//
// This file contains ONLY pure logic: no DB access, no HTTP. The handler
// loads state, calls these functions, and persists the result.

import "errors"

// DuelSide identifies a participant in a duel.
type DuelSide int

const (
	DuelSidePreparer DuelSide = 1
	DuelSideTarget   DuelSide = 2
)

// Declaration is what the declarer calls before dice are revealed.
type Declaration string

const (
	DeclHigh Declaration = "high"
	DeclLow  Declaration = "low"
)

// BoutOutcome is the result of comparing two revealed dice.
type BoutOutcome struct {
	// Match is true if both dice were equal (tie). Both stakes are set aside
	// and no one accumulates a die.
	Match bool
	// WinnerSide is the side that won (if !Match). Both dice are accumulated
	// by the winner.
	WinnerSide DuelSide
	// NextDeclarer is the side that gets initiative for the next bout. Always
	// swaps from the current declarer regardless of outcome.
	NextDeclarer DuelSide
}

// ResolveBout computes the outcome of a bout given the declarer's call and
// both revealed dice. Dice are d6 (1–6).
//
// Returns an error if either die is out of range.
func ResolveBout(declarerSide DuelSide, decl Declaration, declarerDie, responderDie int16) (BoutOutcome, error) {
	if declarerDie < 1 || declarerDie > 6 || responderDie < 1 || responderDie > 6 {
		return BoutOutcome{}, errors.New("dice must be 1–6")
	}
	if decl != DeclHigh && decl != DeclLow {
		return BoutOutcome{}, errors.New("declaration must be 'high' or 'low'")
	}

	opponent := opposingSide(declarerSide)
	next := opponent // initiative always swaps

	if declarerDie == responderDie {
		return BoutOutcome{Match: true, NextDeclarer: next}, nil
	}

	declarerHigher := declarerDie > responderDie
	declarerWins := (decl == DeclHigh && declarerHigher) || (decl == DeclLow && !declarerHigher)

	winner := opponent
	if declarerWins {
		winner = declarerSide
	}
	return BoutOutcome{WinnerSide: winner, NextDeclarer: next}, nil
}

// opposingSide returns the other side.
func opposingSide(s DuelSide) DuelSide {
	if s == DuelSidePreparer {
		return DuelSideTarget
	}
	return DuelSidePreparer
}

// DuelTallies holds the number of unresolved stakes on each side and the
// accumulated winning dice on each side. Used to decide when bouts end.
type DuelTallies struct {
	PreparerRemaining int
	TargetRemaining   int
	PreparerAccumDice []int16 // faces the preparer has won
	TargetAccumDice   []int16 // faces the target has won
}

// BoutsComplete reports whether the bout sequence should end — i.e. one or
// both sides have no unresolved stakes left.
func (t DuelTallies) BoutsComplete() bool {
	return t.PreparerRemaining == 0 || t.TargetRemaining == 0
}

// MaxStakes returns the maximum number of assets a player with the given
// esteem *status* may stake. Per the spec: min 1, max 1+status.
// Status is 6 - rank (so rank 1 → status 5).
func MaxStakes(esteemRank int16) int16 {
	status := max(6-esteemRank, 0)
	return 1 + status
}
