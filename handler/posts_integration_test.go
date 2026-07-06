//go:build integration

// handler/posts_integration_test.go — Chat Overhaul Phase 1 coverage: the
// windowed post-listing modes, the read-marker endpoint's monotonicity, the
// anchor-resolution endpoint, and scene_id stamping on player posts.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// postsHarness wires the real posts/scenes/read-marker routes with one
// seeded session per player.
type postsHarness struct {
	t       *testing.T
	pool    *pgxpool.Pool
	q       *dbgen.Queries
	manager *hub.Manager
	tg      testGame
	router  http.Handler
	tokens  []string
}

func newPostsHarness(t *testing.T, n int) *postsHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, n)
	store := db.NewStore(pool)
	manager := hub.NewManager()

	tokens := make([]string, n)
	for i, p := range tg.Players {
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
			Token: tok, AccountID: p.AccountID,
		})
		require.NoError(t, err)
		tokens[i] = tok
	}

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Get("/api/tables/{id}/posts", ListGamePosts(store))
	r.Post("/api/tables/{id}/posts", CreatePlayerPost(store, manager))
	r.Get("/api/tables/{id}/posts/anchor", GetPostAnchor(store))
	r.Put("/api/tables/{id}/read-marker", UpdateReadMarker(store))
	r.Post("/api/tables/{id}/scenes", CreateScene(store, manager))
	r.Post("/api/tables/{id}/end-scene", EndScene(store, manager))

	return &postsHarness{
		t: t, pool: pool, q: q, manager: manager,
		tg: tg, router: r, tokens: tokens,
	}
}

func (h *postsHarness) do(method string, playerIdx int, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func (h *postsHarness) tablePath(suffix string) string {
	return "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + suffix
}

// createSystemPosts inserts n bare system posts directly (bypassing HTTP) so
// window-mode tests can build up a large, cheap post history.
func (h *postsHarness) createSystemPosts(n int) []dbgen.ScenePost {
	h.t.Helper()
	ctx := context.Background()
	posts := make([]dbgen.ScenePost, 0, n)
	for i := 0; i < n; i++ {
		code := "test.post"
		p, err := h.q.CreateSystemPost(ctx, dbgen.CreateSystemPostParams{
			GameID:     h.tg.Game.ID,
			Body:       fmt.Sprintf("post %d", i),
			Severity:   model.SeverityDefault,
			SystemCode: &code,
		})
		require.NoError(h.t, err)
		posts = append(posts, p)
	}
	return posts
}

func postIDsOf(posts []dbgen.ScenePost) []int64 {
	ids := make([]int64, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}
	return ids
}

// ── ListGamePosts modes ──────────────────────────────────────────────────────

func TestListGamePosts_DefaultWindow_NoUnread(t *testing.T) {
	h := newPostsHarness(t, 2)
	posts := h.createSystemPosts(10)

	code, out := h.do("GET", 0, h.tablePath("/posts"), nil)
	require.Equal(t, http.StatusOK, code)

	got := out["posts"].([]any)
	require.Len(t, got, 10)
	require.Equal(t, float64(0), out["last_read_post_id"])
	require.Equal(t, false, out["has_more_before"])
	require.Equal(t, false, out["has_more_after"])
	require.Equal(t, float64(posts[0].ID), got[0].(map[string]any)["id"])
}

func TestListGamePosts_AfterMode(t *testing.T) {
	h := newPostsHarness(t, 2)
	posts := h.createSystemPosts(5)
	cursor := posts[2].ID

	code, out := h.do("GET", 0, h.tablePath("/posts?after="+strconv.FormatInt(cursor, 10)), nil)
	require.Equal(t, http.StatusOK, code)

	got := out["posts"].([]any)
	require.Len(t, got, 2, "expected the 2 posts strictly after the cursor")
	require.Equal(t, float64(posts[3].ID), got[0].(map[string]any)["id"])
	require.Equal(t, float64(posts[4].ID), got[1].(map[string]any)["id"])
	require.Equal(t, false, out["has_more_after"])
}

func TestListGamePosts_AfterMode_TruncatedAtCap(t *testing.T) {
	h := newPostsHarness(t, 2)
	posts := h.createSystemPosts(catchUpCap + 10)
	cursor := posts[0].ID

	code, out := h.do("GET", 0, h.tablePath("/posts?after="+strconv.FormatInt(cursor, 10)), nil)
	require.Equal(t, http.StatusOK, code)

	got := out["posts"].([]any)
	require.Len(t, got, catchUpCap, "catch-up must stop at the cap")
	require.Equal(t, float64(posts[1].ID), got[0].(map[string]any)["id"])
	require.Equal(t, true, out["has_more_after"],
		"truncation must be signalled so the client re-windows")
}

func TestListGamePosts_BeforeMode(t *testing.T) {
	h := newPostsHarness(t, 2)
	posts := h.createSystemPosts(10)
	cursor := posts[7].ID

	code, out := h.do("GET", 0, h.tablePath(fmt.Sprintf("/posts?before=%d&limit=3", cursor)), nil)
	require.Equal(t, http.StatusOK, code)

	got := out["posts"].([]any)
	require.Len(t, got, 3, "expected the 3 newest posts strictly before the cursor")
	// Ascending order: posts[4], posts[5], posts[6].
	require.Equal(t, float64(posts[4].ID), got[0].(map[string]any)["id"])
	require.Equal(t, float64(posts[5].ID), got[1].(map[string]any)["id"])
	require.Equal(t, float64(posts[6].ID), got[2].(map[string]any)["id"])
	require.Equal(t, true, out["has_more_before"], "posts 0-3 remain before the page")
	require.Equal(t, true, out["has_more_after"], "posts 7-9 remain after the page")
}

func TestListGamePosts_AroundMode(t *testing.T) {
	h := newPostsHarness(t, 2)
	posts := h.createSystemPosts(10)
	anchor := posts[5].ID

	code, out := h.do("GET", 0, h.tablePath(fmt.Sprintf("/posts?around=%d&limit=4", anchor)), nil)
	require.Equal(t, http.StatusOK, code)

	got := out["posts"].([]any)
	// half=2: 2 posts <= anchor (posts[4], posts[5]) + 2 posts > anchor (posts[6], posts[7]).
	require.Len(t, got, 4)
	ids := make([]float64, len(got))
	for i, p := range got {
		ids[i] = p.(map[string]any)["id"].(float64)
	}
	require.Equal(t, []float64{
		float64(posts[4].ID), float64(posts[5].ID), float64(posts[6].ID), float64(posts[7].ID),
	}, ids)
	require.Equal(t, true, out["has_more_before"])
	require.Equal(t, true, out["has_more_after"])
}

// TestBuildInitialWindow_ExtendsForUnreadBeforeWindow exercises
// buildInitialWindow directly (white-box) to check the context-extension
// branch: the base-100 window doesn't cover the reader's marker, so the
// window should extend back to include 30 posts of read context plus every
// unread post.
func TestBuildInitialWindow_ExtendsForUnreadBeforeWindow(t *testing.T) {
	h := newPostsHarness(t, 2)
	// 150 posts total: base-100 covers only the newest 100, so a marker
	// sitting at post 20 (well before the base window's start) must trigger
	// the extension branch.
	posts := h.createSystemPosts(150)
	lastRead := posts[19].ID

	got, err := buildInitialWindow(context.Background(), h.q, h.tg.Game.ID, lastRead)
	require.NoError(t, err)

	// Expect: 30 posts of context before lastRead (posts[-30:20] = index
	// -10..19, but only 19 exist before index 19, i.e. posts[0..19]) plus
	// every post after lastRead (posts[20..149]).
	// Context is capped by how many actually precede lastRead.
	wantContextStart := 19 - initialWindowContext
	if wantContextStart < 0 {
		wantContextStart = 0
	}
	wantIDs := postIDsOf(posts[wantContextStart:])
	gotIDs := postIDsOf(got)
	require.Equal(t, wantIDs, gotIDs)
}

// TestBuildInitialWindow_OverflowFallsBackToNewestCap checks the cap branch:
// when the unread span alone would exceed the cap, the window gives up on
// full context and returns just the newest initialWindowCap posts.
func TestBuildInitialWindow_OverflowFallsBackToNewestCap(t *testing.T) {
	h := newPostsHarness(t, 2)
	total := initialWindowCap + 20
	posts := h.createSystemPosts(total)
	// Never read anything — every post is unread, far exceeding the cap.
	lastRead := int64(0)

	got, err := buildInitialWindow(context.Background(), h.q, h.tg.Game.ID, lastRead)
	require.NoError(t, err)

	require.Len(t, got, initialWindowCap)
	wantIDs := postIDsOf(posts[total-initialWindowCap:])
	gotIDs := postIDsOf(got)
	require.Equal(t, wantIDs, gotIDs)
}

// ── Read marker ──────────────────────────────────────────────────────────────

func TestUpdateReadMarker_MonotonicAndClamped(t *testing.T) {
	h := newPostsHarness(t, 2)
	posts := h.createSystemPosts(5)
	maxID := posts[4].ID

	// Advance to post 3.
	code, out := h.do("PUT", 0, h.tablePath("/read-marker"), map[string]any{
		"last_read_post_id": posts[2].ID,
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, float64(posts[2].ID), out["last_read_post_id"])

	// Attempt to move backwards → stays at post 3 (monotonic).
	code, out = h.do("PUT", 0, h.tablePath("/read-marker"), map[string]any{
		"last_read_post_id": posts[0].ID,
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, float64(posts[2].ID), out["last_read_post_id"], "marker must not move backwards")

	// Attempt to claim reading posts that don't exist yet → clamped to maxID.
	code, out = h.do("PUT", 0, h.tablePath("/read-marker"), map[string]any{
		"last_read_post_id": maxID + 1000,
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, float64(maxID), out["last_read_post_id"], "marker must clamp to the game's newest post id")

	stored, err := h.q.GetPlayerByID(context.Background(), h.tg.Players[0].ID)
	require.NoError(t, err)
	require.Equal(t, maxID, stored.LastReadPostID)
}

// ── Anchor resolution ────────────────────────────────────────────────────────

func TestGetPostAnchor_BySceneID(t *testing.T) {
	h := newPostsHarness(t, 2)

	code, sceneOut := h.do("POST", 0, h.tablePath("/scenes"), map[string]any{
		"location_custom": "The Mill",
		"time_elapsed":    "moments",
	})
	require.Equal(t, http.StatusCreated, code, "%v", sceneOut)
	sceneMap := sceneOut["scene"].(map[string]any)
	sceneID := int64(sceneMap["id"].(float64))

	code, out := h.do("GET", 0, h.tablePath(fmt.Sprintf("/posts/anchor?code=scene.started&scene_id=%d", sceneID)), nil)
	require.Equal(t, http.StatusOK, code)
	require.NotZero(t, out["post_id"])

	post, err := h.q.GetScenePostByID(context.Background(), int64(out["post_id"].(float64)))
	require.NoError(t, err)
	require.Equal(t, "scene.started", *post.SystemCode)
	require.NotNil(t, post.SceneID)
	require.Equal(t, sceneID, *post.SceneID)
}

func TestGetPostAnchor_NotFound(t *testing.T) {
	h := newPostsHarness(t, 2)

	code, _ := h.do("GET", 0, h.tablePath("/posts/anchor?code=scene.started&scene_id=999999"), nil)
	require.Equal(t, http.StatusNotFound, code)
}

func TestGetPostAnchor_RequiresOneSelector(t *testing.T) {
	h := newPostsHarness(t, 2)

	code, _ := h.do("GET", 0, h.tablePath("/posts/anchor?code=scene.started"), nil)
	require.Equal(t, http.StatusBadRequest, code)
}

// ── scene_id stamping on player posts ────────────────────────────────────────

func TestCreatePlayerPost_StampsSceneIDWhileSceneActive(t *testing.T) {
	h := newPostsHarness(t, 2)

	// No scene active yet → post has nil scene_id.
	code, out := h.do("POST", 0, h.tablePath("/posts"), map[string]any{"body": "before any scene"})
	require.Equal(t, http.StatusCreated, code)
	post := out["post"].(map[string]any)
	require.Nil(t, post["scene_id"])

	_, sceneOut := h.do("POST", 0, h.tablePath("/scenes"), map[string]any{
		"location_custom": "The Mill",
		"time_elapsed":    "moments",
	})
	sceneID := int64(sceneOut["scene"].(map[string]any)["id"].(float64))

	// Table-talk (no speaking_as) during the scene → still stamped, per the
	// single-chronology decision.
	code, out = h.do("POST", 0, h.tablePath("/posts"), map[string]any{"body": "table talk"})
	require.Equal(t, http.StatusCreated, code)
	post = out["post"].(map[string]any)
	require.Equal(t, float64(sceneID), post["scene_id"])

	code, out = h.do("POST", 0, h.tablePath("/end-scene"), nil)
	require.Equal(t, http.StatusOK, code, "%v", out)

	// After the scene ends → nil scene_id again.
	code, out = h.do("POST", 0, h.tablePath("/posts"), map[string]any{"body": "after the scene"})
	require.Equal(t, http.StatusCreated, code)
	post = out["post"].(map[string]any)
	require.Nil(t, post["scene_id"])
}
