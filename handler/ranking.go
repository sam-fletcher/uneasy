package handler

// handler/ranking.go — Engrailed-line ranking update algorithm (Phase 2f).
//
// runRankingUpdate is called by advanceRowInner (turn.go) whenever current_row
// crosses an engrailed line (after rows 4, 8, 12).
//
// Algorithm per rulebook §"Updating Rankings":
//
//  1. For each category (power, knowledge, esteem), gather all player tokens.
//  2. Process tokens from worst rank to best rank (bottom of plan stack upward).
//     For each token: swap that player with whoever is one rank above them on the
//     ranking track. If already at rank 1, or the slot above is nil (dummy/static
//     mode), skip.
//  3. After processing all tokens in a category: if every plan type in that
//     category has at least one token, clear all tokens for that category
//     (returning them to their players).
//  4. Upsert all modified ranking slots back to the DB.
//
// Note on multi-token adjacency: the rules process worst-to-best (bottom of
// stack first) with live rank updates after each swap. Two token holders at
// adjacent ranks will effectively cancel each other — this is the intended
// board behaviour when multiple players stack tokens on the same plan.

import (
	"context"
	"sort"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// categorySlots represents a single ranking category's slot array (rank 1-5).
// Index i holds the player ID at rank i+1, or nil if the slot is unoccupied (dummy).
type categorySlots [5]*int64

// applyRankingSwaps applies the worst-to-best token-holder swap algorithm and
// determines whether tokens should be cleared for each category.
//
// Modifies slots and playerRank in place. Returns two maps:
//   - shouldClearTokens[cat]: whether every plan type in that category had at
//     least one token holder (and thus tokens should be cleared after this call).
//   - swapped[cat][playerID]: whether that holder performed an up-swap (true) or
//     was already at the top with no real rival above (false). The chat narration
//     reads this to show an up-arrow vs. a crown; a net-zero "cancel" between two
//     adjacent holders is simply two true entries that undo each other.
func applyRankingSwaps(
	slots map[model.RankingCategory]*categorySlots,
	playerRank map[int64]map[model.RankingCategory]int16,
	tokens []dbgen.PlanToken,
	categoryPlanTypes map[model.RankingCategory][]model.PlanType,
) (map[model.RankingCategory]bool, map[model.RankingCategory]map[int64]bool) {
	shouldClearTokens := make(map[model.RankingCategory]bool)
	swapped := make(map[model.RankingCategory]map[int64]bool)

	for cat, planTypes := range categoryPlanTypes {
		s := slots[cat]

		// Gather unique token players for this category (de-duplicate stacked tokens).
		seen := make(map[int64]struct{})
		var tokenPlayers []int64
		for _, pt := range planTypes {
			for _, tok := range tokens {
				if tok.PlanType == pt {
					if _, dup := seen[tok.PlayerID]; !dup {
						seen[tok.PlayerID] = struct{}{}
						tokenPlayers = append(tokenPlayers, tok.PlayerID)
					}
				}
			}
		}
		if len(tokenPlayers) == 0 {
			shouldClearTokens[cat] = false
			continue
		}

		// Sort worst → best (highest rank# first = bottom of stack first).
		sort.Slice(tokenPlayers, func(i, j int) bool {
			ri := playerRank[tokenPlayers[i]][cat]
			rj := playerRank[tokenPlayers[j]][cat]
			return ri > rj
		})

		catSwapped := make(map[int64]bool)
		for _, pid := range tokenPlayers {
			catSwapped[pid] = swapTokenPlayerWithAbove(pid, cat, s, playerRank)
		}
		swapped[cat] = catSwapped

		// After all tokens in this category are resolved: if every plan type
		// on this sheet has at least one token, mark for clearing.
		allHaveTokens := true
		for _, pt := range planTypes {
			found := false
			for _, tok := range tokens {
				if tok.PlanType == pt {
					found = true
					break
				}
			}
			if !found {
				allHaveTokens = false
				break
			}
		}
		shouldClearTokens[cat] = allHaveTokens
	}

	return shouldClearTokens, swapped
}

// swapTokenPlayerWithAbove advances a token holder upward by swapping them with
// the first real player in a higher rank, skipping past any dummy (nil) slots.
// Modifies slots and playerRank in place. Returns true if a swap actually
// happened, or false if there was no real player above to overtake (the holder
// is at the top of the track) — the chat narration uses this to decide between
// an up-arrow and a crown.
func swapTokenPlayerWithAbove(
	pid int64,
	cat model.RankingCategory,
	s *categorySlots,
	playerRank map[int64]map[model.RankingCategory]int16,
) bool {
	rankMap, ok := playerRank[pid]
	if !ok {
		return false
	}
	myRank := rankMap[cat] // 1-indexed, live value
	if myRank <= 1 {
		return false // already at top, do nothing
	}
	myIdx := myRank - 1 // 0-indexed current slot

	// Search upward from myIdx-1 to find the first non-nil player to swap with.
	var aboveIdx int16
	var above *int64
	for i := myIdx - 1; i >= 0; i-- {
		if s[i] != nil {
			aboveIdx = i
			above = s[i]
			break
		}
	}

	// No real player found above (all dummies up to rank 1) — cannot advance.
	if above == nil {
		return false
	}

	// Swap pid and above in both the slot array and the live rank map.
	// Use a local copy so the pointer outlives this iteration.
	pidCopy := pid
	s[aboveIdx] = &pidCopy
	s[myIdx] = above
	playerRank[pid][cat] = aboveIdx + 1
	if _, ok := playerRank[*above]; ok {
		playerRank[*above][cat] = myIdx + 1
	}
	return true
}

// rankingMove is one preparer's outcome on a plan line, used to narrate the
// update into chat. Glyph is a keyword — "up" or "top" — that the chat emitter
// (EmitRankingUpdated) maps to a symbol, keeping the symbol choice in the
// presentation layer. "up" means the holder performed an up-swap (shown as an
// arrow); "top" means there was no real player above to overtake (a crown).
// A net-zero "cancel" between adjacent holders is just two "up" arrows that
// undo each other — deducible from the ordered set, so it needs no marker.
type rankingMove struct {
	Name  string `json:"name"`
	Glyph string `json:"glyph"`
}

// rankingPlanLine is one prepared plan and the preparers it affected, ordered
// bottom-to-top (worst rank first) to match the rules' resolution order.
type rankingPlanLine struct {
	PlanType model.PlanType `json:"plan_type"`
	Movers   []rankingMove  `json:"movers"`
}

// rankingCategoryDiff is one category's narration: its prepared plans in
// resolution order, the final standing (rank 1→5, with "Dummy" for empty
// slots), and whether the category's preparations cleared.
type rankingCategoryDiff struct {
	Category model.RankingCategory `json:"category"`
	Plans    []rankingPlanLine     `json:"plans"`
	Final    []string              `json:"final"`
	Cleared  bool                  `json:"cleared"`
}

// rankingUpdateDiff is the full payload narrated into chat after an
// engrailed-line ranking update. It is also stowed in the headline post's
// system_data so a future rich renderer can rebuild the view without a schema
// change.
type rankingUpdateDiff struct {
	Categories []rankingCategoryDiff `json:"categories"`
}

// rankingCategoryOrder fixes the display order of categories in the chat
// narration (Power, then Knowledge, then Esteem).
var rankingCategoryOrder = []model.RankingCategory{
	model.CategoryPower,
	model.CategoryKnowledge,
	model.CategoryEsteem,
}

// runRankingUpdate executes the ranking update and returns the updated rankings
// plus a diff describing what moved, for chat narration.
func runRankingUpdate(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
) ([]dbgen.Ranking, *rankingUpdateDiff, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := q.ListPlanTokensByGame(ctx, gameID)
	if err != nil {
		return nil, nil, err
	}

	// Represent each category as a mutable [5]*int64 array (0-indexed = rank−1).
	// A nil element means the slot is held by a static dummy (PlayerID IS NULL).
	// The nil zero-value means no explicit initialization is needed.
	slots := map[model.RankingCategory]*categorySlots{
		model.CategoryPower:     new(categorySlots),
		model.CategoryKnowledge: new(categorySlots),
		model.CategoryEsteem:    new(categorySlots),
	}

	for _, rk := range rankings {
		if rk.Rank < 1 || rk.Rank > 5 {
			continue
		}
		if rk.PlayerID != nil {
			pid := *rk.PlayerID
			slots[rk.Category][rk.Rank-1] = &pid
		}
		// nil PlayerID → slot stays nil (dummy) — zero value already correct.
	}

	// Reverse map: player ID → current rank per category.
	// Kept live — updated after each swap so subsequent swaps use current positions,
	// not the initial snapshot.
	playerRank := make(map[int64]map[model.RankingCategory]int16)
	// startRank is a frozen copy of the pre-swap ranks, used by the diff to
	// decide whether a preparer climbed (final < start) and to order preparers
	// bottom-to-top within a plan line.
	startRank := make(map[int64]map[model.RankingCategory]int16)
	for _, rk := range rankings {
		if rk.PlayerID == nil {
			continue
		}
		if _, ok := playerRank[*rk.PlayerID]; !ok {
			playerRank[*rk.PlayerID] = make(map[model.RankingCategory]int16)
			startRank[*rk.PlayerID] = make(map[model.RankingCategory]int16)
		}
		playerRank[*rk.PlayerID][rk.Category] = rk.Rank
		startRank[*rk.PlayerID][rk.Category] = rk.Rank
	}

	// Phase 2: one plan type per category (extended in future phases).
	categoryPlanTypes := map[model.RankingCategory][]model.PlanType{
		model.CategoryPower:     {model.PlanExchangeCourtiers},
		model.CategoryKnowledge: {model.PlanMakeIntroductions},
		model.CategoryEsteem:    {model.PlanSpreadPropaganda},
	}

	shouldClearTokens, swapped := applyRankingSwaps(slots, playerRank, tokens, categoryPlanTypes)

	// Build the narration diff from the per-holder swap outcomes and the final
	// board (slots, mutated in place by the swaps). startRank is the frozen
	// "before", used only to order preparers bottom-to-top.
	diff := buildRankingDiff(ctx, q, tokens, categoryPlanTypes, startRank, swapped, slots, shouldClearTokens)

	for cat, shouldClear := range shouldClearTokens {
		if shouldClear {
			if err := q.DeletePlanTokensByCategory(ctx, dbgen.DeletePlanTokensByCategoryParams{
				GameID:   gameID,
				Category: cat,
			}); err != nil {
				return nil, nil, err
			}
		}
	}

	// Write all modified slots back to the DB.
	// Each s[i] is *int64: nil → PlayerID IS NULL (dummy), non-nil → real player.
	for cat, s := range slots {
		for i, pid := range s {
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID:   gameID,
				PlayerID: pid, // *int64 maps directly to the nullable column
				Category: cat,
				Rank:     int16(i + 1),
			}); err != nil {
				return nil, nil, err
			}
		}
	}

	final, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return nil, nil, err
	}
	return final, diff, nil
}

// buildRankingDiff assembles the chat-narration diff from the per-holder swap
// outcomes. For each category (in display order) it walks that category's plan
// types in resolution order, and for every plan with at least one preparer
// records the preparers bottom-to-top with an "up" or "top" glyph.
//
// The glyph reflects the operation, not the net result: a holder who performed
// an up-swap is "up" even if a later swap put them back. Read in order, the
// arrows fully reconstruct the final standings — including the cases that net to
// no movement — so no separate "no change" marker is needed.
func buildRankingDiff(
	ctx context.Context,
	q *dbgen.Queries,
	tokens []dbgen.PlanToken,
	categoryPlanTypes map[model.RankingCategory][]model.PlanType,
	startRank map[int64]map[model.RankingCategory]int16,
	swapped map[model.RankingCategory]map[int64]bool,
	finalSlots map[model.RankingCategory]*categorySlots,
	cleared map[model.RankingCategory]bool,
) *rankingUpdateDiff {
	diff := &rankingUpdateDiff{}
	for _, cat := range rankingCategoryOrder {
		catDiff := rankingCategoryDiff{Category: cat, Cleared: cleared[cat]}

		// Final standing, rank 1→5, with "Dummy" for unoccupied (filler) slots —
		// matching the ended-phase rankings display's label convention.
		for _, pid := range finalSlots[cat] {
			if pid == nil {
				catDiff.Final = append(catDiff.Final, "Dummy")
				continue
			}
			catDiff.Final = append(catDiff.Final, playerDisplayName(ctx, q, *pid))
		}

		for _, pt := range categoryPlanTypes[cat] {
			// Preparers of this plan, de-duped, ordered worst rank first so the
			// line reads bottom-to-top like the physical resolution order.
			seen := make(map[int64]struct{})
			var pids []int64
			for _, tok := range tokens {
				if tok.PlanType != pt {
					continue
				}
				if _, dup := seen[tok.PlayerID]; dup {
					continue
				}
				seen[tok.PlayerID] = struct{}{}
				pids = append(pids, tok.PlayerID)
			}
			if len(pids) == 0 {
				continue
			}
			sort.Slice(pids, func(i, j int) bool {
				return startRank[pids[i]][cat] > startRank[pids[j]][cat]
			})

			line := rankingPlanLine{PlanType: pt}
			for _, pid := range pids {
				glyph := "top"
				if swapped[cat][pid] {
					glyph = "up"
				}
				line.Movers = append(line.Movers, rankingMove{
					Name:  playerDisplayName(ctx, q, pid),
					Glyph: glyph,
				})
			}
			catDiff.Plans = append(catDiff.Plans, line)
		}
		diff.Categories = append(diff.Categories, catDiff)
	}
	return diff
}
