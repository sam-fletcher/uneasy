package handler

// handler/endgame.go — Endgame mode selection (Phase 4d).
//
// When a plan would land past row 13, the game must choose how to end.
// The rulebook offers three modes; we implement two:
//
//   - smooth_landing — disallow plans past row 13; let in-flight plans
//     complete on their existing rows; transition to Shake-Up.
//   - explosive_finale — collapse all unprepared plan slots onto row 13;
//     each plan resolves with no scenes between; then Shake-Up.
//
// long_campaign (a second public-record sheet) is not in scope and is
// rejected by /endgame even though the schema allows the value.

import (
	"encoding/json"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// Ending mode constants (mirror games.ending_mode CHECK values).
const (
	EndingModeSmoothLanding   = "smooth_landing"
	EndingModeExplosiveFinale = "explosive_finale"
	EndingModeLongCampaign    = "long_campaign"
)

// SetEndgameMode handles POST /api/tables/{id}/endgame.
//
// Body: {"mode": "smooth_landing" | "explosive_finale"}.
// Facilitator-only. Idempotent — overwrites any prior selection. Long
// Campaign is intentionally rejected (deferred indefinitely).
//
// THIS IS A BROKEN-STATE BACKSTOP, NOT A NORMAL PATH. The ending mode is settled
// by the table vote at the row 7 → 8 boundary (handler/ending_vote.go) — every
// player votes, and an automatic tie-break resolves a split. This route survives
// only to unstick a game whose ending_mode ended up wrong or missing through a
// bug; it is emphatically not an override to reach for when a vote is slow, since
// resolving a decision that belongs to a non-responder is exactly what
// adr/FACILITATOR_POWERS_AUDIT.md rejected.
//
// It deliberately does NOT clear games.ending_vote_open, so it cannot be used to
// short-circuit a vote in progress: the row-state gate keeps naming whoever still
// owes a vote, and the tally overwrites whatever this route wrote the moment the
// last vote lands. That is the point — this route is not the escape hatch the
// audit cut.
//
// Delete it once the vote has run in a real game and it proves unnecessary. The
// PhaseMainEvent guard stays.
func SetEndgameMode(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		game, ok := requireFacilitator(w, r, s.Q)
		if !ok {
			return
		}
		if game.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "endgame mode can only be set during the main event")
			return
		}

		var body struct {
			Mode string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		switch body.Mode {
		case EndingModeSmoothLanding, EndingModeExplosiveFinale:
			// allowed
		case EndingModeLongCampaign:
			respondErr(w, http.StatusBadRequest, "long_campaign is not yet implemented")
			return
		default:
			respondErr(w, http.StatusBadRequest,
				"mode must be smooth_landing or explosive_finale")
			return
		}

		mode := body.Mode
		err := s.Q.SetEndingMode(r.Context(), dbgen.SetEndingModeParams{
			ID: game.ID, EndingMode: &mode,
		})
		if err != nil {
			respondInternalErr(w, r, "could not set endgame mode", err)
			return
		}

		broadcastEvent(manager, game.ID, model.EventEndgameModeSet, model.EndgameModeSetPayload{Mode: mode})
		respond(w, http.StatusOK, map[string]any{"mode": mode})
	}
}
