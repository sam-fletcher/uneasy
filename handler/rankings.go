package handler

import (
	"net/http"

	"uneasy/db"
)

const (
	planTypes       = 3
	rankingsPerType = 5 // also status. Impacts dice rolls.
	totalRankings   = planTypes * rankingsPerType
)

// GetRankings handles GET /api/tables/{id}/rankings.
func GetRankings(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		rankings, err := s.Q.ListRankingsByGame(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, "could not load rankings", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{"rankings": rankings})
	}
}
