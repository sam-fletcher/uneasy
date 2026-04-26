package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

const maxPlayersPerGame = 5

// CreateTable handles POST /api/tables.
//
// Creates a new game table and seats the calling account as facilitator.
// Requires a logged-in session.
func CreateTable(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
			return
		}

		ctx := r.Context()

		code, err := db.GenerateJoinCode()
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not generate join code")
			return
		}

		game, err := q.CreateGame(ctx, code)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create table")
			return
		}

		player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   acct.Username,
			AccountID:     acct.ID,
			IsFacilitator: true,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create player")
			return
		}

		if err := q.SetFacilitator(ctx, dbgen.SetFacilitatorParams{
			FacilitatorID: &player.ID,
			ID:            game.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not set facilitator")
			return
		}
		game.FacilitatorID = &player.ID

		manager.GetOrCreate(game.ID)

		respond(w, http.StatusCreated, map[string]any{
			"game":   game,
			"player": player,
		})
	}
}

// JoinTable handles POST /api/tables/join.
//
// Adds the calling account to an existing table via join code. Idempotent
// if the account is already seated. Rejects if the table is at the
// hard-coded 5-player cap.
func JoinTable(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct := appMiddleware.AccountFromContext(r.Context())
		if acct == nil {
			respondErr(w, http.StatusUnauthorized, "log in first")
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

		ctx := r.Context()

		game, err := q.GetGameByJoinCode(ctx, body.JoinCode)
		if err != nil {
			respondErr(w, http.StatusNotFound, "join code not found")
			return
		}

		// Already seated → idempotent success.
		existing, err := q.GetPlayerByAccountAndGame(ctx, dbgen.GetPlayerByAccountAndGameParams{
			AccountID: acct.ID,
			GameID:    game.ID,
		})
		if err == nil {
			respond(w, http.StatusOK, map[string]any{"game": game, "player": existing})
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			respondErr(w, http.StatusInternalServerError, "could not check membership")
			return
		}

		// Capacity check (not race-free; acceptable for ~10 users).
		count, err := q.CountPlayersInGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check capacity")
			return
		}
		if count >= maxPlayersPerGame {
			respondErr(w, http.StatusConflict, "table is full")
			return
		}

		player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   acct.Username,
			AccountID:     acct.ID,
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
func GetTable(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
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
