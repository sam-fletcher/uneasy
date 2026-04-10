package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

// CreateTable handles POST /api/tables.
//
// Creates a new game table and seats the calling player as facilitator.
// Requires the player to have a cookie identity (POST /api/identity first).
func CreateTable(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
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

		// Generate a unique join code.
		code, err := db.GenerateJoinCode()
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not generate join code")
			return
		}

		// Create the game row.
		game, err := q.CreateGame(ctx, code)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create table")
			return
		}

		// Seat the creator as facilitator.
		player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   ut.DisplayName,
			CookieToken:   token,
			IsFacilitator: true,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create player")
			return
		}

		// Link facilitator on the game row.
		if err := q.SetFacilitator(ctx, dbgen.SetFacilitatorParams{
			FacilitatorID: &player.ID,
			ID:            game.ID,
		}); err != nil {
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
func JoinTable(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
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

		game, err := q.GetGameByJoinCode(ctx, body.JoinCode)
		if err != nil {
			respondErr(w, http.StatusNotFound, "join code not found")
			return
		}

		player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   ut.DisplayName,
			CookieToken:   token,
			IsFacilitator: false,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not join table")
			return
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
func GetTable(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}

		ctx := r.Context()

		game, err := q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}

		players, err := q.GetPlayersByGame(ctx, gameID)
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
