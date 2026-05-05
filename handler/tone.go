package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

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

// ListToneTopics handles GET /api/tables/{id}/tone.
func ListToneTopics(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}

		topics, err := q.ListToneTopics(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load topics")
			return
		}

		respond(w, http.StatusOK, map[string]any{"topics": topics})
	}
}

// UpdateToneTopic handles PUT /api/tables/{id}/tone/{topicId}.
func UpdateToneTopic(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
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

		game, err := q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if tonesLocked(game.Phase) {
			respondErr(w, http.StatusConflict, "tones are locked once the main event begins")
			return
		}

		// Verify the topic belongs to this game.
		topic, err := q.GetToneTopic(ctx, topicID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "topic not found")
			return
		}
		if topic.GameID != gameID {
			respondErr(w, http.StatusForbidden, "topic does not belong to this game")
			return
		}

		if err := q.UpdateToneTopicStatus(ctx, dbgen.UpdateToneTopicStatusParams{
			ID:     topicID,
			Status: status,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update topic")
			return
		}

		// Broadcast the update.
		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventToneUpdated, model.ToneUpdatedPayload{
				TopicID: topicID,
				Topic:   topic.Topic,
				Status:  status,
			})
		}

		respond(w, http.StatusOK, map[string]any{"topic_id": topicID, "status": status})
	}
}

// AddToneTopic handles POST /api/tables/{id}/tone.
func AddToneTopic(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
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
		body.Topic = strings.TrimSpace(body.Topic)
		if body.Topic == "" {
			respondErr(w, http.StatusBadRequest, "topic is required")
			return
		}

		ctx := r.Context()
		game, err := q.GetGameByID(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "table not found")
			return
		}
		if tonesLocked(game.Phase) {
			respondErr(w, http.StatusConflict, "tones are locked once the main event begins")
			return
		}

		topic, err := q.CreateToneTopic(ctx, dbgen.CreateToneTopicParams{
			GameID: gameID,
			Topic:  body.Topic,
			Status: model.ToneDefault,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not add topic")
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
