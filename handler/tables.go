package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

// CreateTable handles POST /api/tables.
//
// Creates a new game table and seats the calling player as facilitator.
// Requires the player to have a cookie identity (POST /api/identity first).
func CreateTable(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ut := appMiddleware.UserTokenFromContext(r.Context())
		if ut == nil {
			respondErr(w, http.StatusUnauthorized, "set an identity first")
			return
		}
		if ut.DisplayName == "" {
			respondErr(w, http.StatusBadRequest, "set a display name before creating a table")
			return
		}

		token := appMiddleware.RawTokenFromRequest(r)

		// Check that they're not already in a game (Phase 1 constraint).
		if existing := appMiddleware.PlayerFromContext(r.Context()); existing != nil {
			respondErr(w, http.StatusConflict, "already in a game — leave it first")
			return
		}

		ctx := r.Context()

		// Create the game row.
		game, err := db.CreateGame(ctx, pool)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create table")
			return
		}

		// Seat the creator as facilitator.
		player, err := db.CreatePlayer(ctx, pool, game.ID, ut.DisplayName, token, true)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create player")
			return
		}

		// Link facilitator on the game row.
		if err := db.SetFacilitator(ctx, pool, game.ID, player.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not set facilitator")
			return
		}
		game.FacilitatorID = &player.ID

		// Pre-create the hub so the WebSocket endpoint is ready immediately.
		manager.GetOrCreate(game.ID)

		respond(w, http.StatusCreated, map[string]any{
			"game":   game,
			"player": player,
		})
	}
}

// JoinTable handles POST /api/tables/join.
//
// Adds the calling player to an existing table via join code.
func JoinTable(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ut := appMiddleware.UserTokenFromContext(r.Context())
		if ut == nil {
			respondErr(w, http.StatusUnauthorized, "set an identity first")
			return
		}

		var body struct {
			JoinCode string `json:"join_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.JoinCode = strings.ToUpper(strings.TrimSpace(body.JoinCode))
		if body.JoinCode == "" {
			respondErr(w, http.StatusBadRequest, "join_code is required")
			return
		}

		// Phase 1: one game per person.
		if existing := appMiddleware.PlayerFromContext(r.Context()); existing != nil {
			respondErr(w, http.StatusConflict, "already in a game")
			return
		}

		ctx := r.Context()
		token := appMiddleware.RawTokenFromRequest(r)

		game, err := db.GetGameByJoinCode(ctx, pool, body.JoinCode)
		if err != nil {
			respondErr(w, http.StatusNotFound, "join code not found")
			return
		}

		player, err := db.CreatePlayer(ctx, pool, game.ID, ut.DisplayName, token, false)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not join table")
			return
		}

		// Notify anyone already connected via WebSocket.
		if h, ok := manager.Get(game.ID); ok {
			players, _ := db.GetPlayersByGame(ctx, pool, game.ID)
			members := make([]any, len(players))
			for i, p := range players {
				members[i] = map[string]any{
					"id":           p.ID,
					"display_name": p.DisplayName,
					"online":       false, // presence is from the hub; HTTP join is offline
				}
			}
			_ = h // presence update is handled automatically when the WS connects
		}

		respond(w, http.StatusCreated, map[string]any{
			"game":   game,
			"player": player,
		})
	}
}

// GetTable handles GET /api/tables/{id}.
//
// Returns table details and the current member list.
func GetTable(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid table id")
			return
		}

		ctx := r.Context()

		// Require membership.
		player := appMiddleware.PlayerFromContext(r.Context())
		if player == nil || player.GameID != gameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
			return
		}

		game, err := db.GetGameByID(ctx, pool, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}

		players, err := db.GetPlayersByGame(ctx, pool, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load members")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"game":    game,
			"players": players,
		})
	}
}
