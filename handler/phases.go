package handler

import (
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// requireFacilitator is a helper that checks the caller is the facilitator
// of the given game. Returns the game and player, or writes an error response.
func requireFacilitator(w http.ResponseWriter, r *http.Request, q *dbgen.Queries) (*dbgen.Game, *dbgen.Player, bool) {
	gameID, player, ok := parseGamePlayer(w, r)
	if !ok {
		return nil, nil, false
	}
	if !player.IsFacilitator {
		respondErr(w, http.StatusForbidden, "only the facilitator can do this")
		return nil, nil, false
	}

	game, err := q.GetGameByID(r.Context(), gameID)
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
func StartToneSetting(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}

		if game.Phase != model.PhaseLobby {
			respondErr(w, http.StatusConflict, "game is not in the lobby phase")
			return
		}

		ctx := r.Context()

		count, err := q.CountPlayersInGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not count players")
			return
		}
		if count < 2 || count > 5 {
			respondErr(w, http.StatusBadRequest, "need 2–5 players to start")
			return
		}

		// Seed tone topics.
		if err := db.SeedDefaultToneTopics(ctx, q, game.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not seed tone topics")
			return
		}

		// Transition.
		if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID:    game.ID,
			Phase: model.PhaseToneSetting,
		}); err != nil {
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
func StartPrologue(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}

		if game.Phase != model.PhaseToneSetting {
			respondErr(w, http.StatusConflict, "game is not in the tone-setting phase")
			return
		}

		ctx := r.Context()

		if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID:    game.ID,
			Phase: model.PhasePrologue,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update phase")
			return
		}

		broadcastPhaseChange(manager, game.ID, model.PhasePrologue)
		respond(w, http.StatusOK, map[string]any{"phase": model.PhasePrologue})
	}
}

// StartMainEvent handles POST /api/tables/{id}/start-main-event.
//
// Transitions the game from prologue → main_event. Validates that
// rankings are fully set and all players have seat orders. Creates the
// public record rows 1–13 and sets current_row to 1.
func StartMainEvent(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, _, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}

		if game.Phase != model.PhasePrologue {
			respondErr(w, http.StatusConflict, "game is not in the prologue phase")
			return
		}

		ctx := r.Context()

		// Validate: rankings must be set (3 tracks × 5 positions = 15 entries).
		rankings, err := q.ListRankingsByGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}
		if len(rankings) < 15 {
			respondErr(w, http.StatusBadRequest, "rankings must be fully set before starting (all 3 tracks × 5 positions)")
			return
		}

		// Validate: every player must have a seat order.
		players, err := q.GetPlayersByGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load players")
			return
		}
		for _, p := range players {
			if p.SeatOrder == nil {
				respondErr(w, http.StatusBadRequest, "all players must have a seat order assigned")
				return
			}
		}

		// Create public record rows 1–13.
		if err := q.CreatePublicRecordRows(ctx, game.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create public record")
			return
		}

		// Set current row to 1.
		if err := q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
			ID:         game.ID,
			CurrentRow: 1,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not set starting row")
			return
		}

		// Pick the first focus player (lowest seat order).
		var lowestPlayer *dbgen.Player
		if game.FocusPlayerID != nil {
			lowestPlayer = &dbgen.Player{ID: *game.FocusPlayerID}
		} else {
			for _, p := range players {
				if p.SeatOrder != nil {
					if lowestPlayer == nil || *p.SeatOrder < *lowestPlayer.SeatOrder {
						lowestPlayer = &p
					}
				}
			}
		}
		if lowestPlayer != nil {
			if err := q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
				ID:            game.ID,
				FocusPlayerID: &lowestPlayer.ID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not set focus player")
				return
			}
		}

		// Transition phase.
		if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID:    game.ID,
			Phase: model.PhaseMainEvent,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update phase")
			return
		}

		broadcastPhaseChange(manager, game.ID, model.PhaseMainEvent)

		// Also broadcast focus change.
		if lowestPlayer != nil {
			if fp, err := q.GetPlayerByID(ctx, lowestPlayer.ID); err == nil {
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
			"focus_player_id": lowestPlayer.ID,
		})
	}
}

// GetGameState handles GET /api/tables/{id}/state.
//
// Returns the full game state: game object, players, rankings, and phase-specific data.
func GetGameState(q *dbgen.Queries) http.HandlerFunc {
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

		result := map[string]any{
			"game":    game,
			"players": players,
		}

		// Include phase-specific data.
		switch game.Phase {
		case model.PhaseToneSetting:
			topics, err := q.ListToneTopics(ctx, gameID)
			if err == nil {
				result["tone_topics"] = topics
			}

		case model.PhasePrologue, model.PhaseMainEvent, model.PhaseEnded:
			rankings, err := q.ListRankingsByGame(ctx, gameID)
			if err == nil {
				result["rankings"] = rankings
			}
		}

		respond(w, http.StatusOK, result)
	}
}
