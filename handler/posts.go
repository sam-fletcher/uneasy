package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// Windowed feed tuning (Chat Overhaul Phase 1b). See
// adr/CHAT_OVERHAUL_PLAN.md for the initial-window algorithm this backs.
const (
	initialWindowBase    = 100
	initialWindowContext = 30
	initialWindowCap     = 500
	defaultPageLimit     = 50
	maxPageLimit         = 200
	// catchUpCap bounds the ?after= reconnect catch-up. Without it the
	// response grows with everything posted since the client's newest post
	// — for a tab reconnecting after weeks away, that's the entire
	// intervening history in one allocation (measured: 42% of all server
	// allocations under load). When the cap truncates, has_more_after=true
	// tells the client to re-window instead of merging (chatFeed.ts
	// reconnectResync).
	catchUpCap = 500
)

// ListGamePosts handles GET /api/tables/{id}/posts.
//
// Returns the unified game-wide chat feed (player messages, log entries,
// boundary markers). Four modes, chosen by query params:
//
//   - (none): the initial catch-up window (see buildInitialWindow).
//   - ?after=<id>: posts newer than <id>, capped at catchUpCap — reconnect
//     catch-up. has_more_after=true signals truncation.
//   - ?before=<id>&limit=N: a page of older posts ending just before <id>.
//   - ?around=<id>&limit=N: a window centred on <id>, for jump-to-anchor.
//
// Every mode returns the same envelope: {posts, has_more_before,
// has_more_after, last_read_post_id}.
func ListGamePosts(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		query := r.URL.Query()

		switch {
		case query.Get("after") != "":
			afterID, err := strconv.ParseInt(query.Get("after"), 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid after id")
				return
			}
			posts, err := s.Q.ListGamePostsAfterLimited(ctx, dbgen.ListGamePostsAfterLimitedParams{
				GameID: gameID, ID: afterID, Limit: catchUpCap,
			})
			if err != nil {
				respondInternalErr(w, r, "could not load posts", err)
				return
			}
			writePostWindow(w, r, ctx, s.Q, gameID, player, posts)

		case query.Get("before") != "":
			beforeID, err := strconv.ParseInt(query.Get("before"), 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid before id")
				return
			}
			limit, err := parseLimit(query, defaultPageLimit, maxPageLimit)
			if err != nil {
				respondErr(w, http.StatusBadRequest, err.Error())
				return
			}
			posts, err := s.Q.ListGamePostsBefore(ctx, dbgen.ListGamePostsBeforeParams{
				GameID: gameID, ID: beforeID, Limit: int32(limit),
			})
			if err != nil {
				respondInternalErr(w, r, "could not load posts", err)
				return
			}
			reversePosts(posts)
			writePostWindow(w, r, ctx, s.Q, gameID, player, posts)

		case query.Get("around") != "":
			aroundID, err := strconv.ParseInt(query.Get("around"), 10, 64)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "invalid around id")
				return
			}
			limit, err := parseLimit(query, defaultPageLimit, maxPageLimit)
			if err != nil {
				respondErr(w, http.StatusBadRequest, err.Error())
				return
			}
			half := int32(limit / 2)
			before, err := s.Q.ListGamePostsBefore(ctx, dbgen.ListGamePostsBeforeParams{
				GameID: gameID, ID: aroundID + 1, Limit: half,
			})
			if err != nil {
				respondInternalErr(w, r, "could not load posts", err)
				return
			}
			reversePosts(before)
			after, err := s.Q.ListGamePostsAfterLimited(ctx, dbgen.ListGamePostsAfterLimitedParams{
				GameID: gameID, ID: aroundID, Limit: half,
			})
			if err != nil {
				respondInternalErr(w, r, "could not load posts", err)
				return
			}
			before = append(before, after...)
			writePostWindow(w, r, ctx, s.Q, gameID, player, before)

		default:
			posts, err := buildInitialWindow(ctx, s.Q, gameID, player.LastReadPostID)
			if err != nil {
				respondInternalErr(w, r, "could not load posts", err)
				return
			}
			writePostWindow(w, r, ctx, s.Q, gameID, player, posts)
		}
	}
}

// buildInitialWindow implements the Phase 1b initial-window algorithm:
//
//  1. Base window = the newest initialWindowBase posts.
//  2. If lastRead is older than the base window's oldest post, the unread
//     span isn't fully covered by the base window — extend back to
//     initialWindowContext posts of read context before lastRead.
//  3. If that combined span would exceed initialWindowCap, give up on
//     showing full context and just return the newest initialWindowCap
//     posts (bounds the DOM at the cost of losing the oldest unread).
func buildInitialWindow(ctx context.Context, q *dbgen.Queries, gameID, lastRead int64) ([]dbgen.ScenePost, error) {
	base, err := q.ListGamePostsNewest(ctx, dbgen.ListGamePostsNewestParams{
		GameID: gameID, Limit: initialWindowBase,
	})
	if err != nil {
		return nil, err
	}
	reversePosts(base)
	if len(base) == 0 || lastRead >= base[0].ID {
		return base, nil
	}

	maxUnread := int32(initialWindowCap - initialWindowContext)
	unread, err := q.ListGamePostsAfterLimited(ctx, dbgen.ListGamePostsAfterLimitedParams{
		GameID: gameID, ID: lastRead, Limit: maxUnread + 1,
	})
	if err != nil {
		return nil, err
	}
	if int32(len(unread)) > maxUnread {
		newest, err := q.ListGamePostsNewest(ctx, dbgen.ListGamePostsNewestParams{
			GameID: gameID, Limit: initialWindowCap,
		})
		if err != nil {
			return nil, err
		}
		reversePosts(newest)
		return newest, nil
	}

	readContext, err := q.ListGamePostsBefore(ctx, dbgen.ListGamePostsBeforeParams{
		GameID: gameID, ID: lastRead + 1, Limit: initialWindowContext,
	})
	if err != nil {
		return nil, err
	}
	reversePosts(readContext)
	return append(readContext, unread...), nil
}

// writePostWindow computes the has_more_before/after flags relative to the
// returned posts' id bounds and writes the standard envelope.
func writePostWindow(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	player *dbgen.Player,
	posts []dbgen.ScenePost,
) {
	var hasBefore, hasAfter bool
	if len(posts) > 0 {
		var err error
		hasBefore, err = q.GamePostExistsBefore(ctx, dbgen.GamePostExistsBeforeParams{
			GameID: gameID, ID: posts[0].ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not check post window bounds", err)
			return
		}
		hasAfter, err = q.GamePostExistsAfter(ctx, dbgen.GamePostExistsAfterParams{
			GameID: gameID, ID: posts[len(posts)-1].ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not check post window bounds", err)
			return
		}
	}
	respond(w, http.StatusOK, map[string]any{
		"posts":             posts,
		"has_more_before":   hasBefore,
		"has_more_after":    hasAfter,
		"last_read_post_id": player.LastReadPostID,
	})
}

// reversePosts reverses a post slice in place (newest-first → oldest-first).
func reversePosts(posts []dbgen.ScenePost) {
	for i, j := 0, len(posts)-1; i < j; i, j = i+1, j-1 {
		posts[i], posts[j] = posts[j], posts[i]
	}
}

// parseLimit reads the "limit" query param, defaulting to def and capping
// at maxLimit. Returns an error for a non-positive or non-numeric value.
func parseLimit(query url.Values, def, maxLimit int) (int, error) {
	s := query.Get("limit")
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid limit")
	}
	if n > maxLimit {
		n = maxLimit
	}
	return n, nil
}

// UpdateReadMarker handles PUT /api/tables/{id}/read-marker.
//
// Monotonic and private — see UpdateReadMarker (sqlc) for the
// GREATEST/clamp logic. No WS broadcast: the marker is per-player state,
// not something other players need to see live.
func UpdateReadMarker(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		var body struct {
			LastReadPostID int64 `json:"last_read_post_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.LastReadPostID < 0 {
			respondErr(w, http.StatusBadRequest, "last_read_post_id must be non-negative")
			return
		}
		stored, err := s.Q.UpdateReadMarker(r.Context(), dbgen.UpdateReadMarkerParams{
			RequestedID: body.LastReadPostID,
			GameID:      gameID,
			PlayerID:    player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not update read marker", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{"last_read_post_id": stored})
	}
}

// GetPostAnchor handles GET /api/tables/{id}/posts/anchor.
//
// Resolves a Public Record jump gesture (row/plan/scene) plus a system_code
// to the post id that anchors it, so the client can `around`-fetch it even
// when it isn't in the currently loaded window. Exactly one of row,
// plan_id, scene_id is required alongside code.
func GetPostAnchor(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		query := r.URL.Query()
		code := query.Get("code")
		if code == "" {
			respondErr(w, http.StatusBadRequest, "code is required")
			return
		}
		ctx := r.Context()

		var (
			postID int64
			err    error
		)
		switch {
		case query.Get("row") != "":
			row, perr := strconv.ParseInt(query.Get("row"), 10, 16)
			if perr != nil {
				respondErr(w, http.StatusBadRequest, "invalid row")
				return
			}
			rowNum := int16(row)
			postID, err = s.Q.FindAnchorPostByRow(ctx, dbgen.FindAnchorPostByRowParams{
				GameID: gameID, SystemCode: &code, RowNumber: &rowNum,
			})
		case query.Get("plan_id") != "":
			planID, perr := strconv.ParseInt(query.Get("plan_id"), 10, 64)
			if perr != nil {
				respondErr(w, http.StatusBadRequest, "invalid plan_id")
				return
			}
			postID, err = s.Q.FindAnchorPostByPlan(ctx, dbgen.FindAnchorPostByPlanParams{
				GameID: gameID, SystemCode: &code, PlanID: &planID,
			})
		case query.Get("scene_id") != "":
			sceneID, perr := strconv.ParseInt(query.Get("scene_id"), 10, 64)
			if perr != nil {
				respondErr(w, http.StatusBadRequest, "invalid scene_id")
				return
			}
			postID, err = s.Q.FindAnchorPostByScene(ctx, dbgen.FindAnchorPostBySceneParams{
				GameID: gameID, SystemCode: &code, SceneID: &sceneID,
			})
		default:
			respondErr(w, http.StatusBadRequest, "one of row, plan_id, scene_id is required")
			return
		}
		if err != nil {
			respondErr(w, http.StatusNotFound, "no matching post")
			return
		}
		respond(w, http.StatusOK, map[string]any{"post_id": postID})
	}
}

// CreatePlayerPost handles POST /api/tables/{id}/posts.
//
// Inserts a free-text player message into the game's chat feed and
// broadcasts it. Player messages are not pinned to a row or plan; the
// row_number/plan_id columns exist only for system-emitted entries. If a
// scene is active, the post is stamped with its scene_id — in-character or
// not (single-chronology decision, adr/CHAT_OVERHAUL_PLAN.md Phase 1e).
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
		text, ok := textField(w, "body", body.Body, maxLongTextLen)
		if !ok {
			return
		}
		body.Body = text
		if body.Body == "" {
			respondErr(w, http.StatusBadRequest, "body is required")
			return
		}

		ctx := r.Context()
		scene, err := loadActiveScene(ctx, s.Q, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load active scene", err)
			return
		}

		if status, msg := validateSpeakingAs(ctx, s.Q, gameID, player.ID, body.SpeakingAsAssetID, scene); status != 0 {
			respondErr(w, status, msg)
			return
		}

		var speakingAs *int64
		if body.SpeakingAsAssetID != 0 {
			id := body.SpeakingAsAssetID
			speakingAs = &id
		}
		var sceneID *int64
		if scene != nil {
			id := scene.ID
			sceneID = &id
		}

		post, err := s.Q.CreatePlayerMessage(ctx, dbgen.CreatePlayerMessageParams{
			GameID:            gameID,
			AuthorID:          &player.ID,
			Body:              body.Body,
			RowNumber:         nil,
			PlanID:            nil,
			SceneID:           sceneID,
			SpeakingAsAssetID: speakingAs,
		})
		if err != nil {
			respondInternalErr(w, r, "could not save post", err)
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
// On error, the post is dropped (with a server-side warning) — the chat log is
// best-effort metadata, not load-bearing for game state, and we don't want a
// chat-write failure to roll back the actual transition. The warning exists
// because a fully silent drop hid a real bug for a long time: rowNumber is
// FK-checked against public_record_rows, so any anchor outside rows 1–13
// vanished without trace. Pass row anchors through logRow (system_posts.go).
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
			loggerFromContext(ctx).WarnContext(ctx, "chat log post dropped: system_data not JSON-encodable",
				"system_code", systemCode, "game_id", gameID, "err", err)
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
		loggerFromContext(ctx).WarnContext(ctx, "chat log post dropped: insert failed",
			"system_code", systemCode, "game_id", gameID,
			"row_number", rowNumber, "plan_id", planID, "scene_id", sceneID,
			"err", err)
		return
	}
	if h, ok := manager.Get(gameID); ok {
		h.BroadcastEvent(model.EventScenePostCreated, model.ScenePostCreatedPayload{Post: post})
	}
}
