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
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// ── Laws ──────────────────────────────────────────────────────────────────────

// ListLaws handles GET /api/tables/{id}/laws.
func ListLaws(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		laws, err := s.Q.ListLaws(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, "could not list laws", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"laws": laws})
	}
}

// UpdateLaw handles PATCH /api/laws/{lawId}.
// Body: {"text": "...", "addendum"?: "..."}. The addendum field is optional;
// when omitted the existing value is preserved.
func UpdateLaw(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lawID, err := strconv.ParseInt(chi.URLParam(r, "lawId"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid law id")
			return
		}
		ctx := r.Context()
		law, err := s.Q.GetLawByID(ctx, lawID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "law not found")
			return
		}
		player, ok := requirePlayerInGame(w, r, s.Q, law.GameID)
		if !ok {
			return
		}

		// Authorization: signatory, else origin plan's preparer.
		if !canEditLaw(ctx, s.Q, &law, player.ID) {
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

		updated, err := s.Q.UpdateLawText(ctx, dbgen.UpdateLawTextParams{
			ID:       lawID,
			Text:     body.Text,
			Addendum: addendum,
		})
		if err != nil {
			respondInternalErr(w, "could not update law", err)
			return
		}
		if h, ok := manager.Get(law.GameID); ok {
			h.BroadcastEvent(model.EventLawUpdated, model.LawUpdatedPayload{Law: updated})
		}
		EmitSystemPost(ctx, s.Q, manager, law.GameID, "law.edited",
			model.SeverityTrace,
			fmt.Sprintf("%s edited a law.", playerDisplayName(ctx, s.Q, player.ID)),
			nil, nil, nil,
			map[string]any{"law_id": updated.ID, "editor_id": player.ID})
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
func ListRumors(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		rumors, err := s.Q.ListRumors(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, "could not list rumors", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"rumors": rumors})
	}
}

// UpdateRumor handles PATCH /api/rumors/{rumorId}.
// Body: {"text": "..."}.
func UpdateRumor(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rumorID, err := strconv.ParseInt(chi.URLParam(r, "rumorId"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid rumor id")
			return
		}
		ctx := r.Context()
		rumor, err := s.Q.GetRumorByID(ctx, rumorID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "rumor not found")
			return
		}
		player, ok := requirePlayerInGame(w, r, s.Q, rumor.GameID)
		if !ok {
			return
		}

		if !canEditRumor(ctx, s.Q, &rumor, player.ID) {
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

		updated, err := s.Q.UpdateRumorText(ctx, dbgen.UpdateRumorTextParams{
			ID:   rumorID,
			Text: body.Text,
		})
		if err != nil {
			respondInternalErr(w, "could not update rumor", err)
			return
		}
		if h, ok := manager.Get(rumor.GameID); ok {
			h.BroadcastEvent(model.EventRumorUpdated, model.RumorUpdatedPayload{Rumor: updated})
		}
		EmitSystemPost(ctx, s.Q, manager, rumor.GameID, "rumor.edited",
			model.SeverityTrace,
			fmt.Sprintf("%s edited a rumor.", playerDisplayName(ctx, s.Q, player.ID)),
			nil, nil, nil,
			map[string]any{"rumor_id": updated.ID, "editor_id": player.ID})
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
