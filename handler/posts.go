package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// ListGamePosts handles GET /api/tables/{id}/posts.
//
// Returns the unified game-wide chat feed (player messages, log entries,
// boundary markers) in chronological order. Supports ?after=<id> for
// catch-up on WebSocket reconnect.
func ListGamePosts(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
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
			posts, err := q.ListGamePostsAfter(ctx, dbgen.ListGamePostsAfterParams{
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

		posts, err := q.ListGamePosts(ctx, gameID)
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
func CreatePlayerPost(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}

		var body struct {
			Body string `json:"body"`
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

		post, err := q.CreatePlayerMessage(r.Context(), dbgen.CreatePlayerMessageParams{
			GameID:    gameID,
			AuthorID:  &player.ID,
			Body:      body.Body,
			RowNumber: nil,
			PlanID:    nil,
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

// EmitBoundary inserts a system-authored boundary marker into the chat feed
// and broadcasts the resulting post over the table's WebSocket hub. Used by
// transition handlers (row.advanced, phase.changed, plan lifecycle, scene
// ended, etc.) to mark phase transitions in the unified chat.
//
// rowNumber and planID are optional context that lets the client render and
// jump-to the boundary; pass nil when not applicable. data is an optional
// JSON-encodable payload stored as JSONB on the row; pass nil for none.
//
// On error, the boundary is silently dropped — boundaries are
// best-effort metadata, not load-bearing for game state, and we don't want
// a chat-write failure to roll back the actual transition.
func EmitBoundary(
	ctx context.Context,
	q *dbgen.Queries,
	manager *hub.Manager,
	gameID int64,
	systemCode string,
	body string,
	rowNumber *int16,
	planID *int64,
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
	post, err := q.CreateBoundaryPost(ctx, dbgen.CreateBoundaryPostParams{
		GameID:     gameID,
		Body:       body,
		RowNumber:  rowNumber,
		PlanID:     planID,
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
