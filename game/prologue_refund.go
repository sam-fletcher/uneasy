package game

// prologue_refund.go — "Max commitment" model for prologue ranking.
//
// Players commit specific heart cards to specific tracks. The system
// continuously computes which hearts are "bright" (doing work — needed
// to maintain the player's current rank) versus "grey" (wasted — would
// be refunded if the track resolved now). At resolution, only bright
// hearts lock in as spent; grey hearts return to the player's hand for
// the next track.
//
// Bright/grey is computed per player by greedy descent from highest-
// value heart: if removing the heart leaves the player's rank
// unchanged, mark grey. This matches the "highest-value-wasted-first"
// refund priority while running in O(P × H) ranking calls per track —
// trivial at the player and heart counts the game uses.

import (
	"errors"
	"sort"
)

// CommittedHeart represents one specific heart card committed by one
// player to one track. CardID identifies the underlying player_cards
// row; Value is the card's face value (e.g. "K", "10", "3").
type CommittedHeart struct {
	PlayerID int64
	Track    string
	CardID   int64
	Value    string
}

// ValidateCommittedHearts checks that no card is committed to more than
// one track.
func ValidateCommittedHearts(committed []CommittedHeart) error {
	seen := map[int64]bool{}
	for _, h := range committed {
		if seen[h.CardID] {
			return errors.New("heart committed to multiple tracks")
		}
		seen[h.CardID] = true
	}
	return nil
}

// ComputeTrackRankingFromCommitments ranks players for a single track
// using each player's specific committed hearts as the heart
// contribution to that track. The values of the specific hearts
// committed participate in tiebreaks; this differs from
// ComputeTrackRanking, which always promotes a player's highest hearts
// up to the declared count.
func ComputeTrackRankingFromCommitments(
	track string,
	allPlayerIDs []int64,
	cards []PlayerCard,
	committed []CommittedHeart,
) (ranked []int64, setAside []int64, err error) {
	if track != PrologueTrackPower && track != PrologueTrackKnowledge && track != PrologueTrackEsteem {
		return nil, nil, errors.New("invalid track")
	}
	if err := ValidateCommittedHearts(committed); err != nil {
		return nil, nil, err
	}
	suit := SuitForTrack(track)

	natural := map[int64][]string{}
	for _, c := range cards {
		if c.Suit == suit {
			natural[c.PlayerID] = append(natural[c.PlayerID], c.Value)
		}
	}
	heartsForTrack := map[int64][]string{}
	for _, h := range committed {
		if h.Track == track {
			heartsForTrack[h.PlayerID] = append(heartsForTrack[h.PlayerID], h.Value)
		}
	}
	return rankFromContributions(allPlayerIDs, natural, heartsForTrack)
}

// rankFromContributions implements the per-track sort given each
// player's natural-suit cards and the specific hearts they're
// contributing. The algorithm matches ComputeTrackRanking:
//
//	count desc → high-card desc → next-card desc → heart-loses-final-tie.
func rankFromContributions(
	allPlayerIDs []int64,
	natural map[int64][]string,
	hearts map[int64][]string,
) (ranked []int64, setAside []int64, err error) {
	type item struct {
		playerID  int64
		count     int
		valSorted []string
		isHeart   []bool
	}
	items := make([]item, 0, len(allPlayerIDs))
	for _, pid := range allPlayerIDs {
		nat := natural[pid]
		hs := hearts[pid]
		if len(nat)+len(hs) == 0 {
			setAside = append(setAside, pid)
			continue
		}
		combined := make([]string, 0, len(nat)+len(hs))
		flags := make([]bool, 0, cap(combined))
		for _, v := range nat {
			combined = append(combined, v)
			flags = append(flags, false)
		}
		for _, v := range hs {
			combined = append(combined, v)
			flags = append(flags, true)
		}
		idx := make([]int, len(combined))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool {
			ra, rb := cardRank(combined[idx[a]]), cardRank(combined[idx[b]])
			if ra != rb {
				return ra > rb
			}
			return !flags[idx[a]] && flags[idx[b]]
		})
		sortedVals := make([]string, len(combined))
		sortedHeart := make([]bool, len(combined))
		for i, j := range idx {
			sortedVals[i] = combined[j]
			sortedHeart[i] = flags[j]
		}
		items = append(items, item{
			playerID: pid, count: len(combined),
			valSorted: sortedVals, isHeart: sortedHeart,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		n := min(len(items[i].valSorted), len(items[j].valSorted))
		for k := range n {
			ri := cardRank(items[i].valSorted[k])
			rj := cardRank(items[j].valSorted[k])
			if ri != rj {
				return ri > rj
			}
			if items[i].isHeart[k] != items[j].isHeart[k] {
				return !items[i].isHeart[k]
			}
		}
		return len(items[i].valSorted) > len(items[j].valSorted)
	})
	for _, it := range items {
		ranked = append(ranked, it.playerID)
	}
	return ranked, setAside, nil
}

// ComputeBrightHearts determines which committed hearts on a track are
// bright (needed to maintain the player's final slot) vs grey (would
// be refunded if the track resolved now). The greedy walks each
// player's hearts from highest to lowest value; a heart is greyed if
// removing it (along with already-greyed hearts) leaves the player at
// the same final slot.
//
// "Final slot" treats set-aside players as just having zero cards:
// they're appended at the end of the ranked sequence in a
// deterministic default order (player_id ascending) and slotted into
// the remaining open ranks. This means a heart that promotes a
// zero-card player from set-aside to ranked but doesn't change which
// open slot they end up occupying is correctly identified as grey.
//
// Returns brightSet[playerID][cardID] = true. Hearts committed to this
// track but absent from brightSet are grey.
func ComputeBrightHearts(
	track string,
	allPlayerIDs []int64,
	cards []PlayerCard,
	committed []CommittedHeart,
) (map[int64]map[int64]bool, error) {
	if track != PrologueTrackPower && track != PrologueTrackKnowledge && track != PrologueTrackEsteem {
		return nil, errors.New("invalid track")
	}

	baseline, err := computeFinalSlots(track, allPlayerIDs, cards, committed)
	if err != nil {
		return nil, err
	}

	perPlayer := map[int64][]CommittedHeart{}
	for _, h := range committed {
		if h.Track == track {
			perPlayer[h.PlayerID] = append(perPlayer[h.PlayerID], h)
		}
	}
	for pid := range perPlayer {
		hs := perPlayer[pid]
		sort.SliceStable(hs, func(i, j int) bool {
			return cardRank(hs[i].Value) > cardRank(hs[j].Value)
		})
		perPlayer[pid] = hs
	}

	bright := map[int64]map[int64]bool{}
	for pid, hs := range perPlayer {
		bright[pid] = map[int64]bool{}
		greyed := map[int64]bool{}
		for _, h := range hs {
			trial := make([]CommittedHeart, 0, len(committed))
			for _, c := range committed {
				if c.CardID == h.CardID || greyed[c.CardID] {
					continue
				}
				trial = append(trial, c)
			}
			trialSlots, err := computeFinalSlots(track, allPlayerIDs, cards, trial)
			if err != nil {
				return nil, err
			}
			if trialSlots[pid] == baseline[pid] {
				greyed[h.CardID] = true
			}
		}
		for _, h := range hs {
			if !greyed[h.CardID] {
				bright[pid][h.CardID] = true
			}
		}
	}
	return bright, nil
}

// computeFinalSlots returns each player's final rank slot (1..5) for
// the given commitment state. Set-aside players (zero cards on this
// track) are appended in player_id order — only the deterministic
// default; the rank-1 player can later override the order when
// multiple set-asides exist, but bright/grey decisions are made
// against this default since no other ordering is yet committed.
func computeFinalSlots(
	track string,
	allPlayerIDs []int64,
	cards []PlayerCard,
	committed []CommittedHeart,
) (map[int64]int, error) {
	ranked, setAside, err := ComputeTrackRankingFromCommitments(track, allPlayerIDs, cards, committed)
	if err != nil {
		return nil, err
	}
	sortedSetAside := append([]int64(nil), setAside...)
	sort.Slice(sortedSetAside, func(i, j int) bool {
		return sortedSetAside[i] < sortedSetAside[j]
	})
	seq := append([]int64(nil), ranked...)
	seq = append(seq, sortedSetAside...)
	open := openRanksForCount(len(allPlayerIDs))
	out := make(map[int64]int, len(seq))
	for i, pid := range seq {
		if i >= len(open) {
			break
		}
		out[pid] = open[i]
	}
	return out, nil
}

// openRanksForCount returns the rank slots (1..5) that aren't blocked
// by dummy tokens given the player count, in ascending order.
// Mirrors the openRanks helper in handler/prologue_ranking.go.
func openRanksForCount(n int) []int {
	var dummies []int
	switch n {
	case 4:
		dummies = []int{3}
	case 3:
		dummies = []int{1, 5}
	case 2:
		dummies = []int{1, 3, 5}
	}
	out := make([]int, 0, 5)
	for r := 1; r <= 5; r++ {
		skip := false
		for _, d := range dummies {
			if r == d {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, r)
		}
	}
	return out
}
