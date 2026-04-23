package handler

// handler/laws_rumors.go — list and edit laws and rumors.
//
// Laws and rumors are long-form narrative text created by Propose Decree and
// Spread Rumors (plus Host Festivity's spread-rumor path). Once created, the
// authoring player may revise the text as the fiction develops. Anyone at
// the table can read them.
//
// Authorization for edits:
//   - Law:   the signatory (if set), else the origin plan's preparer.
//   - Rumor: the source player (if set), else the origin plan's preparer.
//     Rumors whose source was hidden (SourcePlayerID == nil) fall back to the
//     origin plan's preparer — the hidden author still drafts the text.

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// ── Laws ──────────────────────────────────────────────────────────────────────

// ListLaws handles GET /api/tables/{id}/laws.
func ListLaws(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}
		laws, err := q.ListLaws(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not list laws")
			return
		}
		respond(w, http.StatusOK, map[string]any{"laws": laws})
	}
}

// UpdateLaw handles PATCH /api/laws/{lawId}.
// Body: {"text": "...", "addendum"?: "..."}. The addendum field is optional;
// when omitted the existing value is preserved.
func UpdateLaw(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lawID, err := strconv.ParseInt(chi.URLParam(r, "lawId"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid law id")
			return
		}
		ctx := r.Context()
		law, err := q.GetLawByID(ctx, lawID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "law not found")
			return
		}
		player := appMiddleware.PlayerFromContext(ctx)
		if player == nil || player.GameID != law.GameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
			return
		}

		// Authorization: signatory, else origin plan's preparer.
		if !canEditLaw(ctx, q, &law, player.ID) {
			respondErr(w, http.StatusForbidden, "only the signatory or plan preparer may edit this law")
			return
		}

		var body struct {
			Text     string  `json:"text"`
			Addendum *string `json:"addendum"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Text == "" {
			respondErr(w, http.StatusBadRequest, "text is required")
			return
		}

		// Preserve existing addendum when the field is absent.
		addendum := law.Addendum
		if body.Addendum != nil {
			if *body.Addendum == "" {
				addendum = nil
			} else {
				addendum = body.Addendum
			}
		}

		updated, err := q.UpdateLawText(ctx, dbgen.UpdateLawTextParams{
			ID:       lawID,
			Text:     body.Text,
			Addendum: addendum,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update law")
			return
		}
		if h, ok := manager.Get(law.GameID); ok {
			h.BroadcastEvent(model.EventLawUpdated, model.LawUpdatedPayload{Law: updated})
		}
		respond(w, http.StatusOK, map[string]any{"law": updated})
	}
}

func canEditLaw(ctx context.Context, q *dbgen.Queries, law *dbgen.Law, playerID int64) bool {
	if law.SignatoryID != nil && *law.SignatoryID == playerID {
		return true
	}
	if law.OriginPlanID != nil {
		if plan, err := q.GetPlanByID(ctx, *law.OriginPlanID); err == nil && plan.PreparerID == playerID {
			return true
		}
	}
	return false
}

// ── Rumors ────────────────────────────────────────────────────────────────────

// ListRumors handles GET /api/tables/{id}/rumors.
func ListRumors(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}
		rumors, err := q.ListRumors(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not list rumors")
			return
		}
		respond(w, http.StatusOK, map[string]any{"rumors": rumors})
	}
}

// UpdateRumor handles PATCH /api/rumors/{rumorId}.
// Body: {"text": "..."}.
func UpdateRumor(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rumorID, err := strconv.ParseInt(chi.URLParam(r, "rumorId"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid rumor id")
			return
		}
		ctx := r.Context()
		rumor, err := q.GetRumorByID(ctx, rumorID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "rumor not found")
			return
		}
		player := appMiddleware.PlayerFromContext(ctx)
		if player == nil || player.GameID != rumor.GameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
			return
		}

		if !canEditRumor(ctx, q, &rumor, player.ID) {
			respondErr(w, http.StatusForbidden, "only the source player or plan preparer may edit this rumor")
			return
		}

		var body struct {
			Text string `json:"text"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			respondErr(w, http.StatusBadRequest, "text is required")
			return
		}

		updated, err := q.UpdateRumorText(ctx, dbgen.UpdateRumorTextParams{
			ID:   rumorID,
			Text: body.Text,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update rumor")
			return
		}
		if h, ok := manager.Get(rumor.GameID); ok {
			h.BroadcastEvent(model.EventRumorUpdated, model.RumorUpdatedPayload{Rumor: updated})
		}
		respond(w, http.StatusOK, map[string]any{"rumor": updated})
	}
}

func canEditRumor(ctx context.Context, q *dbgen.Queries, rumor *dbgen.Rumor, playerID int64) bool {
	if rumor.SourcePlayerID != nil && *rumor.SourcePlayerID == playerID {
		return true
	}
	if rumor.OriginPlanID != nil {
		if plan, err := q.GetPlanByID(ctx, *rumor.OriginPlanID); err == nil && plan.PreparerID == playerID {
			return true
		}
	}
	return false
}
