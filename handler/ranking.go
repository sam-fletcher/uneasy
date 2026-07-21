package handler

// handler/ranking.go — Engrailed-line ranking update algorithm (Phase 2f).
//
// runRankingUpdate is called by advanceRowInner (turn.go) whenever current_row
// crosses an engrailed line (after rows 4, 8, 12).
//
// Algorithm per rulebook §"Updating Rankings" (resolution order confirmed by
// the designer):
//
//  1. Process categories in display order: Power, then Knowledge, then Esteem.
//     The three sheets are independent, so the order only matters for a
//     consistent action log.
//  2. Within a sheet, resolve every *token* one-by-one — NOT one per player.
//     Start at the bottom-most plan of the sheet and move up; within a plan,
//     start at the bottom-most token (the first player to have prepared it) and
//     move up the stack. For each token, swap that player with the token one
//     position above them on the sheet's ranking track. If already at the top
//     (or only dummy/static slots sit above), do nothing. Ranks update live, so
//     a player who holds several tokens on a sheet climbs once per token.
//  3. After processing all tokens in a category: if every plan on that sheet has
//     at least one token, clear all tokens for that category (returning them to
//     their players).
//  4. Upsert all modified ranking slots back to the DB.
//
// Token order within a plan is placement order (earliest first), read from the
// token row id — NOT the holders' current ranks, which may have shifted since
// they prepared. Plan order within a sheet is the reverse of categorySheetPlans
// (which lists plans top-to-bottom), so the bottom-most plan resolves first.

import (
	"context"
	"sort"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// categorySlots represents a single ranking category's slot array (rank 1-5).
// Index i holds the player ID at rank i+1, or nil if the slot is unoccupied (dummy).
type categorySlots [5]*int64

// tokensForPlan returns the tokens on one plan, ordered bottom-to-top of the
// stack — i.e. by placement order (earliest-prepared first), which the row id
// (a BIGSERIAL) tracks. This is the order the rules resolve a plan's stack in,
// and it is deliberately NOT the holders' current ranks: a holder's rank may
// have shifted since they prepared, but their position in the stack does not.
func tokensForPlan(tokens []dbgen.PlanToken, pt model.PlanType) []dbgen.PlanToken {
	var out []dbgen.PlanToken
	for _, tok := range tokens {
		if tok.PlanType == pt {
			out = append(out, tok)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// applyRankingSwaps resolves every token on each sheet one-by-one and determines
// whether tokens should be cleared for each category. Each token is its own
// swap, so a player holding several tokens on a sheet climbs once per token.
//
// Modifies slots and playerRank in place. Returns two maps:
//   - shouldClearTokens[cat]: whether every plan on that sheet had at least one
//     token (and thus tokens should be cleared after this call).
//   - swapped[cat][planType][playerID]: whether that specific token performed an
//     up-swap (true) or found only the top / dummy slots above it (false). The
//     chat narration reads this to show an up-arrow vs. a crown.
func applyRankingSwaps(
	slots map[model.RankingCategory]*categorySlots,
	playerRank map[int64]map[model.RankingCategory]int16,
	tokens []dbgen.PlanToken,
	categoryPlanTypes map[model.RankingCategory][]model.PlanType,
) (map[model.RankingCategory]bool, map[model.RankingCategory]map[model.PlanType]map[int64]bool) {
	shouldClearTokens := make(map[model.RankingCategory]bool)
	swapped := make(map[model.RankingCategory]map[model.PlanType]map[int64]bool)

	for cat, planTypes := range categoryPlanTypes {
		s := slots[cat]
		catSwapped := make(map[model.PlanType]map[int64]bool)
		swapped[cat] = catSwapped

		// Resolve plans bottom-to-top on the sheet (categoryPlanTypes lists them
		// top-to-bottom, so iterate in reverse), and within each plan resolve its
		// stack in placement order. Each token triggers one up-swap.
		allHaveTokens := true
		for i := len(planTypes) - 1; i >= 0; i-- {
			planTokens := tokensForPlan(tokens, planTypes[i])
			if len(planTokens) == 0 {
				allHaveTokens = false
				continue
			}
			planSwapped := make(map[int64]bool)
			catSwapped[planTypes[i]] = planSwapped
			for _, tok := range planTokens {
				planSwapped[tok.PlayerID] = swapTokenPlayerWithAbove(tok.PlayerID, cat, s, playerRank)
			}
		}

		// Clear the sheet's tokens only if every plan on it had a holder.
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
// PlayerID lets the emitter mark the name (playerMark) when it writes the
// chat body; Name itself stays plain, since this struct is also stowed in
// system_data where log markup has no meaning.
type rankingMove struct {
	PlayerID int64  `json:"player_id"`
	Name     string `json:"name"`
	Glyph    string `json:"glyph"`
}

// rankingPlanLine is one prepared plan and the preparers it affected, ordered
// bottom-to-top of the token stack (placement order, earliest preparer first)
// to match the rules' resolution order.
type rankingPlanLine struct {
	PlanType model.PlanType `json:"plan_type"`
	Movers   []rankingMove  `json:"movers"`
}

// rankStanding is one occupied slot in a category's final standing: a real
// player and their (true, 1-indexed) rank. Dummy/filler slots are a backend
// contrivance for sub-5-player games and are never represented here — they are
// the absence of a player, so they are omitted entirely from the narration. A
// gap in the rank numbers (e.g. "2 Sam · 4 Charlie") is the only trace they
// leave.
// PlayerID is carried for the same reason as rankingMove's: the body is marked
// at emission time, the stowed JSON keeps a plain name.
type rankStanding struct {
	Rank     int16  `json:"rank"`
	PlayerID int64  `json:"player_id"`
	Name     string `json:"name"`
}

// rankingCategoryDiff is one category's narration: its prepared plans in
// resolution order, the final standing (occupied ranks only, dummies omitted),
// and whether the category's preparations cleared.
type rankingCategoryDiff struct {
	Category model.RankingCategory `json:"category"`
	Plans    []rankingPlanLine     `json:"plans"`
	Final    []rankStanding        `json:"final"`
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

// categorySheetPlans is the full composition of each ranking category's plan
// preparation sheet: all four plans on that sheet, in the rules' top-to-bottom
// listing order (THE_12_PLANS_RULES.md). The ranking update walks these to
// gather token holders and to decide whether every plan on the sheet was
// prepared (which clears the sheet's tokens). The set is asserted against the
// plan registry in TestCategorySheetPlansMatchRegistry so it can't drift.
var categorySheetPlans = map[model.RankingCategory][]model.PlanType{
	model.CategoryPower: {
		model.PlanMakeDemands,
		model.PlanProposeDecree,
		model.PlanExchangeCourtiers,
		model.PlanMakeWar,
	},
	model.CategoryKnowledge: {
		model.PlanMakeIntroductions,
		model.PlanSeekAnswers,
		model.PlanChronicleHistories,
		model.PlanClandestinelyLiaise,
	},
	model.CategoryEsteem: {
		model.PlanSpreadPropaganda,
		model.PlanSpreadRumors,
		model.PlanProposeDuel,
		model.PlanHostFestivity,
	},
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
	for _, rk := range rankings {
		if rk.PlayerID == nil {
			continue
		}
		if _, ok := playerRank[*rk.PlayerID]; !ok {
			playerRank[*rk.PlayerID] = make(map[model.RankingCategory]int16)
		}
		playerRank[*rk.PlayerID][rk.Category] = rk.Rank
	}

	categoryPlanTypes := categorySheetPlans

	shouldClearTokens, swapped := applyRankingSwaps(slots, playerRank, tokens, categoryPlanTypes)

	// Build the narration diff from the per-token swap outcomes and the final
	// board (slots, mutated in place by the swaps).
	diff := buildRankingDiff(ctx, q, tokens, categoryPlanTypes, swapped, slots, shouldClearTokens)

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

// buildRankingDiff assembles the chat-narration diff from the per-token swap
// outcomes. For each category (in display order) it walks that category's plans
// in resolution order (bottom-most plan first), and for every plan with at least
// one token records its holders in placement order with an "up" or "top" glyph.
//
// The glyph reflects the operation, not the net result: a token that performed
// an up-swap is "up" even if a later swap put its holder back. Read in order,
// the arrows fully reconstruct the final standings — including the cases that
// net to no movement — so no separate "no change" marker is needed.
func buildRankingDiff(
	ctx context.Context,
	q *dbgen.Queries,
	tokens []dbgen.PlanToken,
	categoryPlanTypes map[model.RankingCategory][]model.PlanType,
	swapped map[model.RankingCategory]map[model.PlanType]map[int64]bool,
	finalSlots map[model.RankingCategory]*categorySlots,
	cleared map[model.RankingCategory]bool,
) *rankingUpdateDiff {
	diff := &rankingUpdateDiff{}
	for _, cat := range rankingCategoryOrder {
		catDiff := rankingCategoryDiff{Category: cat, Cleared: cleared[cat]}

		// Final standing — occupied ranks only. Unoccupied (dummy/filler) slots
		// are skipped entirely; their rank number is simply absent from the list.
		for i, pid := range finalSlots[cat] {
			if pid == nil {
				continue
			}
			catDiff.Final = append(catDiff.Final, rankStanding{
				Rank:     int16(i + 1),
				PlayerID: *pid,
				Name:     playerPlainName(ctx, q, *pid),
			})
		}

		// Plans resolve bottom-to-top on the sheet; categoryPlanTypes lists them
		// top-to-bottom, so walk it in reverse to match the resolution sequence.
		planTypes := categoryPlanTypes[cat]
		for i := len(planTypes) - 1; i >= 0; i-- {
			pt := planTypes[i]
			// Holders of this plan, in placement order (bottom-most token first).
			planTokens := tokensForPlan(tokens, pt)
			if len(planTokens) == 0 {
				continue
			}

			line := rankingPlanLine{PlanType: pt}
			for _, tok := range planTokens {
				glyph := "top"
				if swapped[cat][pt][tok.PlayerID] {
					glyph = "up"
				}
				line.Movers = append(line.Movers, rankingMove{
					PlayerID: tok.PlayerID,
					Name:     playerPlainName(ctx, q, tok.PlayerID),
					Glyph:    glyph,
				})
			}
			catDiff.Plans = append(catDiff.Plans, line)
		}
		diff.Categories = append(diff.Categories, catDiff)
	}
	return diff
}
