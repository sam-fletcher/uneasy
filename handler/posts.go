package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// ListGamePosts handles GET /api/tables/{id}/posts.
//
// Returns the unified game-wide chat feed (player messages, log entries,
// boundary markers) in chronological order. Supports ?after=<id> for
// catch-up on WebSocket reconnect.
func ListGamePosts(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()

		if afterStr := r.URL.Query().Get("after"); afterStr != "" {
			afterID, err := strconv.ParseInt(afterStr, 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid after id")
				return
			}
			posts, err := s.Q.ListGamePostsAfter(ctx, dbgen.ListGamePostsAfterParams{
				GameID: gameID,
				ID:     afterID,
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not load posts")
				return
			}
			respond(w, http.StatusOK, map[string]any{"posts": posts})
			return
		}

		posts, err := s.Q.ListGamePosts(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load posts")
			return
		}
		respond(w, http.StatusOK, map[string]any{"posts": posts})
	}
}

// CreatePlayerPost handles POST /api/tables/{id}/posts.
//
// Inserts a free-text player message into the game's chat feed and
// broadcasts it. Player messages are not pinned to a row or plan; the
// row_number/plan_id columns exist only for system-emitted entries.
func CreatePlayerPost(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		var body struct {
			Body              string `json:"body"`
			SpeakingAsAssetID int64  `json:"speaking_as_asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Body = strings.TrimSpace(body.Body)
		if body.Body == "" {
			respondErr(w, http.StatusBadRequest, "body is required")
			return
		}

		if status, msg := validateSpeakingAs(r.Context(), s.Q, gameID, player.ID, body.SpeakingAsAssetID); status != 0 {
			respondErr(w, status, msg)
			return
		}

		var speakingAs *int64
		if body.SpeakingAsAssetID != 0 {
			id := body.SpeakingAsAssetID
			speakingAs = &id
		}

		post, err := s.Q.CreatePlayerMessage(r.Context(), dbgen.CreatePlayerMessageParams{
			GameID:            gameID,
			AuthorID:          &player.ID,
			Body:              body.Body,
			RowNumber:         nil,
			PlanID:            nil,
			SceneID:           nil,
			SpeakingAsAssetID: speakingAs,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save post")
			return
		}

		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventScenePostCreated, model.ScenePostCreatedPayload{Post: post})
		}
		respond(w, http.StatusCreated, map[string]any{"post": post})
	}
}

// EmitSystemPost inserts a system-authored post into the chat feed and
// broadcasts the resulting post over the table's WebSocket hub. Used by
// transition handlers (row.advanced, phase.changed, plan lifecycle, scene
// ended, etc.) and by future action-log emission points.
//
// severity is the integer scale from model/severity.go — use SeverityBoundary
// for structural anchors the Public Record sidebar jumps to (row.advanced,
// scene.started, plan.prepared), and the lower tiers for log-only events.
//
// rowNumber, planID, and sceneID are optional anchors used by the rail's
// jump-to-anchor gestures; pass nil when not applicable. data is an
// optional JSON-encodable payload stored as JSONB; its schema is determined
// by systemCode.
//
// On error, the post is silently dropped — the chat log is best-effort
// metadata, not load-bearing for game state, and we don't want a chat-write
// failure to roll back the actual transition.
func EmitSystemPost(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	systemCode string,
	severity int32,
	body string,
	rowNumber *int16,
	planID *int64,
	sceneID *int64,
	data any,
) {
	var raw []byte
	if data != nil {
		var err error
		raw, err = json.Marshal(data)
		if err != nil {
			return
		}
	}
	post, err := q.CreateSystemPost(ctx, dbgen.CreateSystemPostParams{
		GameID:     gameID,
		Body:       body,
		RowNumber:  rowNumber,
		PlanID:     planID,
		SceneID:    sceneID,
		Severity:   severity,
		SystemCode: &systemCode,
		SystemData: raw,
	})
	if err != nil {
		return
	}
	if h, ok := manager.Get(gameID); ok {
		h.BroadcastEvent(model.EventScenePostCreated, model.ScenePostCreatedPayload{Post: post})
	}
}
