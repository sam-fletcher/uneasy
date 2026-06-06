package game

// prologue_rankings.go — Pure rules for the prologue ranking algorithm
// (Phase 4b). Implements the per-track sort prescribed by PROLOGUE_RULES.md:
//
//   - Each player's "count" for a track is their natural-suit cards plus any
//     hearts they declared as that track's suit.
//   - Players with a count of zero are set aside (placed by the rank-1
//     player after the sort).
//   - Players with positive counts are sorted by count desc, with ties
//     broken by highest card → next-highest card → heart-as-final-loser.
//
// The "heart-as-final-loser" rule applies when two players have identical
// card lists by value (only possible if a heart was declared into the mix):
// the one whose tied high card was a heart loses the tie.

import (
	"errors"
	"sort"
	"strings"
)

// PlayerCard is a simplified view of a row in the player_cards table.
type PlayerCard struct {
	PlayerID int64
	Suit     rune
	Value    string
}

// HeartDeclaration captures one player's heart→track allocation.
type HeartDeclaration struct {
	PlayerID int64
	Track    string
	Count    int
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

// ComputeTrackRanking ranks players for a single track. cards is the full
// player_cards table for the game. declarations carries every heart
// declaration submitted so far across all tracks; only those matching the
// requested track contribute to the count, but all of them are used to
// "spend" hearts so the same heart can't be counted twice.
//
// Returns rank-ordered IDs (rank-1 first) and the set-aside IDs (any
// surviving order, for the rank-1 player to permute).
//
//nolint:gocognit,funlen // ranking algorithm with multi-key tiebreaking; refactoring would obscure the rules
func ComputeTrackRanking(
	track string,
	allPlayerIDs []int64,
	cards []PlayerCard,
	declarations []HeartDeclaration,
) (ranked []int64, setAside []int64, err error) {
	if track != PrologueTrackPower && track != PrologueTrackKnowledge && track != PrologueTrackEsteem {
		return nil, nil, errors.New("invalid track")
	}

	suit := SuitForTrack(track)

	// Hearts declared as this track's suit, per player.
	heartsForTrack := map[int64]int{}
	heartsAllocated := map[int64]int{} // total hearts spent across all tracks
	for _, d := range declarations {
		heartsAllocated[d.PlayerID] += d.Count
		if d.Track == track {
			heartsForTrack[d.PlayerID] = d.Count
		}
	}

	// Validate: hearts allocated <= hearts held.
	heartsHeld := map[int64]int{}
	for _, c := range cards {
		if c.Suit == SuitHearts {
			heartsHeld[c.PlayerID]++
		}
	}
	for pid, used := range heartsAllocated {
		if used > heartsHeld[pid] {
			return nil, nil, errors.New("player declared more hearts than they hold")
		}
	}

	// Build per-player card lists for this track: natural-suit cards +
	// the player's N highest hearts allocated to this track. Hearts
	// allocated to *other* tracks are not available here.
	natural := map[int64][]string{}
	hearts := map[int64][]string{}
	for _, c := range cards {
		switch c.Suit {
		case suit:
			natural[c.PlayerID] = append(natural[c.PlayerID], c.Value)
		case SuitHearts:
			hearts[c.PlayerID] = append(hearts[c.PlayerID], c.Value)
		}
	}

	// Sort each player's hearts desc so we pick the highest first.
	for pid := range hearts {
		sort.Slice(hearts[pid], func(i, j int) bool {
			return cardRank(hearts[pid][i]) > cardRank(hearts[pid][j])
		})
	}

	type entry struct {
		playerID    int64
		values      []string // natural-suit card values
		heartValues []string // hearts contributed to this track (subset of player's hearts)
	}

	entries := make([]entry, 0, len(allPlayerIDs))
	for _, pid := range allPlayerIDs {
		nat := natural[pid]
		want := min(heartsForTrack[pid], len(hearts[pid]))
		hs := append([]string(nil), hearts[pid][:want]...)
		entries = append(entries, entry{playerID: pid, values: nat, heartValues: hs})
	}

	// Set-aside: count of natural + declared hearts == 0.
	rankedEntries := entries[:0]
	for _, e := range entries {
		if len(e.values)+len(e.heartValues) == 0 {
			setAside = append(setAside, e.playerID)
			continue
		}
		rankedEntries = append(rankedEntries, e)
	}

	// Sort: by count desc, then by sorted-desc value list lex desc, with
	// hearts losing the final tie. We build a "sortKey" per entry:
	// combined value list sorted desc; tag whether each value came from a
	// heart; on tie at every position, the entry whose tied position is a
	// heart loses.
	type rankedItem struct {
		playerID int64
		count    int
		// values sorted desc paired with isHeart flag
		valSorted []string
		isHeart   []bool
	}
	items := make([]rankedItem, 0, len(rankedEntries))
	for _, e := range rankedEntries {
		combined := make([]string, 0, len(e.values)+len(e.heartValues))
		isHeartFlags := make([]bool, 0, cap(combined))
		for _, v := range e.values {
			combined = append(combined, v)
			isHeartFlags = append(isHeartFlags, false)
		}
		for _, v := range e.heartValues {
			combined = append(combined, v)
			isHeartFlags = append(isHeartFlags, true)
		}
		// Sort descending by card rank, with hearts breaking ties by
		// going *after* non-hearts (so a tied position favors non-heart).
		idx := make([]int, len(combined))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool {
			ra, rb := cardRank(combined[idx[a]]), cardRank(combined[idx[b]])
			if ra != rb {
				return ra > rb
			}
			// Same rank — non-heart sorts before heart.
			return !isHeartFlags[idx[a]] && isHeartFlags[idx[b]]
		})
		sortedVals := make([]string, len(combined))
		sortedHeart := make([]bool, len(combined))
		for i, j := range idx {
			sortedVals[i] = combined[j]
			sortedHeart[i] = isHeartFlags[j]
		}
		items = append(items, rankedItem{
			playerID:  e.playerID,
			count:     len(combined),
			valSorted: sortedVals,
			isHeart:   sortedHeart,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		// Compare by card value only, highest to lowest. Per
		// PROLOGUE_RULES.md, when the high cards tie we move on to the next
		// card — heart-ness does NOT break the tie at this stage.
		n := min(len(items[j].valSorted), len(items[i].valSorted))
		for k := range n {
			ri := cardRank(items[i].valSorted[k])
			rj := cardRank(items[j].valSorted[k])
			if ri != rj {
				return ri > rj
			}
		}
		if len(items[i].valSorted) != len(items[j].valSorted) {
			// Identical so far; longer wins (more cards is better).
			return len(items[i].valSorted) > len(items[j].valSorted)
		}
		// All card values tie. Final fallback: "the player whose high card
		// was a heart loses." Only the high card (position 0) decides.
		if items[i].isHeart[0] != items[j].isHeart[0] {
			return !items[i].isHeart[0]
		}
		return false
	})

	for _, it := range items {
		ranked = append(ranked, it.playerID)
	}
	return ranked, setAside, nil
}

// ValidateHeartDeclarations returns nil if every player's total declared
// hearts (across all tracks) is ≤ their hearts held.
func ValidateHeartDeclarations(cards []PlayerCard, declarations []HeartDeclaration) error {
	heartsHeld := map[int64]int{}
	for _, c := range cards {
		if c.Suit == SuitHearts {
			heartsHeld[c.PlayerID]++
		}
	}
	used := map[int64]int{}
	for _, d := range declarations {
		used[d.PlayerID] += d.Count
	}
	for pid, u := range used {
		if u > heartsHeld[pid] {
			return errors.New("player " + itoa64(pid) + " declared more hearts than held")
		}
	}
	return nil
}

func itoa64(n int64) string {
	var b strings.Builder
	if n < 0 {
		b.WriteRune('-')
		n = -n
	}
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	b.Write(digits)
	return b.String()
}
