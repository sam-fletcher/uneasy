package handler

import (
	"net/http"

	"uneasy/db"
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
			respondInternalErr(w, r, "could not load rankings", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{"rankings": rankings})
	}
}
