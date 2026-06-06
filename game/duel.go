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

// BoutWinnerID returns the player ID that won a bout, or nil on a tie (match).
// Pure restatement of the handler's winner pick: the declarer wins iff the
// outcome's winning side is the declarer's side.
func BoutWinnerID(outcome BoutOutcome, declarerSide DuelSide, declarerID, responderID int64) *int64 {
	if outcome.Match {
		return nil
	}
	wid := declarerID
	if outcome.WinnerSide != declarerSide {
		wid = responderID
	}
	return &wid
}

// DuelBoutView is the domain snapshot of one resolved bout for dice
// accumulation — only the fields the rule needs, decoupled from dbgen. The
// handler builds these from the bout rows in chronological order.
type DuelBoutView struct {
	DeclarerDie  *int16
	ResponderDie *int16
	IsMatch      bool
	// WinnerIsPreparer is nil when no winner is recorded (an unfinished or
	// undecided bout — skipped). Consulted only when !IsMatch.
	WinnerIsPreparer *bool
}

// AccumulateDuelDice walks the bouts in order and returns the dice faces each
// side has won. Tied-bout dice carry over: they wait in a pending pool and are
// awarded, together with the current bout's two dice, to the winner of the next
// non-tie bout (per the rules — "the winner gets both dice from that round as
// well as any set aside from previous tied bouts"). Tied dice that never reach
// a later non-tie bout (e.g. a trailing tie) are dropped — they stay with their
// stakes. Bouts missing a die, or non-tie bouts with no recorded winner, are
// skipped.
func AccumulateDuelDice(bouts []DuelBoutView) (preparerDice, targetDice []int16) {
	var pending []int16
	for _, b := range bouts {
		if b.DeclarerDie == nil || b.ResponderDie == nil {
			continue
		}
		if b.IsMatch {
			pending = append(pending, *b.DeclarerDie, *b.ResponderDie)
			continue
		}
		if b.WinnerIsPreparer == nil {
			continue
		}
		gained := append([]int16{*b.DeclarerDie, *b.ResponderDie}, pending...)
		pending = nil
		if *b.WinnerIsPreparer {
			preparerDice = append(preparerDice, gained...)
		} else {
			targetDice = append(targetDice, gained...)
		}
	}
	return preparerDice, targetDice
}
