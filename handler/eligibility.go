package handler

// handler/eligibility.go — DB-backed plan-eligibility and ranking lookups.
//
// These are I/O helpers (they query Postgres), so per the functional-core /
// imperative-shell split they live in the handler package, not game/. They
// were relocated here from game/ to keep that package free of dbgen.
//
// The checks read from an eligibilityBoard: one snapshot of the game's plan
// tokens, rankings and plans, loaded once per request. The prep grid asks the
// same questions for all twelve plan types, and asking them per type used to
// cost four lookups each (own token, fall-through count, own rank, the
// shield's tokens, then a rank per token) — 50-odd serial round trips on a
// GET that runs on every main-event table load. Against a remote database
// that was over a second. The board answers them all from three reads.

import (
	"context"
	"sort"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// playerRankInCategory returns the player's rank (1–5) in the given category.
func playerRankInCategory(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
	category model.RankingCategory,
) (int16, error) {
	r, err := q.GetRanking(ctx, dbgen.GetRankingParams{
		GameID:   gameID,
		PlayerID: &playerID,
		Category: category,
	})
	if err != nil {
		return 0, err
	}
	return r.Rank, nil
}

// eligibilityBoard is the read-only snapshot behind the plan-eligibility
// checks: every plan token, ranking row and plan in one game. Build it with
// loadEligibilityBoard once per request and pass it down; it never queries.
type eligibilityBoard struct {
	gameID   int64
	tokens   []dbgen.PlanToken
	rankings []dbgen.Ranking
	plans    []dbgen.Plan
}

// loadEligibilityBoard reads the three tables the board needs. The reads are
// independent and fan out (see fanOut), so on the request path the board
// costs one round trip, not three.
func loadEligibilityBoard(ctx context.Context, q *dbgen.Queries, gameID int64) (*eligibilityBoard, error) {
	b := &eligibilityBoard{gameID: gameID}
	var tokensErr, ranksErr, plansErr error
	run, wait := fanOut(q)
	run(func() { b.tokens, tokensErr = q.ListPlanTokensByGame(ctx, gameID) })
	run(func() { b.rankings, ranksErr = q.ListRankingsByGame(ctx, gameID) })
	run(func() { b.plans, plansErr = q.ListPlansByGame(ctx, gameID) })
	wait()
	for _, err := range []error{tokensErr, ranksErr, plansErr} {
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

// rank returns playerID's rank in category, or ok=false when the player has
// no ranking row there (a dummy token's row carries no player).
func (b *eligibilityBoard) rank(playerID int64, category model.RankingCategory) (int16, bool) {
	for _, r := range b.rankings {
		if r.Category == category && r.PlayerID != nil && *r.PlayerID == playerID {
			return r.Rank, true
		}
	}
	return 0, false
}

// checkPlanEligible reports whether playerID may prepare planType on
// currentRow: no token of their own on that plan's shield, no higher-ranked
// player's token on it, and no plan of that type of their own that fell through
// on this row. The reason is user-facing.
func (b *eligibilityBoard) checkPlanEligible(
	playerID int64,
	currentRow int16,
	planType model.PlanType,
	category model.RankingCategory,
) (bool, string) {
	for _, tok := range b.tokens {
		if tok.PlanType == planType && tok.PlayerID == playerID {
			return false, "You already have this plan prepared"
		}
	}

	// A plan of this type that fell through on this row blocks a re-pick of the
	// same type until the next row. This used to happen by accident — the plan
	// token was never deleted on a fall-through — and the token is now removed
	// (the shield records real preparations only, per
	// adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §6), so the block is derived from
	// prepared_at_row instead. It is wanted on its own merits: the delay faces
	// of the two plans that can fall through are CHOSEN, not rolled, so a free
	// retry would let a preparer re-declare until the average lands where they
	// want, making the reveal meaningless.
	//
	// Scoped to the row, not the turn: a player can hold focus more than once on
	// a row when several plans share it, and the retry is just as available on
	// the second turn as on the first.
	for i := range b.plans {
		p := &b.plans[i]
		if p.PreparerID == playerID && p.PlanType == planType &&
			p.Status == model.PlanCancelled && p.PreparedAtRow == currentRow {
			return false, "this plan fell through on this row — prepare a different one, " +
				"or try this again on a later row"
		}
	}

	myRank, ok := b.rank(playerID, category)
	if !ok {
		return false, "could not determine your ranking"
	}
	for _, tok := range b.tokens {
		if tok.PlanType != planType {
			continue
		}
		theirRank, ok := b.rank(tok.PlayerID, category)
		if !ok {
			continue
		}
		if theirRank < myRank {
			return false, "a higher-ranked player already has a token on this plan's shield"
		}
	}
	return true, ""
}

// hasEsteemLockout reports whether a player has an active esteem lockout from
// a Spread Propaganda mar option (b) "censured": it takes the player's recent
// plans newest-first (the same window ListRecentPlansByPreparer returned),
// maps them to domain views (parsing the SP lockout flag), and delegates the
// decision to the pure game.EsteemLockoutActive.
func (b *eligibilityBoard) hasEsteemLockout(playerID int64) bool {
	const recentWindow = 20
	recent := make([]dbgen.Plan, 0, recentWindow)
	for i := range b.plans {
		if b.plans[i].PreparerID == playerID {
			recent = append(recent, b.plans[i])
		}
	}
	sort.SliceStable(recent, func(i, j int) bool {
		if recent[i].PreparedAtRow != recent[j].PreparedAtRow {
			return recent[i].PreparedAtRow > recent[j].PreparedAtRow
		}
		return recent[i].ID > recent[j].ID
	})
	if len(recent) > recentWindow {
		recent = recent[:recentWindow]
	}

	views := make([]game.PlanLockoutView, len(recent))
	for i, p := range recent {
		v := game.PlanLockoutView{Category: p.Category, PlanType: p.PlanType}
		if p.PlanType == model.PlanSpreadPropaganda {
			rd := game.LoadResolutionData(p.ResolutionData)
			v.EsteemLockout = rd.SpreadPropaganda != nil && rd.SpreadPropaganda.EsteemLockout
		}
		views[i] = v
	}
	return game.EsteemLockoutActive(views)
}

// finaleSlotSpent is the board's answer to CountFinaleBonusPlans: the
// player's one Explosive Finale plan is spent iff a live bonus plan of theirs
// exists. Same status filter as the query, for the same (today vacuous) reason.
func (b *eligibilityBoard) finaleSlotSpent(playerID int64) bool {
	for i := range b.plans {
		p := &b.plans[i]
		if p.PreparerID == playerID && p.IsFinaleBonus && p.Status != model.PlanCancelled {
			return true
		}
	}
	return false
}

// playerHasPeers reports whether a player has at least one non-destroyed peer.
func playerHasPeers(ctx context.Context, q *dbgen.Queries, gameID, playerID int64) (bool, error) {
	count, err := q.CountPeerAssets(ctx, dbgen.CountPeerAssetsParams{
		GameID:  gameID,
		OwnerID: playerID,
	})
	return count > 0, err
}
