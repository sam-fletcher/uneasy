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
//     ranking track. If already at rank 1, or the slot above is a dummy (static
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

	// Represent each category as a mutable [5]int64 array (0-indexed = rank−1).
	// dummySentinel marks a slot occupied by a dummy token (player_id IS NULL).
	const dummySentinel = int64(-1)
	type catSlots [5]int64

	slots := map[model.RankingCategory]*catSlots{
		model.CategoryPower:     new(catSlots),
		model.CategoryKnowledge: new(catSlots),
		model.CategoryEsteem:    new(catSlots),
	}
	for _, s := range slots {
		for i := range s {
			s[i] = dummySentinel
		}
	}
	for _, rk := range rankings {
		if rk.Rank < 1 || rk.Rank > 5 {
			continue
		}
		s := slots[rk.Category]
		if rk.PlayerID == nil {
			s[rk.Rank-1] = dummySentinel
		} else {
			s[rk.Rank-1] = *rk.PlayerID
		}
	}

	// Reverse map: player ID → current rank per category.
	// This is kept live — updated after each swap so subsequent swaps use
	// current positions, not the initial snapshot.
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

			above := s[aboveIdx]
			if above == dummySentinel {
				continue // static dummy above — cannot advance past it
			}

			// Swap pid and above in both the slot array and the live rank map.
			s[aboveIdx] = pid
			s[myIdx] = above
			playerRank[pid][cat] = aboveIdx + 1
			if _, ok := playerRank[above]; ok {
				playerRank[above][cat] = myIdx + 1
			}
		}

		// After all tokens in this category are resolved: if every plan type
		// on this sheet has at least one token, clear all tokens for the category.
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
		if allHaveTokens {
			if err := q.DeletePlanTokensByCategory(ctx, dbgen.DeletePlanTokensByCategoryParams{
				GameID:   gameID,
				Category: cat,
			}); err != nil {
				return nil, err
			}
		}
	}

	// Write all modified slots back to the DB.
	for cat, s := range slots {
		for i, pid := range s {
			rank := int16(i + 1)
			var playerIDPtr *int64
			if pid != dummySentinel {
				p := pid
				playerIDPtr = &p
			}
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID:   gameID,
				PlayerID: playerIDPtr,
				Category: cat,
				Rank:     rank,
			}); err != nil {
				return nil, err
			}
		}
	}

	return q.ListRankingsByGame(ctx, gameID)
}
