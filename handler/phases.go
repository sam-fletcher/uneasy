package handler

import (
	"context"
	"fmt"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// requireFacilitator is a helper that checks the caller is the facilitator
// of the given game. Returns the game, or writes an error response.
func requireFacilitator(w http.ResponseWriter, r *http.Request, q *dbgen.Queries) (*dbgen.Game, bool) {
	gameID, player, ok := parseGamePlayer(w, r, q)
	if !ok {
		return nil, false
	}
	if !player.IsFacilitator {
		respondErr(w, http.StatusForbidden, "only the facilitator can do this")
		return nil, false
	}

	game, err := q.GetGameByID(r.Context(), gameID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "table not found")
		return nil, false
	}

	return &game, true
}

// broadcastPhaseChange sends a phase.changed event to all connected clients.
func broadcastPhaseChange(manager *hub.Manager, gameID int64, phase model.GamePhase) {
	if h, ok := manager.Get(gameID); ok {
		h.BroadcastEvent(model.EventPhaseChanged, model.PhaseChangedPayload{Phase: phase})
	}
}

// findLowestSeatOrderPlayer finds the player with the lowest seat order.
func findLowestSeatOrderPlayer(
	game *dbgen.Game,
	players []dbgen.Player,
) *dbgen.Player {
	if game.FocusPlayerID != nil {
		return &dbgen.Player{ID: *game.FocusPlayerID}
	}
	var lowestPlayer *dbgen.Player
	for _, p := range players {
		if p.SeatOrder != nil {
			if lowestPlayer == nil || *p.SeatOrder < *lowestPlayer.SeatOrder {
				lowestPlayer = &p
			}
		}
	}
	return lowestPlayer
}

// setAndBroadcastFocusPlayer sets the focus player and broadcasts the change.
func setAndBroadcastFocusPlayer(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	lowestPlayer *dbgen.Player,
) bool {
	if lowestPlayer == nil {
		return true
	}
	err := q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID:            gameID,
		FocusPlayerID: &lowestPlayer.ID,
	})
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not set focus player")
		return false
	}
	if fp, err := q.GetPlayerByID(ctx, lowestPlayer.ID); err == nil {
		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventFocusChanged, model.FocusChangedPayload{
				PlayerID:    fp.ID,
				DisplayName: fp.DisplayName,
			})
		}
	}
	return true
}

func validateStartMainEvent(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	gameID int64,
) ([]dbgen.Ranking, []dbgen.Player, bool) {
	// Validate: rankings must be set (3 tracks × 5 positions = 15 entries).
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not load rankings")
		return nil, nil, false
	}
	if len(rankings) < totalRankings {
		respondErr(w, http.StatusBadRequest,
			fmt.Sprintf("rankings must be fully set before starting (all %d tracks × %d positions)",
				planTypes, rankingsPerType),
		)
		return nil, nil, false
	}

	// Validate: every player must have a seat order.
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not load players")
		return nil, nil, false
	}
	for _, p := range players {
		if p.SeatOrder == nil {
			respondErr(w, http.StatusBadRequest, "all players must have a seat order assigned")
			return nil, nil, false
		}
	}

	return rankings, players, true
}

// StartToneSetting handles POST /api/tables/{id}/start-tone-setting.
//
// Transitions the game from lobby → tone_setting. Requires 2–5 players.
// Seeds the default tone topic list.
func StartToneSetting(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, q)
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
		if count < minPlayerCount || count > maxPlayerCount {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("need %d–%d players to start", minPlayerCount, maxPlayerCount))
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
// Transitions the game from tone_setting → prologue and auto-creates one
// empty main-character peer asset per player. The peer's name and
// marginalia get filled in via the existing asset-edit endpoints; we just
// need the row to exist before any title-choice tries to "add the title to
// your main character's peer."
func StartPrologue(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}

		if game.Phase != model.PhaseToneSetting {
			respondErr(w, http.StatusConflict, "game is not in the tone-setting phase")
			return
		}

		ctx := r.Context()

		players, err := q.GetPlayersByGame(ctx, game.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load players")
			return
		}
		for _, p := range players {
			_, err = q.CreateAsset(ctx, dbgen.CreateAssetParams{
				GameID:          game.ID,
				OwnerID:         p.ID,
				CreatorID:       p.ID,
				AssetType:       model.AssetPeer,
				Name:            p.DisplayName,
				IsMainCharacter: true,
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not create main character")
				return
			}
		}

		err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID:    game.ID,
			Phase: model.PhasePrologue,
		})
		if err != nil {
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
		game, ok := requireFacilitator(w, r, q)
		if !ok {
			return
		}

		if game.Phase != model.PhasePrologue {
			respondErr(w, http.StatusConflict, "game is not in the prologue phase")
			return
		}
		// The ranking sub-flow is either not started (legacy facilitator
		// batch-set path) or has reached its terminal step (extra_peers
		// for ≤3 players, or NULL after the last finalize for 4+).
		if game.PrologueRankingStep != nil &&
			*game.PrologueRankingStep != "extra_peers" {
			respondErr(w, http.StatusConflict, "prologue ranking is still in progress")
			return
		}

		ctx := r.Context()

		// Validate preconditions for starting main event.
		_, players, ok := validateStartMainEvent(ctx, w, q, game.ID)
		if !ok {
			return
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
		lowestPlayer := findLowestSeatOrderPlayer(game, players)
		if !setAndBroadcastFocusPlayer(ctx, w, q, manager, game.ID, lowestPlayer) {
			return
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

		result := map[string]any{
			"game":    game,
			"players": players,
		}

		// Include phase-specific data.
		switch game.Phase {
		case model.PhaseLobby, model.PhaseShakeUp:
			// No phase-specific data needed

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
			if game.Phase != model.PhasePrologue {
				if laws, err := q.ListLaws(ctx, gameID); err == nil {
					result["laws"] = laws
				}
				if rumors, err := q.ListRumors(ctx, gameID); err == nil {
					result["rumors"] = rumors
				}
			}
		}

		respond(w, http.StatusOK, result)
	}
}
