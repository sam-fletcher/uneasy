package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// tonesLocked reports whether tone edits are locked. Tones are
// reference material throughout lobby + prologue and lock once the
// main event begins.
func tonesLocked(phase model.GamePhase) bool {
	switch phase {
	case model.PhaseLobby, model.PhasePrologue:
		return false
	case model.PhaseMainEvent, model.PhaseShakeUp, model.PhaseEnded:
		return true
	}
	return true
}

// tonesUnlockedPhases inverts tonesLocked into the phase list
// UpdateToneTopicStatusScoped gates its write on. Derived rather than
// written out again so the lock rule keeps a single home, in tonesLocked.
func tonesUnlockedPhases() []string {
	all := model.AllGamePhases()
	out := make([]string, 0, len(all))
	for _, p := range all {
		if !tonesLocked(p) {
			out = append(out, string(p))
		}
	}
	return out
}

// ListToneTopics handles GET /api/tables/{id}/tone.
func ListToneTopics(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		topics, err := s.Q.ListToneTopics(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load topics", err)
			return
		}

		respond(w, http.StatusOK, map[string]any{"topics": topics})
	}
}

// UpdateToneTopic handles PUT /api/tables/{id}/tone/{topicId}.
func UpdateToneTopic(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		topicID, err := strconv.ParseInt(chi.URLParam(r, "topicId"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid topic id")
			return
		}

		var body struct {
			Status string `json:"status"`
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		status := model.ToneTopicStatus(body.Status)
		switch status {
		case model.ToneDefault, model.ToneInclude, model.ToneAvoidDetail, model.ToneNever:
			// valid
		default:
			respondErr(w, http.StatusBadRequest, "invalid status: must be default, include, avoid_detail, or never")
			return
		}

		ctx := r.Context()

		// Ownership, phase gate, and write in one round trip. Players cycle a
		// tile through four statuses by tapping it, so this endpoint is hit in
		// bursts and every saved hop shows up in the tile's responsiveness.
		res, err := s.Q.UpdateToneTopicStatusScoped(ctx, dbgen.UpdateToneTopicStatusScopedParams{
			ID:            topicID,
			GameID:        gameID,
			Status:        status,
			AllowedPhases: tonesUnlockedPhases(),
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			respondErr(w, http.StatusNotFound, "topic not found")
			return
		case err != nil:
			respondInternalErr(w, r, "could not update topic", err)
			return
		}
		if res.GameID != gameID {
			respondErr(w, http.StatusForbidden, "topic does not belong to this game")
			return
		}
		// The topic exists and is ours, so the phase gate is the only guard
		// left that could have held the write back.
		if !res.DidUpdate {
			respondErr(w, http.StatusConflict, "tones are locked once the main event begins")
			return
		}

		// Broadcast the update.
		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventToneUpdated, model.ToneUpdatedPayload{
				TopicID: topicID,
				Topic:   res.Topic,
				Status:  status,
			})
		}

		respond(w, http.StatusOK, map[string]any{"topic_id": topicID, "status": status})
	}
}

// AddToneTopic handles POST /api/tables/{id}/tone.
func AddToneTopic(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		var body struct {
			Topic string `json:"topic"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		topicText, ok := textField(w, "topic", body.Topic, maxToneTopicLen)
		if !ok {
			return
		}
		body.Topic = topicText
		if body.Topic == "" {
			respondErr(w, http.StatusBadRequest, "topic is required")
			return
		}

		ctx := r.Context()
		game, err := s.Q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if tonesLocked(game.Phase) {
			respondErr(w, http.StatusConflict, "tones are locked once the main event begins")
			return
		}

		topic, err := s.Q.CreateToneTopic(ctx, dbgen.CreateToneTopicParams{
			GameID: gameID,
			Topic:  body.Topic,
			Status: model.ToneDefault,
		})
		if err != nil {
			respondInternalErr(w, r, "could not add topic", err)
			return
		}

		// Broadcast the new topic.
		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventToneUpdated, model.ToneUpdatedPayload{
				TopicID: topic.ID,
				Topic:   topic.Topic,
				Status:  topic.Status,
			})
		}

		respond(w, http.StatusCreated, map[string]any{"topic": topic})
	}
}
