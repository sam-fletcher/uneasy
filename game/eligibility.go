package game

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// CheckPlanEligible reports whether playerID may prepare planType.
func CheckPlanEligible(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
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

	myRank, err := PlayerRankInCategory(ctx, q, gameID, playerID, category)
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
		theirRank, err := PlayerRankInCategory(ctx, q, gameID, tok.PlayerID, category)
		if err != nil {
			continue
		}
		if theirRank < myRank {
			return false, "a higher-ranked player already has a token on this plan's shield", nil
		}
	}
	return true, "", nil
}

// PlayerHasPeers reports whether a player has at least one non-destroyed peer.
func PlayerHasPeers(ctx context.Context, q *dbgen.Queries, gameID, playerID int64) (bool, error) {
	count, err := q.CountPeerAssets(ctx, dbgen.CountPeerAssetsParams{
		GameID:  gameID,
		OwnerID: playerID,
	})
	return count > 0, err
}

// HasEsteemLockout reports whether a player has an active esteem lockout from
// a Spread Propaganda mar option (b) "censured". The lockout is active when
// the player's most recently prepared plan in chronological order is an esteem
// plan whose ResData.EsteemLockout is true. It clears the moment any non-esteem
// plan is prepared (that plan becomes the most recent).
//
// Algorithm: iterate recent plans newest-first. The first non-esteem plan
// proves the lockout has cleared. The first SP plan with EsteemLockout = true
// (with no non-esteem plan seen yet) proves it's still active.
func HasEsteemLockout(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID int64,
) (bool, error) {
	plans, err := q.ListRecentPlansByPreparer(ctx, dbgen.ListRecentPlansByPreparerParams{
		GameID:     gameID,
		PreparerID: playerID,
	})
	if err != nil || len(plans) == 0 {
		return false, err
	}

	for _, p := range plans {
		if p.Category != model.CategoryEsteem {
			// Non-esteem plan found after (newer than) any SP lockout → cleared.
			return false, nil
		}
		if p.PlanType == model.PlanSpreadPropaganda {
			rd := LoadResolutionData(p.ResolutionData)
			if rd.SpreadPropaganda != nil && rd.SpreadPropaganda.EsteemLockout {
				return true, nil
			}
		}
	}
	return false, nil
}
