package game

import "slices"

// prologue_rankings.go — Shared primitives for the prologue ranking
// algorithm. The per-track sort itself lives in prologue_refund.go
// (ComputeTrackRankingFromCommitments / rankFromContributions); this file
// holds the card view and value-comparison helper both paths use.

// DummyRanks returns the rank slots (1-indexed) occupied by dummy tokens for
// a game with n real players, per PROLOGUE_RULES.md:
//   - 5 players: none
//   - 4 players: rank 3
//   - 3 players: ranks 1 and 5
//   - 2 players: ranks 1, 3, and 5
//
// Dummies pad the fixed 1–5 track so smaller games still span the full status
// range. Note rank 1 is a dummy in 2–3 player games, so the top *real* player
// is not at rank 1.
func DummyRanks(n int) []int16 {
	switch n {
	case 4:
		return []int16{3}
	case 3:
		return []int16{1, 5}
	case 2:
		return []int16{1, 3, 5}
	}
	return nil
}

// OpenRanks returns ranks 1..5 with the dummy positions removed, in ascending
// order — i.e. the slots real players occupy, highest status (lowest rank
// number) first. For n real players it always has length n.
func OpenRanks(n int) []int16 {
	dummies := DummyRanks(n)
	out := make([]int16, 0, 5)
	for r := int16(1); r <= 5; r++ {
		if !slices.Contains(dummies, r) {
			out = append(out, r)
		}
	}
	return out
}

// TopOfTrackPlayer returns the highest-status *real* player on a ranking
// track — the non-dummy occupant with the lowest rank number. Callers can't
// assume rank 1: dummy tokens occupy real rank slots (rank 1 in 2–3 player
// games — see DummyRanks), so the top real player may sit at rank 2. Returns
// nil if the track has no ranked (non-dummy) players yet.
//
// Shared by PlaceSetAsides's auth check and the prologue wait-state's
// place-set-asides mode — both need the exact same "who's on top" answer,
// and this used to be duplicated per-caller.
func TopOfTrackPlayer(rankings []RankingRow, category string) *int64 {
	var top *int64
	var topRank int16
	for _, rk := range rankings {
		if rk.Category != category || rk.PlayerID == nil {
			continue
		}
		if top == nil || rk.Rank < topRank {
			top = rk.PlayerID
			topRank = rk.Rank
		}
	}
	return top
}

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
