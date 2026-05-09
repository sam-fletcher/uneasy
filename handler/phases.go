package handler

import (
	"context"
	"fmt"
	"net/http"

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

// broadcastPhaseChange sends a phase.changed event and writes a boundary
// post into the unified chat feed so the transition is visible inline.
func broadcastPhaseChange(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	phase model.GamePhase,
) {
	if h, ok := manager.Get(gameID); ok {
		h.BroadcastEvent(model.EventPhaseChanged, model.PhaseChangedPayload{Phase: phase})
	}
	EmitSystemPost(ctx, q, manager, gameID, "phase.changed",
		model.SeverityBoundary,
		phaseBoundaryLabel(phase), nil, nil, nil,
		map[string]any{"phase": string(phase)})
}

// phaseBoundaryLabel produces the human-readable boundary text for a phase
// transition. Kept compact — clients render a divider, not a paragraph.
func phaseBoundaryLabel(phase model.GamePhase) string {
	switch phase {
	case model.PhaseLobby:
		return "The lobby is open"
	case model.PhasePrologue:
		return "Prologue begins"
	case model.PhaseMainEvent:
		return "Main event begins"
	case model.PhaseShakeUp:
		return "Shake-up begins"
	case model.PhaseEnded:
		return "Game ends"
	default:
		return "Phase: " + string(phase)
	}
}

// findFirstFocusPlayer picks the first focus player at main-event start.
// Per PROLOGUE_RULES.md, this is the player with the lowest cumulative
// status (sum of ranks across the three tracks), tie broken by lowest
// power rank. If a focus player is already set, that wins.
func findFirstFocusPlayer(
	game *dbgen.Game,
	players []dbgen.Player,
	rankings []dbgen.Ranking,
) *dbgen.Player {
	if game.FocusPlayerID != nil {
		return &dbgen.Player{ID: *game.FocusPlayerID}
	}
	if len(players) == 0 {
		return nil
	}

	totals := make(map[int64]int, len(players))
	powerRank := make(map[int64]int, len(players))
	for _, r := range rankings {
		if r.PlayerID == nil {
			continue
		}
		totals[*r.PlayerID] += int(r.Rank)
		if r.Category == model.CategoryPower {
			powerRank[*r.PlayerID] = int(r.Rank)
		}
	}

	var best *dbgen.Player
	for i := range players {
		p := &players[i]
		if best == nil {
			best = p
			continue
		}
		bt, pt := totals[best.ID], totals[p.ID]
		switch {
		case pt < bt:
			best = p
		case pt == bt:
			if powerRank[p.ID] < powerRank[best.ID] {
				best = p
			}
		}
	}
	return best
}

// setAndBroadcastFocusPlayer sets the focus player and broadcasts the change.
func setAndBroadcastFocusPlayer(
	ctx context.Context,
	w http.ResponseWriter,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	focusPlayer *dbgen.Player,
) bool {
	if focusPlayer == nil {
		return true
	}
	err := q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID:            gameID,
		FocusPlayerID: &focusPlayer.ID,
	})
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not set focus player")
		return false
	}
	if fp, err := q.GetPlayerByID(ctx, focusPlayer.ID); err == nil {
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

	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "could not load players")
		return nil, nil, false
	}

	return rankings, players, true
}

// StartPrologue handles POST /api/tables/{id}/start-prologue.
//
// Transitions the game from lobby → prologue. Requires 2–5 players.
// Tone topics are already seeded at table creation; the Tones page is
// available throughout lobby + prologue and locks at main-event start.
// Main-character peer assets are created at player-join time, not here.
func StartPrologue(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
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

		err = q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID:    game.ID,
			Phase: model.PhasePrologue,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update phase")
			return
		}

		broadcastPhaseChange(ctx, q, manager, game.ID, model.PhasePrologue)
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
		rankings, players, ok := validateStartMainEvent(ctx, w, q, game.ID)
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

		// Pick the first focus player from cumulative ranking totals.
		focusPlayer := findFirstFocusPlayer(game, players, rankings)
		if !setAndBroadcastFocusPlayer(ctx, w, q, manager, game.ID, focusPlayer) {
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

		broadcastPhaseChange(ctx, q, manager, game.ID, model.PhaseMainEvent)

		var focusID *int64
		if focusPlayer != nil {
			focusID = &focusPlayer.ID
		}
		respond(w, http.StatusOK, map[string]any{
			"phase":           model.PhaseMainEvent,
			"current_row":     1,
			"focus_player_id": focusID,
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

		// Tone topics are always available (read-only after main_event begins).
		if topics, err := q.ListToneTopics(ctx, gameID); err == nil {
			result["tone_topics"] = topics
		}

		// Include phase-specific data.
		switch game.Phase {
		case model.PhaseLobby, model.PhaseShakeUp:
			// No further phase-specific data.

		case model.PhasePrologue, model.PhaseMainEvent, model.PhaseEnded:
			rankings, err := q.ListRankingsByGame(ctx, gameID)
			if err == nil {
				result["rankings"] = rankings
			}
			if game.Phase == model.PhasePrologue && game.PrologueRankingStep == nil {
				active, _, err := prologueTurnState(ctx, q, gameID)
				if err == nil {
					var id *int64
					if active != nil {
						id = &active.ID
					}
					result["current_prologue_player_id"] = id
				}
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
