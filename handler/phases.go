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
// status — the underdog. Because rank 1 is the *highest* status (see
// difficulty_test.go), lowest status means the highest sum of ranks across
// the three tracks. Ties are broken by lowest power status, i.e. the highest
// power rank number. If a focus player is already set, that wins.
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
		case pt > bt:
			// Higher rank sum = lower status = more of an underdog.
			best = p
		case pt == bt:
			// Tie on total: the lower-status-on-power player (higher power
			// rank number) takes the marker.
			if powerRank[p.ID] > powerRank[best.ID] {
				best = p
			}
		}
	}
	return best
}

// advanceToMainEvent performs the prologue → main_event transition: seeds
// public record rows 1–13, sets current_row, picks the first focus player,
// flips phase, and broadcasts. Callers must have verified prologue-complete
// preconditions (rankings fully set; extra peers done for ≤3 players).
func advanceToMainEvent(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
) error {
	game, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load game: %w", err)
	}
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load players: %w", err)
	}
	rankings, err := q.ListRankingsByGame(ctx, gameID)
	if err != nil {
		return fmt.Errorf("load rankings: %w", err)
	}

	if err := q.CreatePublicRecordRows(ctx, gameID); err != nil {
		return fmt.Errorf("create public record: %w", err)
	}
	if err := q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: gameID, CurrentRow: 1,
	}); err != nil {
		return fmt.Errorf("set current row: %w", err)
	}

	focusPlayer := findFirstFocusPlayer(&game, players, rankings)
	if focusPlayer != nil {
		if err := q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
			ID: gameID, FocusPlayerID: &focusPlayer.ID,
		}); err != nil {
			return fmt.Errorf("set focus player: %w", err)
		}
		if fp, err := q.GetPlayerByID(ctx, focusPlayer.ID); err == nil {
			if h, ok := manager.Get(gameID); ok {
				h.BroadcastEvent(model.EventFocusChanged, model.FocusChangedPayload{
					PlayerID:    fp.ID,
					DisplayName: fp.DisplayName,
				})
			}
		}
		broadcastRowState(ctx, q, manager, gameID)
	}

	if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: gameID, Phase: model.PhaseMainEvent,
	}); err != nil {
		return fmt.Errorf("update phase: %w", err)
	}
	broadcastPhaseChange(ctx, q, manager, gameID, model.PhaseMainEvent)
	return nil
}

// StartPrologue handles POST /api/tables/{id}/start-prologue.
//
// Transitions the game from lobby → prologue. Requires 2–5 players.
// Tone topics are already seeded at table creation; the Tones page is
// available throughout lobby + prologue and locks at main-event start.
// Main-character peer assets are created at player-join time, not here.
func StartPrologue(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, s.Q)
		if !ok {
			return
		}

		if game.Phase != model.PhaseLobby {
			respondErr(w, http.StatusConflict, "game is not in the lobby phase")
			return
		}

		ctx := r.Context()

		count, err := s.Q.CountPlayersInGame(ctx, game.ID)
		if err != nil {
			respondInternalErr(w, r, "could not count players", err)
			return
		}
		if count < minPlayerCount || count > maxPlayerCount {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("need %d–%d players to start", minPlayerCount, maxPlayerCount))
			return
		}

		err = s.Q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID:    game.ID,
			Phase: model.PhasePrologue,
		})
		if err != nil {
			respondInternalErr(w, r, "could not update phase", err)
			return
		}

		broadcastPhaseChange(ctx, s.Q, manager, game.ID, model.PhasePrologue)
		respond(w, http.StatusOK, map[string]any{"phase": model.PhasePrologue})
	}
}

// GetGameState handles GET /api/tables/{id}/state.
//
// Returns the full game state: game object, players, rankings, and phase-specific data.
func GetGameState(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		ctx := r.Context()

		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}

		players, err := s.Q.GetPlayersByGame(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load members", err)
			return
		}

		result := map[string]any{
			"game":    game,
			"players": players,
		}

		// Tone topics are always available (read-only after main_event begins).
		if topics, err := s.Q.ListToneTopics(ctx, gameID); err == nil {
			result["tone_topics"] = topics
		}

		// Laws & rumors are visible in every phase — their header buttons sit
		// alongside Tones and stay accessible from t=0 (the lists are empty
		// until the prologue's Laws & Rumors box is claimed).
		if laws, err := s.Q.ListLaws(ctx, gameID); err == nil {
			result["laws"] = laws
		}
		if rumors, err := s.Q.ListRumors(ctx, gameID); err == nil {
			result["rumors"] = rumors
		}

		// Include phase-specific data.
		switch game.Phase {
		case model.PhaseLobby:
			// No further phase-specific data.

		case model.PhasePrologue, model.PhaseMainEvent, model.PhaseShakeUp, model.PhaseEnded:
			// Shake-up needs rankings too: turn order for both steps (reverse
			// rank) and the bump-rank spend can move them mid-endgame.
			rankings, err := s.Q.ListRankingsByGame(ctx, gameID)
			if err == nil {
				result["rankings"] = rankings
			}
			if game.Phase == model.PhasePrologue && game.PrologueRankingStep == nil {
				active, _, err := prologueTurnState(ctx, s.Q, gameID)
				if err == nil {
					var id *int64
					if active != nil {
						id = &active.ID
					}
					result["current_prologue_player_id"] = id
				}
			}
			// In main_event, surface the authoritative RowState (which step
			// of the row are we in?) so the client renders directly off
			// the server's verdict instead of inferring from event side
			// effects. See model/row_state.go.
			if game.Phase == model.PhaseMainEvent && game.CurrentRow > 0 {
				if rs, err := ComputeRowState(ctx, s.Q, gameID); err == nil {
					result["row_state"] = rs
				}
			}
		}

		respond(w, http.StatusOK, result)
	}
}
