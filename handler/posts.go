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

// ListPosts handles GET /api/tables/{id}/posts.
//
// Returns posts in chronological order. Supports ?after=<post_id> for
// catch-up on WebSocket reconnect.
func ListPosts(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid table id")
			return
		}

		// Require membership.
		player := appMiddleware.PlayerFromContext(r.Context())
		if player == nil || player.GameID != gameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
			return
		}

		ctx := r.Context()
		var posts []model.Post

		if afterStr := r.URL.Query().Get("after"); afterStr != "" {
			afterID, err := strconv.ParseInt(afterStr, 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid after id")
				return
			}
			posts, err = db.ListPostsAfter(ctx, pool, gameID, afterID)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not load posts")
				return
			}
		} else {
			posts, err = db.ListPosts(ctx, pool, gameID)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not load posts")
				return
			}
		}

		respond(w, http.StatusOK, map[string]any{"posts": posts})
	}
}

// CreatePost handles POST /api/tables/{id}/posts.
//
// Inserts a post and broadcasts a post.created event to all connected clients.
func CreatePost(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid table id")
			return
		}

		player := appMiddleware.PlayerFromContext(r.Context())
		if player == nil || player.GameID != gameID {
			respondErr(w, http.StatusForbidden, "not a member of this table")
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

		ctx := r.Context()

		post, err := db.CreatePost(ctx, pool, gameID, player.ID, body.Body)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save post")
			return
		}

		// Broadcast to all connected WebSocket clients for this table.
		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventPostCreated, model.PostCreatedPayload{Post: post})
		}

		respond(w, http.StatusCreated, map[string]any{"post": post})
	}
}
