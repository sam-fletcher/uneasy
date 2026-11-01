//go:build integration

// handler/focus_pass_log_integration_test.go — the main-event focus pass writes a
// log line (scene.focus_passed), anchored to the current row (rows 1–13; focus
// only moves in the main event). When the rotation passes over players with no
// peers left, the line names them.

package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// focusPassRouter mounts the two routes that move the focus marker.
func focusPassRouter(t *testing.T, pool *pgxpool.Pool, q *dbgen.Queries) http.Handler {
	t.Helper()
	store := db.NewStore(pool)
	manager := hub.NewManager()
	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/pass-focus", PassFocus(store, manager))
	r.Post("/api/tables/{id}/refresh-assets", RefreshAssets(store, manager))
	return r
}

func focusPassSetup(t *testing.T) (*dbgen.Queries, testGame, http.Handler, string) {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 2)
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID: tg.Game.ID, FocusPlayerID: &tg.Players[0].ID,
	}))
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: tok, AccountID: tg.Players[0].AccountID})
	require.NoError(t, err)
	return q, tg, focusPassRouter(t, pool, q), tok
}

func assertFocusPassPosted(t *testing.T, q *dbgen.Queries, tg testGame) {
	t.Helper()
	ctx := context.Background()
	posts := postsWithCode(t, q, tg.Game.ID, "scene.focus_passed")
	require.Len(t, posts, 1)
	assert.Equal(t, fmt.Sprintf("%s passes the focus to %s.",
		playerDisplayName(ctx, q, tg.Players[0].ID), playerDisplayName(ctx, q, tg.Players[1].ID)), posts[0].Body)
	assert.Equal(t, model.SeverityMinor, posts[0].Severity)
}

func TestFocusPassLog_PassFocus_Posts(t *testing.T) {
	q, tg, router, tok := focusPassSetup(t)

	req := httptest.NewRequest("POST", "/api/tables/"+strconv.FormatInt(tg.Game.ID, 10)+"/pass-focus", nil)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusOK, rec.Code, "pass-focus: %s", rec.Body.String())

	assertFocusPassPosted(t, q, tg)
}

// Players with no peers left are passed over; the line says who, so a reader
// does not mistake a skipped turn for a missed one. Two skipped names read as
// "X and Y".
func TestFocusPassLog_PassFocus_NamesSkippedPlayers(t *testing.T) {
	cases := []struct {
		name    string
		players int
		skipped []int
		to      int
		want    string
	}{
		{"one skipped", 3, []int{1}, 2, "%[1]s passes the focus to %[2]s, skipping %[3]s (no peers left)."},
		{"two skipped", 4, []int{1, 2}, 3, "%[1]s passes the focus to %[2]s, skipping %[3]s (no peers left)."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pool := openTestDB(t)
			q := dbgen.New(pool)
			ctx := context.Background()
			tg := newTestGame(t, q, c.players)
			require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
				ID: tg.Game.ID, FocusPlayerID: &tg.Players[0].ID,
			}))
			names := make([]string, 0, len(c.skipped))
			for _, i := range c.skipped {
				destroyPlayerAssets(t, q, tg.Players[i].ID)
				names = append(names, playerDisplayName(ctx, q, tg.Players[i].ID))
			}
			tok, err := db.NewCookieToken()
			require.NoError(t, err)
			_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: tok, AccountID: tg.Players[0].AccountID})
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/tables/"+strconv.FormatInt(tg.Game.ID, 10)+"/pass-focus", nil)
			req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
			rec := httptest.NewRecorder()
			focusPassRouter(t, pool, q).ServeHTTP(rec, req)
			require.Equalf(t, http.StatusOK, rec.Code, "pass-focus: %s", rec.Body.String())

			posts := postsWithCode(t, q, tg.Game.ID, "scene.focus_passed")
			require.Len(t, posts, 1)
			assert.Equal(t, fmt.Sprintf(c.want,
				playerDisplayName(ctx, q, tg.Players[0].ID),
				playerDisplayName(ctx, q, tg.Players[c.to].ID),
				joinWithAnd(names)), posts[0].Body)
			if len(names) == 2 {
				assert.Contains(t, posts[0].Body, names[0]+" and "+names[1])
			}
		})
	}
}

// The automatic pass after a turn's action (autoPassFocus) is the normal path;
// PassFocus is only the recovery endpoint, so both must post.
func TestFocusPassLog_AutoPassFocus_Posts(t *testing.T) {
	q, tg, router, tok := focusPassSetup(t)

	req := httptest.NewRequest("POST", "/api/tables/"+strconv.FormatInt(tg.Game.ID, 10)+"/refresh-assets",
		strings.NewReader(`{"asset_ids":[]}`))
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusOK, rec.Code, "refresh-assets: %s", rec.Body.String())

	assertFocusPassPosted(t, q, tg)
}
