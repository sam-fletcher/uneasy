package handler

import (
	"net/http"

	dbgen "uneasy/db/gen"
)

const (
	planTypes       = 3
	rankingsPerType = 5 // also status. Impacts dice rolls.
	totalRankings   = planTypes * rankingsPerType
)

// GetRankings handles GET /api/tables/{id}/rankings.
func GetRankings(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}

		rankings, err := q.ListRankingsByGame(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}

		respond(w, http.StatusOK, map[string]any{"rankings": rankings})
	}
}
