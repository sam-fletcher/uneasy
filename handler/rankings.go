package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
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

// SetRankings handles PUT /api/tables/{id}/rankings.
//
// Facilitator batch-sets all rankings. Expects a JSON array of
// {player_id (nullable for dummy), category, rank} objects.
// All 15 positions (3 tracks × 5 ranks) must be provided.
func SetRankings(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}

		if game.Phase != model.PhasePrologue {
			respondErr(w, http.StatusConflict, "rankings can only be set during the prologue")
			return
		}

		var body struct {
			Rankings []struct {
				PlayerID *int64 `json:"player_id"`
				Category string `json:"category"`
				Rank     int16  `json:"rank"`
			} `json:"rankings"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if len(body.Rankings) != totalRankings {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("must provide exactly %d rankings (%d tracks × %d positions)",
					totalRankings, planTypes, rankingsPerType))
			return
		}

		ctx := r.Context()

		// Validate each entry.
		for _, entry := range body.Rankings {
			cat := model.RankingCategory(entry.Category)
			if cat != model.CategoryPower && cat != model.CategoryKnowledge && cat != model.CategoryEsteem {
				respondErr(w, http.StatusBadRequest, "invalid category: "+entry.Category)
				return
			}
			if entry.Rank < 1 || entry.Rank > 5 {
				respondErr(w, http.StatusBadRequest, "rank must be 1–5")
				return
			}
		}

		// Clear existing rankings and re-set.
		if err := q.DeleteRankingsByGame(ctx, game.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not clear rankings")
			return
		}

		for _, entry := range body.Rankings {
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID:   game.ID,
				PlayerID: entry.PlayerID,
				Category: model.RankingCategory(entry.Category),
				Rank:     entry.Rank,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set ranking")
				return
			}
		}

		// Fetch and broadcast the new rankings.
		rankings, err := q.ListRankingsByGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}

		if h, ok := manager.Get(game.ID); ok {
			h.BroadcastEvent(model.EventRankingsUpdated, model.RankingsUpdatedPayload{Rankings: rankings})
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
