package handler

import (
	"encoding/json"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/model"
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

// SetSeats handles PUT /api/tables/{id}/seats.
//
// Facilitator assigns seat order to all players.
func SetSeats(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}

		if game.Phase != model.PhasePrologue {
			respondErr(w, http.StatusConflict, "seats can only be set during the prologue")
			return
		}

		var body struct {
			Seats []struct {
				PlayerID  int64 `json:"player_id"`
				SeatOrder int16 `json:"seat_order"`
			} `json:"seats"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		for _, seat := range body.Seats {
			if err := q.SetPlayerSeatOrder(ctx, dbgen.SetPlayerSeatOrderParams{
				ID:        seat.PlayerID,
				SeatOrder: &seat.SeatOrder,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set seat order")
				return
			}
		}

		respond(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}
