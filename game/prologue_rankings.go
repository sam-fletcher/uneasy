package game

// prologue_rankings.go — Shared primitives for the prologue ranking
// algorithm. The per-track sort itself lives in prologue_refund.go
// (ComputeTrackRankingFromCommitments / rankFromContributions); this file
// holds the card view and value-comparison helper both paths use.

// PlayerCard is a simplified view of a row in the player_cards table.
type PlayerCard struct {
	PlayerID int64
	Suit     rune
	Value    string
}

// cardRank maps a card value to a comparable integer; A is high. Higher is
// better.
func cardRank(value string) int {
	switch value {
	case "A":
		return 14
	case "K":
		return 13
	case "Q":
		return 12
	case "J":
		return 11
	}
	// Numeric "2"–"10". "10" is two characters; rest are one digit.
	if len(value) == 2 && value == "10" {
		return 10
	}
	if len(value) == 1 {
		return int(value[0] - '0')
	}
	return 0
}
