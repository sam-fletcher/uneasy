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

// ListScenePosts handles GET /api/tables/{id}/rows/{row}/posts.
//
// Returns posts for a scene thread. Supports ?plan_id=X to filter by plan,
// and ?after=Y for catch-up on WebSocket reconnect.
func ListScenePosts(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}
		rowNum, err := strconv.ParseInt(chi.URLParam(r, "row"), 10, 16)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid row number")
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
			posts, err := q.ListScenePostsAfter(ctx, dbgen.ListScenePostsAfterParams{
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

		row := int16(rowNum)

		// Filter by plan_id if provided, otherwise open scene (plan_id IS NULL).
		if pidStr := r.URL.Query().Get("plan_id"); pidStr != "" {
			pid, err := strconv.ParseInt(pidStr, 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid plan_id")
				return
			}
			posts, err := q.ListScenePostsByRowAndPlan(ctx, dbgen.ListScenePostsByRowAndPlanParams{
				GameID:    gameID,
				RowNumber: &row,
				PlanID:    &pid,
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not load posts")
				return
			}
			respond(w, http.StatusOK, map[string]any{"posts": posts})
			return
		}

		// Open scene (no plan filter).
		posts, err := q.ListScenePostsByRowOpenScene(ctx, dbgen.ListScenePostsByRowOpenSceneParams{
			GameID:    gameID,
			RowNumber: &row,
		})
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
func CreateScenePost(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}
		rowNum, err := strconv.ParseInt(chi.URLParam(r, "row"), 10, 16)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid row number")
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

		post, err := q.CreateScenePost(ctx, dbgen.CreateScenePostParams{
			GameID:    gameID,
			RowNumber: &row,
			PlanID:    body.PlanID,
			AuthorID:  player.ID,
			Body:      body.Body,
		})
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
