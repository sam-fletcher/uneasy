package handler

// handler/reveals.go — Simultaneous Reveals endpoints (Phase 3c).
//
// Simultaneous reveals coordinate multi-player "all submit before any see"
// die reveal. Used by:
//   - Clandestinely Liaise delay (liaise_delay)
//   - Clandestinely Liaise re-delay (liaise_redelay)
//   - Make War delay (make_war_delay, Phase 3d)
//
// When a player submits, the server broadcasts reveal.submitted with no face.
// When ALL participants have submitted, the server marks the reveal complete,
// broadcasts reveal.complete with all faces, and triggers any downstream
// effects (e.g. updating the linked plan's row_number for liaise_delay).
//
// Routes (mounted in main.go):
//   POST /api/reveals/:revealId/submit   Submit a die face.
//   GET  /api/reveals/:revealId          Read reveal state (faces hidden until complete).

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── GetReveal ─────────────────────────────────────────────────────────────────

// GetReveal handles GET /api/reveals/:revealId.
//
// Returns the reveal state. Faces are hidden (null) until is_complete = true.
func GetReveal(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reveal, _, ok := requireRevealAccess(w, r, q)
		if !ok {
			return
		}

		ctx := r.Context()
		entries, err := q.ListRevealEntries(ctx, reveal.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load reveal entries")
			return
		}

		// Build response: hide faces until the reveal is complete.
		type entryView struct {
			PlayerID  int64  `json:"player_id"`
			Submitted bool   `json:"submitted"`
			Face      *int16 `json:"face,omitempty"` // only present when complete
		}
		views := make([]entryView, 0, len(entries))
		for _, e := range entries {
			ev := entryView{
				PlayerID:  e.PlayerID,
				Submitted: e.RevealedAt.Valid,
			}
			if reveal.IsComplete && e.Face != nil {
				ev.Face = e.Face
			}
			views = append(views, ev)
		}

		respond(w, http.StatusOK, map[string]any{
			"reveal":  reveal,
			"entries": views,
		})
	}
}

// ── SubmitReveal ──────────────────────────────────────────────────────────────

// SubmitReveal handles POST /api/reveals/:revealId/submit.
//
// A participant submits their die face. For liaise_delay and make_war_delay,
// face must be 1–6. For liaise_redelay, face may be 0 (cancel).
//
// When all participants have submitted, the reveal is marked complete, all
// faces are broadcast, and any linked plan effects are applied.
//
// Request body: {"face": N}
//
//nolint:funlen // simultaneous reveal lifecycle
func SubmitReveal(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reveal, player, ok := requireRevealAccess(w, r, q)
		if !ok {
			return
		}
		if reveal.IsComplete {
			respondErr(w, http.StatusConflict, "reveal is already complete")
			return
		}

		var body struct {
			Face int16 `json:"face"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Validate face range by reveal type.
		minFace := int16(1)
		if reveal.RevealType == "liaise_redelay" {
			minFace = 0 // 0 means "cancel future meeting"
		}
		if body.Face < minFace || body.Face > 6 {
			respondErr(w, http.StatusBadRequest,
				"face must be between "+strconv.Itoa(int(minFace))+" and 6")
			return
		}

		ctx := r.Context()

		// Verify this player is a registered participant.
		entry, err := q.GetRevealEntry(ctx, dbgen.GetRevealEntryParams{
			RevealID: reveal.ID,
			PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusForbidden, "you are not a participant in this reveal")
			return
		}
		if entry.RevealedAt.Valid {
			respondErr(w, http.StatusConflict, "you have already submitted your reveal")
			return
		}

		// Record the face.
		face := body.Face
		err = q.SetRevealEntryFace(ctx, dbgen.SetRevealEntryFaceParams{
			RevealID: reveal.ID,
			PlayerID: player.ID,
			Face:     &face,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record your reveal")
			return
		}

		// Broadcast that this player submitted (face still hidden).
		broadcastEvent(manager, reveal.GameID, model.EventRevealSubmitted, model.RevealSubmittedPayload{
			RevealID: reveal.ID,
			PlayerID: player.ID,
		})

		// Check if all participants have now submitted.
		submitted, err := q.CountRevealEntriesSubmitted(ctx, reveal.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check reveal status")
			return
		}
		total, err := q.CountRevealEntries(ctx, reveal.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not count reveal entries")
			return
		}

		if submitted < total {
			// Not everyone has submitted yet.
			respond(w, http.StatusOK, map[string]any{
				"reveal_id":       reveal.ID,
				"submitted_count": submitted,
				"total_count":     total,
				"is_complete":     false,
			})
			return
		}

		// All submitted — compute result delay and complete the reveal.
		entries, err := q.ListRevealEntries(ctx, reveal.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load reveal entries")
			return
		}

		resultDelay := revealCeilAverage(entries)
		if err := q.SetRevealComplete(ctx, dbgen.SetRevealCompleteParams{
			ID:          reveal.ID,
			ResultDelay: &resultDelay,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not complete reveal")
			return
		}

		// Build payload with all faces.
		entryResults := make([]model.RevealEntryResult, 0, len(entries))
		for _, e := range entries {
			var f int16
			if e.Face != nil {
				f = *e.Face
			}
			entryResults = append(entryResults, model.RevealEntryResult{
				PlayerID: e.PlayerID,
				Face:     f,
			})
		}
		broadcastEvent(manager, reveal.GameID, model.EventRevealComplete, model.RevealCompletePayload{
			RevealID:    reveal.ID,
			Entries:     entryResults,
			ResultDelay: resultDelay,
		})

		// Apply downstream effects for liaise_delay: set the plan's row_number.
		if reveal.RevealType == "liaise_delay" && reveal.PlanID != nil {
			applyLiaiseDelayResult(ctx, q, manager, *reveal.PlanID, resultDelay)
		}
		if reveal.RevealType == "make_war_delay" && reveal.PlanID != nil {
			applyMakeWarDelayResult(ctx, q, manager, *reveal.PlanID, resultDelay)
		}

		respond(w, http.StatusOK, map[string]any{
			"reveal_id":    reveal.ID,
			"result_delay": resultDelay,
			"entries":      entryResults,
			"is_complete":  true,
		})
	}
}

// ── applyLiaiseDelayResult ────────────────────────────────────────────────────

// applyLiaiseDelayResult updates the linked CL plan's row_number after the
// delay reveal completes. If the computed row exceeds row 13, the plan is
// cancelled instead.
func applyLiaiseDelayResult(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	planID int64,
	resultDelay int16,
) {
	plan, err := q.GetPlanByID(ctx, planID)
	if err != nil {
		return
	}
	game, err := q.GetGameByID(ctx, plan.GameID)
	if err != nil {
		return
	}

	targetRow := game.CurrentRow + resultDelay

	if targetRow > publicRecordRowCount {
		// No room — cancel the plan.
		_ = q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID:     planID,
			Status: model.PlanCancelled,
		})
		broadcastEvent(manager, plan.GameID, model.EventPlanResolved, model.PlanResolvedPayload{
			PlanID: planID,
			Result: "cancelled",
		})
		EmitPlanResolved(ctx, q, manager, plan, "cancelled")
		return
	}

	// Update the plan's row_number to the computed target.
	_ = q.SetPlanRowNumber(ctx, dbgen.SetPlanRowNumberParams{
		ID:        planID,
		RowNumber: targetRow,
	})

	// Refresh plan to get the updated row_number for the boundary anchor.
	if refreshed, err := q.GetPlanByID(ctx, planID); err == nil {
		plan = refreshed
	}
	broadcastEvent(manager, plan.GameID, model.EventPlanPrepared, model.PlanPayload{Plan: plan})
	EmitPlanPrepared(ctx, q, manager, plan)
}

// ── requireRevealAccess ───────────────────────────────────────────────────────

// requireRevealAccess parses the revealId URL param, loads the reveal, and
// verifies the caller belongs to the same game.
func requireRevealAccess(
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
) (*dbgen.SimultaneousReveal, *dbgen.Player, bool) {
	revealID, err := strconv.ParseInt(chi.URLParam(r, "revealId"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid reveal id")
		return nil, nil, false
	}
	reveal, err := q.GetSimultaneousReveal(r.Context(), revealID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "reveal not found")
		return nil, nil, false
	}
	player, ok := requirePlayerInGame(w, r, q, reveal.GameID)
	if !ok {
		return nil, nil, false
	}
	return &reveal, player, true
}

// ── revealCeilAverage ─────────────────────────────────────────────────────────

// revealCeilAverage returns ceil(average of submitted faces).
// Entries with nil face (not yet submitted) are excluded.
func revealCeilAverage(entries []dbgen.SimultaneousRevealEntry) int16 {
	faces := make([]int16, 0, len(entries))
	for _, e := range entries {
		if e.Face != nil {
			faces = append(faces, *e.Face)
		}
	}
	return gamepkg.CeilAverage(faces)
}
