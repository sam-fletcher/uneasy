package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"

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
	// Clear the ranking step so "phase == main_event" implies
	// "prologue_ranking_step IS NULL" — the closing-ready idempotency guard
	// (maybeAdvanceFromClosing's fresh re-read) relies on this to recognize
	// a concurrent request already advanced the game, since the step would
	// otherwise be left stuck at "closing" forever.
	if err := q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: gameID, PrologueRankingStep: nil,
	}); err != nil {
		return fmt.Errorf("clear ranking step: %w", err)
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

// gameStateFanOut bounds how many of a state load's independent reads run at
// once. Each concurrent query holds one pool connection for its duration, and
// pgxpool's default MaxConns is max(4, NumCPU) — a request that grabbed the
// whole pool would stall every other request behind it. Four is enough to
// collapse the serial chain (these reads are ~25ms apiece against a Neon in
// another region, and there are up to seven of them) without monopolising it.
const gameStateFanOut = 4

// gameStateReads is the phase-dependent half of a state load. Every field is
// optional except players: the rest are best-effort, and a failure costs the
// one thing it feeds rather than the whole table.
type gameStateReads struct {
	players    []dbgen.Player
	playersErr error

	activity   []dbgen.ListPlayerActivityByGameRow
	activityOK bool
	topics     []dbgen.ToneTopic
	topicsOK   bool
	laws       []dbgen.Law
	lawsOK     bool
	rumors     []dbgen.Rumor
	rumorsOK   bool
	rankings   []dbgen.Ranking
	rankingsOK bool

	rowState   model.RowState
	rowStateOK bool

	prologueActiveID  *int64
	prologueActiveSet bool
}

// loadGameStateReads runs every read a state load needs beyond the game row
// itself. They are mutually independent and were sequential only because they
// were written that way, which cost one Neon round trip each on the hottest
// path in the app. Each goroutine writes its own fields and nothing else, so
// no mutex is involved — returning the struct after Wait() is what publishes
// them.
func loadGameStateReads(ctx context.Context, q *dbgen.Queries, game dbgen.Game) *gameStateReads {
	out := &gameStateReads{}
	gameID := game.ID

	var wg sync.WaitGroup
	sem := make(chan struct{}, gameStateFanOut)
	run := func(fn func()) {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			fn()
		})
	}

	// The only read whose failure is fatal — the client cannot render a roster
	// it does not have.
	run(func() { out.players, out.playersErr = q.GetPlayersByGame(ctx, gameID) })

	// Presence/reminder summary for the Retinue header. Best-effort: a failure
	// here costs a header line, not the table.
	run(func() {
		var e error
		out.activity, e = q.ListPlayerActivityByGame(ctx, gameID)
		out.activityOK = e == nil
	})
	// Tone topics are always available (read-only after main_event begins).
	run(func() {
		var e error
		out.topics, e = q.ListToneTopics(ctx, gameID)
		out.topicsOK = e == nil
	})
	// Laws & rumors are visible in every phase — their header buttons sit
	// alongside Tones and stay accessible from t=0 (the lists are empty until
	// the prologue's Laws & Rumors box is claimed).
	run(func() {
		var e error
		out.laws, e = q.ListLaws(ctx, gameID)
		out.lawsOK = e == nil
	})
	run(func() {
		var e error
		out.rumors, e = q.ListRumors(ctx, gameID)
		out.rumorsOK = e == nil
	})

	if game.Phase != model.PhaseLobby {
		// Shake-up needs rankings too: turn order for both steps (reverse rank)
		// and the bump-rank spend can move them mid-endgame.
		run(func() {
			var e error
			out.rankings, e = q.ListRankingsByGame(ctx, gameID)
			out.rankingsOK = e == nil
		})
		run(func() { out.loadPhaseSpecific(ctx, q, game) })
	}

	wg.Wait()
	return out
}

// loadPhaseSpecific covers the two reads only one phase each can want. They
// share a slot in the fan-out above because no game is ever in both phases.
func (out *gameStateReads) loadPhaseSpecific(ctx context.Context, q *dbgen.Queries, game dbgen.Game) {
	switch {
	case game.Phase == model.PhasePrologue && game.PrologueRankingStep == nil:
		active, _, err := prologueTurnState(ctx, q, game.ID)
		if err != nil {
			return
		}
		if active != nil {
			out.prologueActiveID = &active.ID
		}
		out.prologueActiveSet = true

	// In main_event, surface the authoritative RowState (which step of the row
	// are we in?) so the client renders directly off the server's verdict
	// instead of inferring from event side effects. See model/row_state.go.
	//
	// computeRowStateForGame, not ComputeRowState: the latter re-fetches the
	// game we already hold, which was a second GetGameByID on every state load.
	case game.Phase == model.PhaseMainEvent && game.CurrentRow > 0:
		rs, err := computeRowStateForGame(ctx, q, game)
		if err != nil {
			return
		}
		out.rowState, out.rowStateOK = rs, true
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

		// Fetched first and alone: every read below is gated on the phase, so
		// this one round trip is genuinely serial. Everything after it is not.
		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}

		reads := loadGameStateReads(ctx, s.Q, game)
		if reads.playersErr != nil {
			respondInternalErr(w, r, "could not load members", reads.playersErr)
			return
		}

		result := map[string]any{"game": game, "players": reads.players}
		if reads.activityOK {
			result["player_activity"] = buildPlayerActivity(reads.activity)
		}
		if reads.topicsOK {
			result["tone_topics"] = reads.topics
		}
		if reads.lawsOK {
			result["laws"] = reads.laws
		}
		if reads.rumorsOK {
			result["rumors"] = reads.rumors
		}
		if reads.rankingsOK {
			result["rankings"] = reads.rankings
		}
		if reads.prologueActiveSet {
			result["current_prologue_player_id"] = reads.prologueActiveID
		}
		if reads.rowStateOK {
			result["row_state"] = reads.rowState
		}

		respond(w, http.StatusOK, result)
	}
}
