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
// Modifies slots and playerRank in place. Returns a map indicating, for each
// category, whether all plan types in that category had at least one token holder
// (and thus tokens should be cleared after this call).
func applyRankingSwaps(
	slots map[model.RankingCategory]*categorySlots,
	playerRank map[int64]map[model.RankingCategory]int16,
	tokens []dbgen.PlanToken,
	categoryPlanTypes map[model.RankingCategory][]model.PlanType,
) map[model.RankingCategory]bool {
	shouldClearTokens := make(map[model.RankingCategory]bool)

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

		for _, pid := range tokenPlayers {
			rankMap, ok := playerRank[pid]
			if !ok {
				continue
			}
			myRank := rankMap[cat] // 1-indexed, live value
			if myRank <= 1 {
				continue // already at top, do nothing
			}
			aboveIdx := myRank - 2 //nolint:mnd // 0-indexed slot one rank above
			myIdx := myRank - 1    // 0-indexed current slot

			above := s[aboveIdx] // *int64, nil if dummy
			if above == nil {
				continue // static dummy above — cannot advance past it
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
		}

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

	return shouldClearTokens
}

// runRankingUpdate executes the ranking update and returns the updated rankings.
func runRankingUpdate(ctx context.Context, q *dbgen.Queries, gameID int64) ([]dbgen.Ranking, error) {
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return nil, err
	}
	tokens, err := q.ListPlanTokensByGame(ctx, gameID)
	if err != nil {
		return nil, err
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
	for _, rk := range rankings {
		if rk.PlayerID == nil {
			continue
		}
		if _, ok := playerRank[*rk.PlayerID]; !ok {
			playerRank[*rk.PlayerID] = make(map[model.RankingCategory]int16)
		}
		playerRank[*rk.PlayerID][rk.Category] = rk.Rank
	}

	// Phase 2: one plan type per category (extended in future phases).
	categoryPlanTypes := map[model.RankingCategory][]model.PlanType{
		model.CategoryPower:     {model.PlanExchangeCourtiers},
		model.CategoryKnowledge: {model.PlanMakeIntroductions},
		model.CategoryEsteem:    {model.PlanSpreadPropaganda},
	}

	shouldClearTokens := applyRankingSwaps(slots, playerRank, tokens, categoryPlanTypes)

	for cat, shouldClear := range shouldClearTokens {
		if shouldClear {
			if err := q.DeletePlanTokensByCategory(ctx, dbgen.DeletePlanTokensByCategoryParams{
				GameID:   gameID,
				Category: cat,
			}); err != nil {
				return nil, err
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
				return nil, err
			}
		}
	}

	return q.ListRankingsByGame(ctx, gameID)
}
