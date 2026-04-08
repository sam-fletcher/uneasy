package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/db"
	"uneasy/hub"
	"uneasy/model"
	appMiddleware "uneasy/middleware"
)

// ListScenePosts handles GET /api/tables/{id}/rows/{row}/posts.
//
// Returns posts for a scene thread. Supports ?plan_id=X to filter by plan,
// and ?after=Y for catch-up on WebSocket reconnect.
func ListScenePosts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid table id")
			return
		}
		rowNum, err := strconv.ParseInt(chi.URLParam(r, "row"), 10, 16)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid row number")
			return
		}

		player := appMiddleware.PlayerFromContext(r.Context())
		if player == nil || player.GameID != gameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
			return
		}

		ctx := r.Context()

		// Check for ?after=<id> catch-up mode.
		if afterStr := r.URL.Query().Get("after"); afterStr != "" {
			afterID, err := strconv.ParseInt(afterStr, 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid after id")
				return
			}
			posts, err := db.ListScenePostsAfter(ctx, pool, gameID, afterID)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not load posts")
				return
			}
			respond(w, http.StatusOK, map[string]any{"posts": posts})
			return
		}

		// Filter by plan_id if provided, otherwise open scene (plan_id IS NULL).
		var planID *int64
		if pidStr := r.URL.Query().Get("plan_id"); pidStr != "" {
			pid, err := strconv.ParseInt(pidStr, 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid plan_id")
				return
			}
			planID = &pid
		}

		row := int16(rowNum)
		posts, err := db.ListScenePostsByRow(ctx, pool, gameID, row, planID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load posts")
			return
		}

		respond(w, http.StatusOK, map[string]any{"posts": posts})
	}
}

// CreateScenePost handles POST /api/tables/{id}/rows/{row}/posts.
//
// Inserts a scene post and broadcasts it to all connected clients.
func CreateScenePost(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid table id")
			return
		}
		rowNum, err := strconv.ParseInt(chi.URLParam(r, "row"), 10, 16)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid row number")
			return
		}

		player := appMiddleware.PlayerFromContext(r.Context())
		if player == nil || player.GameID != gameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
			return
		}

		var body struct {
			Body   string `json:"body"`
			PlanID *int64 `json:"plan_id"`
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

		ctx := r.Context()
		row := int16(rowNum)

		post, err := db.CreateScenePost(ctx, pool, gameID, &row, body.PlanID, player.ID, body.Body)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save post")
			return
		}

		// Broadcast to all connected WebSocket clients for this table.
		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventScenePostCreated, model.ScenePostCreatedPayload{Post: post})
		}

		respond(w, http.StatusCreated, map[string]any{"post": post})
	}
}
