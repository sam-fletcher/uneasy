package handler

// handler/prologue_closing.go — The `closing` prologue step ("The Stage is
// Set"), Session 1 of adr/PROLOGUE_CLOSING_STAGE_PLAN.md.
//
// Every player count converges on this step once ranking finishes (see the
// resolveTrack / PlaceSetAsides rewire in prologue_committed_hearts.go /
// prologue_ranking.go). Players ready up via ClosingReady; once everyone is
// ready the server re-validates the hard conditions one more time (a player
// could have un-named their main character after readying — there is no
// un-ready hook on UpdateAsset) and, if they all still pass, advances to
// main_event. CreateExtraPeer shares the same all-ready check, since
// completing the last extra peer can be what makes everyone ready.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ClosingReadyView is the JSON shape for one player's ready flag, returned
// as part of GetPrologueRankingState's closing_ready field.
type ClosingReadyView struct {
	PlayerID int64 `json:"player_id"`
	Ready    bool  `json:"ready"`
}

// ClosingReady handles POST /api/tables/{id}/prologue/closing-ready.
//
// Body: {"ready": bool}. Un-readying is always allowed, like the track-done
// toggle. Readying requires the caller's main character to be named (not
// blank, not the creation placeholder) and, in games of 3 or fewer players,
// their extra peer to already exist. When this write makes every player
// ready, the server re-validates those same conditions for everyone before
// advancing to main_event.
func ClosingReady(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		game := loadGameForPrologue(w, ctx, s.Q, gameID)
		if game == nil {
			return
		}
		if !requirePrologueStep(w, game, gamepkg.PrologueStepClosing) {
			return
		}

		var body struct {
			Ready bool `json:"ready"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if body.Ready {
			reason, err := closingReadyGateFailure(ctx, s.Q, gameID, player.ID)
			if err != nil {
				respondInternalErr(w, r, "could not check readiness", err)
				return
			}
			if reason != "" {
				respondErr(w, http.StatusConflict, reason)
				return
			}
		}

		err := s.InTx(ctx, func(q *dbgen.Queries) error {
			if sErr := q.SetClosingReady(ctx, dbgen.SetClosingReadyParams{
				GameID: gameID, PlayerID: player.ID, Ready: body.Ready,
			}); sErr != nil {
				return errors.New("could not save readiness")
			}
			if body.Ready {
				return maybeAdvanceFromClosing(ctx, q, manager, gameID)
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		broadcastEvent(manager, gameID, model.EventPrologueClosingReadyChg,
			model.PrologueClosingReadyChangedPayload{PlayerID: player.ID, Ready: body.Ready})
		respond(w, http.StatusOK, map[string]any{"ready": body.Ready})
	}
}

// closingReadyGateFailure checks the hard-gate conditions for one player and
// returns a human-readable reason if they fail, or "" if they pass.
func closingReadyGateFailure(ctx context.Context, q *dbgen.Queries, gameID, playerID int64) (string, error) {
	mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: gameID, OwnerID: playerID,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No main character at all — can't happen in the prologue in
		// practice (MC assets are created at join time and the main-event
		// replacement flow doesn't exist yet), but is unambiguously
		// "not named" if it ever does.
		return "name your main character first", nil
	case err != nil:
		return "", err
	case strings.TrimSpace(mc.Name) == "" || mc.Name == model.MainCharacterPlaceholder:
		return "name your main character first", nil
	}
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return "", err
	}
	if len(players) <= 3 {
		exists, err := q.ExtraPeerExistsForPlayer(ctx, dbgen.ExtraPeerExistsForPlayerParams{
			GameID: gameID, PlayerID: playerID,
		})
		if err != nil {
			return "", err
		}
		if !exists {
			return "create your extra peer first", nil
		}
	}
	return "", nil
}

// maybeAdvanceFromClosing re-reads the step for idempotency against
// concurrent requests, then — once every player is marked ready —
// re-validates the hard-gate conditions for all of them before advancing to
// main_event. A player who was ready but now fails re-validation (e.g.
// renamed their main character back to the placeholder) has their ready
// flag cleared and the change broadcast instead of the game advancing.
// Shared by ClosingReady and CreateExtraPeer, since completing the last
// extra peer can itself complete the all-ready condition.
func maybeAdvanceFromClosing(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID int64) error {
	fresh, err := q.GetGameByID(ctx, gameID)
	if err != nil {
		return err
	}
	if fresh.PrologueRankingStep == nil || *fresh.PrologueRankingStep != gamepkg.PrologueStepClosing {
		// Another request already advanced past closing. Nothing to do.
		return nil
	}
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return err
	}
	readyRows, err := q.ListClosingReadyByGame(ctx, gameID)
	if err != nil {
		return err
	}
	ready := make(map[int64]bool, len(readyRows))
	for _, row := range readyRows {
		if row.Ready {
			ready[row.PlayerID] = true
		}
	}
	for _, p := range players {
		if !ready[p.ID] {
			return nil // not everyone's ready yet
		}
	}

	allOK := true
	for _, p := range players {
		reason, rErr := closingReadyGateFailure(ctx, q, gameID, p.ID)
		if rErr != nil {
			return rErr
		}
		if reason == "" {
			continue
		}
		allOK = false
		if sErr := q.SetClosingReady(ctx, dbgen.SetClosingReadyParams{
			GameID: gameID, PlayerID: p.ID, Ready: false,
		}); sErr != nil {
			return sErr
		}
		broadcastEvent(manager, gameID, model.EventPrologueClosingReadyChg,
			model.PrologueClosingReadyChangedPayload{PlayerID: p.ID, Ready: false})
	}
	if !allOK {
		return nil
	}
	return advanceToMainEvent(ctx, q, manager, gameID)
}
