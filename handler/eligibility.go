package handler

// handler/eligibility.go — DB-backed plan-eligibility and ranking lookups.
//
// These are I/O helpers (they query Postgres), so per the functional-core /
// imperative-shell split they live in the handler package, not game/. They
// were relocated here from game/ to keep that package free of dbgen.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

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

// checkPlanEligible reports whether playerID may prepare planType on
// currentRow: no token of their own on that plan's shield, no higher-ranked
// player's token on it, and no plan of that type of their own that fell through
// on this row.
func checkPlanEligible(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
	currentRow int16,
	planType model.PlanType,
	category model.RankingCategory,
) (bool, string, error) {
	_, err := q.GetPlanTokenByTypeAndPlayer(ctx, dbgen.GetPlanTokenByTypeAndPlayerParams{
		GameID:   gameID,
		PlanType: planType,
		PlayerID: playerID,
	})
	if err == nil {
		return false, "You already have this plan prepared", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, "", err
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
	fellThrough, err := q.CountFallenThroughPlansOfTypeOnRow(ctx, dbgen.CountFallenThroughPlansOfTypeOnRowParams{
		GameID:        gameID,
		PreparerID:    playerID,
		PlanType:      planType,
		PreparedAtRow: currentRow,
	})
	if err != nil {
		return false, "", err
	}
	if fellThrough > 0 {
		return false, "this plan fell through on this row — prepare a different one, " +
			"or try this again on a later row", nil
	}

	myRank, err := playerRankInCategory(ctx, q, gameID, playerID, category)
	if err != nil {
		return false, "could not determine your ranking", err
	}

	tokens, err := q.ListPlanTokensByType(ctx, dbgen.ListPlanTokensByTypeParams{
		GameID:   gameID,
		PlanType: planType,
	})
	if err != nil {
		return false, "", err
	}
	for _, tok := range tokens {
		theirRank, err := playerRankInCategory(ctx, q, gameID, tok.PlayerID, category)
		if err != nil {
			continue
		}
		if theirRank < myRank {
			return false, "a higher-ranked player already has a token on this plan's shield", nil
		}
	}
	return true, "", nil
}

// playerHasPeers reports whether a player has at least one non-destroyed peer.
func playerHasPeers(ctx context.Context, q *dbgen.Queries, gameID, playerID int64) (bool, error) {
	count, err := q.CountPeerAssets(ctx, dbgen.CountPeerAssetsParams{
		GameID:  gameID,
		OwnerID: playerID,
	})
	return count > 0, err
}

// hasEsteemLockout reports whether a player has an active esteem lockout from
// a Spread Propaganda mar option (b) "censured". This is the I/O shell: it
// loads the player's recent plans (newest-first), maps them to domain views
// (parsing the SP lockout flag), and delegates the decision to the pure
// game.EsteemLockoutActive.
func hasEsteemLockout(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
) (bool, error) {
	plans, err := q.ListRecentPlansByPreparer(ctx, dbgen.ListRecentPlansByPreparerParams{
		GameID:     gameID,
		PreparerID: playerID,
	})
	if err != nil {
		return false, err
	}

	views := make([]game.PlanLockoutView, len(plans))
	for i, p := range plans {
		v := game.PlanLockoutView{Category: p.Category, PlanType: p.PlanType}
		if p.PlanType == model.PlanSpreadPropaganda {
			rd := game.LoadResolutionData(p.ResolutionData)
			v.EsteemLockout = rd.SpreadPropaganda != nil && rd.SpreadPropaganda.EsteemLockout
		}
		views[i] = v
	}
	return game.EsteemLockoutActive(views), nil
}
