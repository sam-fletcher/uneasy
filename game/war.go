package game

// war.go — Pure rules & helpers for Make War (Phase 3d).
//
// Make War introduces a long-lived state (the war) that outlives the plan
// row on which it's declared. Until the war ends, every row advance the
// server forces each active participant to pay the cost of battle once per
// opposing-side opponent, in reverse power order.
//
// This file contains only pure helpers (delay computation, side/opponent
// arithmetic, reverse-power ordering). DB-touching logic lives in the
// handler package.

import (
	"maps"
	"math"
	"slices"
	"sort"
)

// War sides. A war has exactly two sides; each participant is on one or the
// other.
const (
	WarSideDeclarer = int16(1) // the Make War preparer and their allies
	WarSideEnemy    = int16(2) // declared enemies and their allies
)

// Cost-of-battle choice keys.
const (
	WarCostBreakAsset  = "break_asset"
	WarCostLeverageTwo = "leverage_two"
)

// IsValidBattleCostChoice returns true if key names a valid cost option.
func IsValidBattleCostChoice(key string) bool {
	return key == WarCostBreakAsset || key == WarCostLeverageTwo
}

// End reasons for wars.
const (
	WarEndPeace          = "peace"
	WarEndSurrender      = "surrender"       // one side fully surrendered
	WarEndAllSurrendered = "all_surrendered" // degenerate: both sides gone
)

// Peace proposal statuses.
const (
	PeaceOpen     = "open"
	PeaceAccepted = "accepted"
	PeaceRejected = "rejected"
)

// CeilAverage returns ceil(average) of the given faces. Faces are expected
// to be 1–6; callers must filter out un-submitted entries. Returns 0 for an
// empty input (caller should treat as an error). Used for Make War delay
// and Clandestinely Liaise re-delay computations.
func CeilAverage(faces []int16) int16 {
	if len(faces) == 0 {
		return 0
	}
	sum := 0
	for _, f := range faces {
		sum += int(f)
	}
	return int16(math.Ceil(float64(sum) / float64(len(faces))))
}

// OpposingSide returns the other side number.
func OpposingSide(side int16) int16 {
	if side == WarSideDeclarer {
		return WarSideEnemy
	}
	return WarSideDeclarer
}

// MergeSides returns a new side-map containing every (player, side) pair
// from `sides` overlaid with `extra`. Used by cost-of-battle computations
// when running ActiveOpponents from the perspective of a late joiner who
// isn't yet in the canonical side-map.
func MergeSides(sides, extra map[int64]int16) map[int64]int16 {
	out := make(map[int64]int16, len(sides)+len(extra))
	maps.Copy(out, sides)
	maps.Copy(out, extra)
	return out
}

// ActiveOpponents returns the player IDs of participants on the opposite
// side of payerID who have not surrendered. `sides` maps player_id → side,
// `surrendered` is the set of player_ids who have surrendered.
// The returned slice is sorted ascending for deterministic iteration.
func ActiveOpponents(payerID int64, sides map[int64]int16, surrendered map[int64]bool) []int64 {
	payerSide, ok := sides[payerID]
	if !ok {
		return nil
	}
	opp := OpposingSide(payerSide)
	out := make([]int64, 0, len(sides))
	for id, side := range sides {
		if side != opp {
			continue
		}
		if surrendered[id] {
			continue
		}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// ReversePowerOrder sorts the given player IDs by power rank in reverse
// (rank 5 first, then 4, 3, 2, 1). ranks maps player_id → power rank.
// Players missing from the map are placed after all ranked players.
// Stable sort; ties broken by player_id ascending.
func ReversePowerOrder(players []int64, ranks map[int64]int16) []int64 {
	out := append([]int64(nil), players...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, oki := ranks[out[i]]
		rj, okj := ranks[out[j]]
		switch {
		case oki && okj && ri != rj:
			return ri > rj // higher rank number = lower power = pays earlier
		case !oki && okj:
			return false // unranked last
		case oki && !okj:
			return true
		default:
			return out[i] < out[j]
		}
	})
	return out
}

// MissingBattleCosts returns a slice of (payer, opponent) pairs still owed for the
// given row. `activeParticipants` is the list of non-surrendered participants,
// `sides` is the side map (which may still contain surrendered players, since a
// war continues while either side has an active member), `surrendered` is the
// set of player_ids who have surrendered, and `paid` is the set of (payer,
// opponent) pairs already satisfied.
//
// Both payers and opponents are restricted to non-surrendered participants: a
// surrendered player neither owes the cost of battle nor is owed it. Returned
// slice is empty if every active participant has paid once per active opponent.
// Entries are ordered by reverse-power (payer) then opponent.
func MissingBattleCosts(
	activeParticipants []int64,
	sides map[int64]int16,
	ranks map[int64]int16,
	surrendered map[int64]bool,
	paid map[BattleCostKey]bool,
) []BattleCostKey {
	ordered := ReversePowerOrder(activeParticipants, ranks)

	var missing []BattleCostKey
	for _, payer := range ordered {
		for _, opp := range ActiveOpponents(payer, sides, surrendered) {
			key := BattleCostKey{PayerID: payer, OpponentID: opp}
			if !paid[key] {
				missing = append(missing, key)
			}
		}
	}
	return missing
}

// BattleCostKey identifies one required (payer, opponent) payment per row.
type BattleCostKey struct {
	PayerID    int64
	OpponentID int64
}

// SurrenderOutcome returns whether the war should end after payerID surrenders
// and, if so, the reason. `sides` maps player_id → side for all full
// participants (including the payer and anyone previously surrendered).
// `surrendered` is the set of player_ids who had already surrendered before
// this call — the payer is treated as surrendered regardless of the map.
//
// The war ends when at least one side has no remaining active participants;
// reason is WarEndAllSurrendered if both sides are empty, WarEndSurrender
// otherwise.
func SurrenderOutcome(
	sides map[int64]int16,
	surrendered map[int64]bool,
	payerID int64,
) (ended bool, reason string) {
	remaining := map[int16]int{}
	for id, side := range sides {
		if id == payerID || surrendered[id] {
			continue
		}
		remaining[side]++
	}
	side1, side2 := remaining[WarSideDeclarer], remaining[WarSideEnemy]
	if side1 > 0 && side2 > 0 {
		return false, ""
	}
	if side1 == 0 && side2 == 0 {
		return true, WarEndAllSurrendered
	}
	return true, WarEndSurrender
}

// PeaceTally reports whether every active participant has voted accept.
// If not, it names the first (by iteration order of `active`) awaited voter.
// `active` is the list of active (non-surrendered) participants; `votes`
// holds accept votes keyed by player_id (missing entries and explicit false
// both count as "not yet accepted").
func PeaceTally(active []int64, votes map[int64]bool) (unanimous bool, awaiting int64) {
	for _, p := range active {
		if !votes[p] {
			return false, p
		}
	}
	return true, 0
}
