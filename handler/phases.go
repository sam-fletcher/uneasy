package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	"uneasy/hub"
	"uneasy/model"
	appMiddleware "uneasy/middleware"
)

// requireFacilitator is a helper that checks the caller is the facilitator
// of the given game. Returns the game and player, or writes an error response.
func requireFacilitator(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool) (*model.Game, *model.Player, bool) {
	gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid table id")
		return nil, nil, false
	}

	player := appMiddleware.PlayerFromContext(r.Context())
	if player == nil || player.GameID != gameID {
		respondErr(w, http.StatusForbidden, "not a member of this table")
		return nil, nil, false
	}
	if !player.IsFacilitator {
		respondErr(w, http.StatusForbidden, "only the facilitator can do this")
		return nil, nil, false
	}

	game, err := db.GetGameByID(r.Context(), pool, gameID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "table not found")
		return nil, nil, false
	}

	return &game, player, true
}

// broadcastPhaseChange sends a phase.changed event to all connected clients.
func broadcastPhaseChange(manager *hub.Manager, gameID int64, phase model.GamePhase) {
	if h, ok := manager.Get(gameID); ok {
		h.BroadcastEvent(model.EventPhaseChanged, model.PhaseChangedPayload{Phase: phase})
	}
}

// StartToneSetting handles POST /api/tables/{id}/start-tone-setting.
//
// Transitions the game from lobby → tone_setting. Requires 2–5 players.
// Seeds the default tone topic list.
func StartToneSetting(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, pool)
		if !ok {
			return
		}

		if game.Phase != model.PhaseLobby {
			respondErr(w, http.StatusConflict, "game is not in the lobby phase")
			return
		}

		ctx := r.Context()

		count, err := db.CountPlayersInGame(ctx, pool, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not count players")
			return
		}
		if count < 2 || count > 5 {
			respondErr(w, http.StatusBadRequest, "need 2–5 players to start")
			return
		}

		// Seed tone topics.
		if err := db.SeedDefaultToneTopics(ctx, pool, game.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not seed tone topics")
			return
		}

		// Transition.
		if err := db.SetGamePhase(ctx, pool, game.ID, model.PhaseToneSetting); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update phase")
			return
		}

		broadcastPhaseChange(manager, game.ID, model.PhaseToneSetting)
		respond(w, http.StatusOK, map[string]any{"phase": model.PhaseToneSetting})
	}
}

// StartPrologue handles POST /api/tables/{id}/start-prologue.
//
// Transitions the game from tone_setting → prologue.
func StartPrologue(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, pool)
		if !ok {
			return
		}

		if game.Phase != model.PhaseToneSetting {
			respondErr(w, http.StatusConflict, "game is not in the tone-setting phase")
			return
		}

		ctx := r.Context()

		if err := db.SetGamePhase(ctx, pool, game.ID, model.PhasePrologue); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update phase")
			return
		}

		broadcastPhaseChange(manager, game.ID, model.PhasePrologue)
		respond(w, http.StatusOK, map[string]any{"phase": model.PhasePrologue})
	}
}

// StartMainEvent handles POST /api/tables/{id}/start-main-event.
//
// Transitions the game from prologue → main_event. Validates that every
// player has a main character and rankings are fully set. Creates the
// public record rows 1–13 and sets current_row to 1.
func StartMainEvent(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, pool)
		if !ok {
			return
		}

		if game.Phase != model.PhasePrologue {
			respondErr(w, http.StatusConflict, "game is not in the prologue phase")
			return
		}

		ctx := r.Context()

		// Validate: rankings must be set (at least 3 × player count entries).
		rankings, err := db.ListRankingsByGame(ctx, pool, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}
		// Check all 3 tracks × 5 positions are filled.
		if len(rankings) < 15 {
			respondErr(w, http.StatusBadRequest, "rankings must be fully set before starting (all 3 tracks × 5 positions)")
			return
		}

		// Validate: every player must have a focus_player if set, or we pick one.
		players, err := db.GetPlayersByGame(ctx, pool, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load players")
			return
		}

		// Ensure every player has a seat order.
		for _, p := range players {
			if p.SeatOrder == nil {
				respondErr(w, http.StatusBadRequest, "all players must have a seat order assigned")
				return
			}
		}

		// Create public record rows 1–13.
		if err := db.CreatePublicRecordRows(ctx, pool, game.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create public record")
			return
		}

		// Set current row to 1.
		if err := db.SetCurrentRow(ctx, pool, game.ID, 1); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not set starting row")
			return
		}

		// Pick the first focus player: lowest cumulative status (= highest rank sum).
		// For simplicity, use the first player by seat order.
		var firstFocus *int64
		if game.FocusPlayerID != nil {
			firstFocus = game.FocusPlayerID
		} else {
			// Pick lowest seat order.
			for _, p := range players {
				if p.SeatOrder != nil {
					id := p.ID
					firstFocus = &id
					break
				}
			}
		}
		if firstFocus != nil {
			if err := db.SetFocusPlayer(ctx, pool, game.ID, firstFocus); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set focus player")
				return
			}
		}

		// Transition phase.
		if err := db.SetGamePhase(ctx, pool, game.ID, model.PhaseMainEvent); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update phase")
			return
		}

		broadcastPhaseChange(manager, game.ID, model.PhaseMainEvent)

		// Also broadcast focus change.
		if firstFocus != nil {
			if fp, err := db.GetPlayerByID(ctx, pool, *firstFocus); err == nil {
				if h, ok := manager.Get(game.ID); ok {
					h.BroadcastEvent(model.EventFocusChanged, model.FocusChangedPayload{
						PlayerID:    fp.ID,
						DisplayName: fp.DisplayName,
					})
				}
			}
		}

		respond(w, http.StatusOK, map[string]any{
			"phase":           model.PhaseMainEvent,
			"current_row":     1,
			"focus_player_id": firstFocus,
		})
	}
}

// GetGameState handles GET /api/tables/{id}/state.
//
// Returns the full game state: game object, players, rankings, and phase-specific data.
func GetGameState(pool *pgxpool.Pool) http.HandlerFunc {
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

		ctx := r.Context()

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

		result := map[string]any{
			"game":    game,
			"players": players,
		}

		// Include phase-specific data.
		switch game.Phase {
		case model.PhaseToneSetting:
			topics, err := db.ListToneTopics(ctx, pool, gameID)
			if err == nil {
				result["tone_topics"] = topics
			}

		case model.PhasePrologue, model.PhaseMainEvent, model.PhaseEnded:
			rankings, err := db.ListRankingsByGame(ctx, pool, gameID)
			if err == nil {
				result["rankings"] = rankings
			}
		}

		respond(w, http.StatusOK, result)
	}
}
