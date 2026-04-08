package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	"uneasy/hub"
	"uneasy/model"
	appMiddleware "uneasy/middleware"
)

// GetRankings handles GET /api/tables/{id}/rankings.
func GetRankings(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid table id")
			return
		}

		player := appMiddleware.PlayerFromContext(r.Context())
		if player == nil || player.GameID != gameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
			return
		}

		rankings, err := db.ListRankingsByGame(r.Context(), pool, gameID)
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
func SetRankings(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, pool)
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

		if len(body.Rankings) != 15 {
			respondErr(w, http.StatusBadRequest, "must provide exactly 15 rankings (3 tracks × 5 positions)")
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
		if err := db.DeleteRankingsByGame(ctx, pool, game.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not clear rankings")
			return
		}

		for _, entry := range body.Rankings {
			if err := db.UpsertRanking(ctx, pool, game.ID, entry.PlayerID, model.RankingCategory(entry.Category), entry.Rank); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set ranking")
				return
			}
		}

		// Fetch and broadcast the new rankings.
		rankings, err := db.ListRankingsByGame(ctx, pool, game.ID)
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
func SetSeats(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, pool)
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
			if err := db.SetPlayerSeatOrder(ctx, pool, seat.PlayerID, seat.SeatOrder); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set seat order")
				return
			}
		}

		respond(w, http.StatusOK, map[string]any{"status": "ok"})
	}
}
