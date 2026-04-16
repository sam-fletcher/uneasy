package game

import (
	"context"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// PlayerRankInCategory returns the player's rank (1–5) in the given category.
func PlayerRankInCategory(
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
