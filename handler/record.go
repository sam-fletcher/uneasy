package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// RecordRow is the per-row shape returned by GetFullRecord.
type RecordRow struct {
	RowNumber int16              `json:"row_number"`
	Entries   []dbgen.SceneEntry `json:"entries"`
	Plans     []dbgen.Plan       `json:"plans"`
}

// GetFullRecord handles GET /api/tables/:id/record.
//
// Returns all 13 public-record rows with their scene entries and plans,
// grouped by row number. Plans will be empty until Phase 2f; the shape
// is stable now so the frontend can render the timeline.
func GetFullRecord(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		ctx := r.Context()

		rows, err := s.Q.ListPublicRecordRows(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load public record", err)
			return
		}

		entries, err := s.Q.ListSceneEntries(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load scene entries", err)
			return
		}

		plans, err := s.Q.ListPlansByGame(ctx, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load plans", err)
			return
		}

		// Group entries and plans by row number. Pre-allocate empty slices
		// so the JSON output is [] rather than null.
		entryByRow := make(map[int16][]dbgen.SceneEntry)
		for _, e := range entries {
			entryByRow[e.RowNumber] = append(entryByRow[e.RowNumber], e)
		}

		planByRow := make(map[int16][]dbgen.Plan)
		for _, p := range plans {
			// Plans awaiting a delay reveal have no row yet — skip them
			// here; the row_state kind 'await_delay_reveal' surfaces them
			// in the play area instead.
			if p.RowNumber == nil {
				continue
			}
			planByRow[*p.RowNumber] = append(planByRow[*p.RowNumber], p)
		}

		result := make([]RecordRow, len(rows))
		for i, row := range rows {
			e := entryByRow[row.RowNumber]
			if e == nil {
				e = []dbgen.SceneEntry{}
			}
			p := planByRow[row.RowNumber]
			if p == nil {
				p = []dbgen.Plan{}
			}
			result[i] = RecordRow{
				RowNumber: row.RowNumber,
				Entries:   e,
				Plans:     p,
			}
		}

		respond(w, http.StatusOK, map[string]any{"rows": result})
	}
}

// CreateSceneEntry handles POST /api/tables/:id/rows/:row/summary.
//
// Adds a summary line to the public record for the given row and broadcasts
// the new entry to all connected clients.
func CreateSceneEntry(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		rowNum, err := strconv.ParseInt(chi.URLParam(r, "row"), 10, 16)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid row number")
			return
		}

		var body struct {
			Body string `json:"body"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Body = strings.TrimSpace(body.Body)
		if body.Body == "" {
			respondErr(w, http.StatusBadRequest, "body is required")
			return
		}

		ctx := r.Context()
		row := int16(rowNum)

		entry, err := s.Q.CreateSceneEntry(ctx, dbgen.CreateSceneEntryParams{
			GameID:    gameID,
			RowNumber: row,
			AuthorID:  player.ID,
			Body:      body.Body,
		})
		if err != nil {
			respondInternalErr(w, r, "could not save entry", err)
			return
		}

		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventSceneEntryCreated, model.SceneEntryCreatedPayload{Entry: entry})
		}

		respond(w, http.StatusCreated, map[string]any{"entry": entry})
	}
}
