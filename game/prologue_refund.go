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
// bright (needed to maintain the player's current rank) vs grey
// (would be refunded if the track resolved now). The greedy walks each
// player's hearts from highest to lowest value; a heart is greyed if
// removing it (along with already-greyed hearts) leaves the player's
// rank unchanged.
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

	baselineRanked, baselineSetAside, err := ComputeTrackRankingFromCommitments(track, allPlayerIDs, cards, committed)
	if err != nil {
		return nil, err
	}
	baseline := rankIndex(baselineRanked, baselineSetAside)

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
			tRanked, tSetAside, err := ComputeTrackRankingFromCommitments(track, allPlayerIDs, cards, trial)
			if err != nil {
				return nil, err
			}
			if rankIndex(tRanked, tSetAside)[pid] == baseline[pid] {
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

// rankIndex maps playerID → rank slot. Ranked players get 0-based
// position; set-aside players share the sentinel -1 (treated as a
// single equivalence class for refund comparison).
func rankIndex(ranked, setAside []int64) map[int64]int {
	m := make(map[int64]int, len(ranked)+len(setAside))
	for i, pid := range ranked {
		m[pid] = i
	}
	for _, pid := range setAside {
		m[pid] = -1
	}
	return m
}
