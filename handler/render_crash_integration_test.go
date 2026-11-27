//go:build integration

// handler/render_crash_integration_test.go — POST /api/render-crashes
// (handler/render_crash.go): a session-authed report of a render throw a
// svelte:boundary caught in the browser, written to render_crash_reports and
// rate-limited per account with its own table count.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// renderCrashHarness wires only POST /api/render-crashes, plus a logged-in
// session for each of the two players of a two-player game.
type renderCrashHarness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	tg     testGame
	router http.Handler
	tokens []string // session cookie per player index
}

func newRenderCrashHarness(t *testing.T) *renderCrashHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	store := db.NewStore(pool)

	tokens := make([]string, len(tg.Players))
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
	r.Post("/api/render-crashes", CreateRenderCrashReport(store))

	return &renderCrashHarness{t: t, pool: pool, q: q, tg: tg, router: r, tokens: tokens}
}

// do sends a request as player index `as`, or anonymously when as < 0.
func (h *renderCrashHarness) do(as int, body any) (int, map[string]any) {
	h.t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(h.t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/render-crashes", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if as >= 0 {
		req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[as]})
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

// countReports returns the rows in render_crash_reports, optionally only those
// filed by one account.
func (h *renderCrashHarness) countReports(accountID *int64) int {
	h.t.Helper()
	var n int
	var err error
	if accountID == nil {
		err = h.pool.QueryRow(context.Background(), "SELECT count(*) FROM render_crash_reports").Scan(&n)
	} else {
		err = h.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM render_crash_reports WHERE account_id = $1", *accountID).Scan(&n)
	}
	require.NoError(h.t, err)
	return n
}

// seedReports inserts n reports for an account directly, so the request under
// test is the one that meets the limit.
func (h *renderCrashHarness) seedReports(accountID int64, n int) {
	h.t.Helper()
	for i := 0; i < n; i++ {
		_, err := h.q.InsertRenderCrashReport(context.Background(), dbgen.InsertRenderCrashReportParams{
			AccountID: &accountID, Boundary: "seeded", Message: "prior crash", Route: "/", Build: "b",
		})
		require.NoError(h.t, err)
	}
}

func validReport(gameID int64) map[string]any {
	return map[string]any{
		"boundary": "table phase view (main_event)",
		"message":  "each_key_duplicate",
		"stack":    "Error: each_key_duplicate\n    at main.svelte:12",
		"route":    "/table/7",
		"build":    "1700000000000",
		"game_id":  gameID,
	}
}

func TestCreateRenderCrashReport_WritesThenRateLimited(t *testing.T) {
	h := newRenderCrashHarness(t)
	acctID := h.tg.Players[0].AccountID

	// The first report is written, with every field the client sent.
	code, out := h.do(0, validReport(h.tg.Game.ID))
	require.Equalf(t, http.StatusCreated, code, "report accepted: %v", out)
	assert.Equal(t, 1, h.countReports(&acctID))

	var boundary, message, stack, route, build string
	var gameID, storedAcct *int64
	err := h.pool.QueryRow(context.Background(),
		`SELECT boundary, message, stack, route, build, game_id, account_id
		   FROM render_crash_reports ORDER BY id DESC LIMIT 1`,
	).Scan(&boundary, &message, &stack, &route, &build, &gameID, &storedAcct)
	require.NoError(t, err)
	assert.Equal(t, "table phase view (main_event)", boundary)
	assert.Equal(t, "each_key_duplicate", message)
	assert.Equal(t, "Error: each_key_duplicate\n    at main.svelte:12", stack)
	assert.Equal(t, "/table/7", route)
	assert.Equal(t, "1700000000000", build)
	require.NotNil(t, gameID)
	assert.Equal(t, h.tg.Game.ID, *gameID)
	require.NotNil(t, storedAcct)
	assert.Equal(t, acctID, *storedAcct, "the account comes from the session")

	// Then the account is rate-limited. Top the count up to the limit directly,
	// so the next request through the real handler is the one that meets it: it
	// is 429 and is not written.
	h.seedReports(acctID, renderCrashRateLimit-1)
	require.Equal(t, renderCrashRateLimit, h.countReports(&acctID))
	code, out = h.do(0, validReport(h.tg.Game.ID))
	require.Equalf(t, http.StatusTooManyRequests, code, "eleventh report within the hour rate-limited: %v", out)
	assert.Equal(t, renderCrashRateLimit, h.countReports(&acctID), "rejected report must not be inserted")
}

func TestCreateRenderCrashReport_AllowsUpToTheLimit(t *testing.T) {
	h := newRenderCrashHarness(t)
	acctID := h.tg.Players[0].AccountID

	h.seedReports(acctID, renderCrashRateLimit-1)
	code, out := h.do(0, validReport(h.tg.Game.ID))
	require.Equalf(t, http.StatusCreated, code, "the %dth report (nine prior) accepted: %v", renderCrashRateLimit, out)
	assert.Equal(t, renderCrashRateLimit, h.countReports(&acctID))
}

func TestCreateRenderCrashReport_LimitIsPerAccount(t *testing.T) {
	h := newRenderCrashHarness(t)
	first := h.tg.Players[0].AccountID
	second := h.tg.Players[1].AccountID

	h.seedReports(first, renderCrashRateLimit)
	code, out := h.do(1, validReport(h.tg.Game.ID))
	require.Equalf(t, http.StatusCreated, code, "another player's budget is untouched: %v", out)
	assert.Equal(t, 1, h.countReports(&second))
}

// Feedback and crash reports each have their own budget: a player who has used
// up the feedback limit can still have a crash reported, and the reverse.
func TestCreateRenderCrashReport_NotCountedAgainstFeedback(t *testing.T) {
	h := newRenderCrashHarness(t)
	acctID := h.tg.Players[0].AccountID

	for i := 0; i < feedbackRateLimit; i++ {
		_, err := h.q.InsertFeedbackSubmission(context.Background(), dbgen.InsertFeedbackSubmissionParams{
			Kind: model.FeedbackKindFeedback, AccountID: &acctID, Body: "prior feedback",
		})
		require.NoError(t, err)
	}

	code, out := h.do(0, validReport(h.tg.Game.ID))
	require.Equalf(t, http.StatusCreated, code, "a full feedback budget does not block a crash report: %v", out)
	assert.Equal(t, 1, h.countReports(&acctID))
}

func TestCreateRenderCrashReport_RequiresLogin(t *testing.T) {
	h := newRenderCrashHarness(t)

	code, out := h.do(-1, validReport(h.tg.Game.ID))
	require.Equalf(t, http.StatusUnauthorized, code, "no session cookie: %v", out)
	assert.Zero(t, h.countReports(nil))
}

func TestCreateRenderCrashReport_RequiresBoundaryAndMessage(t *testing.T) {
	h := newRenderCrashHarness(t)

	noBoundary := validReport(h.tg.Game.ID)
	noBoundary["boundary"] = "   "
	code, out := h.do(0, noBoundary)
	require.Equalf(t, http.StatusBadRequest, code, "blank boundary rejected: %v", out)

	noMessage := validReport(h.tg.Game.ID)
	noMessage["message"] = ""
	code, out = h.do(0, noMessage)
	require.Equalf(t, http.StatusBadRequest, code, "empty message rejected: %v", out)

	assert.Zero(t, h.countReports(nil))
}

// gameIDStored returns the game_id of the newest report, or nil when it has none.
func (h *renderCrashHarness) gameIDStored() *int64 {
	h.t.Helper()
	var gameID *int64
	err := h.pool.QueryRow(context.Background(),
		"SELECT game_id FROM render_crash_reports ORDER BY id DESC LIMIT 1").Scan(&gameID)
	require.NoError(h.t, err)
	return gameID
}

// A tab left open after its game was deleted must still have its crash
// recorded: the report is kept, and only the link to the game is dropped.
func TestCreateRenderCrashReport_DeletedGameStoresNull(t *testing.T) {
	h := newRenderCrashHarness(t)
	gameID := h.tg.Game.ID

	_, err := h.q.DeleteGame(context.Background(), gameID)
	require.NoError(t, err)

	code, out := h.do(0, validReport(gameID))
	require.Equalf(t, http.StatusCreated, code, "report for a deleted game is still accepted: %v", out)
	assert.Equal(t, 1, h.countReports(nil))
	assert.Nil(t, h.gameIDStored(), "game_id must be NULL for a game that no longer exists")
}

// A game the account was never seated in is treated the same way: the report
// is kept without the link, so a client cannot attach a crash to a game it is
// not playing in.
func TestCreateRenderCrashReport_UnseatedGameStoresNull(t *testing.T) {
	h := newRenderCrashHarness(t)
	other := newTestGame(t, h.q, 2)
	require.NotEqual(t, h.tg.Game.ID, other.Game.ID)

	code, out := h.do(0, validReport(other.Game.ID))
	require.Equalf(t, http.StatusCreated, code, "report naming a game the account is not seated in is accepted: %v", out)
	assert.Nil(t, h.gameIDStored(), "game_id must be NULL when the account has no seat in that game")
}

// A seated player's report keeps the game it was filed from.
func TestCreateRenderCrashReport_SeatedGameIsKept(t *testing.T) {
	h := newRenderCrashHarness(t)

	code, out := h.do(0, validReport(h.tg.Game.ID))
	require.Equalf(t, http.StatusCreated, code, "report accepted: %v", out)
	gameID := h.gameIDStored()
	require.NotNil(t, gameID)
	assert.Equal(t, h.tg.Game.ID, *gameID)
}

func TestCreateRenderCrashReport_OptionalStackAndGame(t *testing.T) {
	h := newRenderCrashHarness(t)

	// A throw that was not an Error has no stack, and a crash off a table page
	// has no game: both are stored as NULL, not refused.
	code, out := h.do(0, map[string]any{
		"boundary": "somewhere", "message": "a bare string", "route": "/profile", "build": "b",
	})
	require.Equalf(t, http.StatusCreated, code, "stack and game_id are optional: %v", out)

	var stack *string
	var gameID *int64
	err := h.pool.QueryRow(context.Background(),
		"SELECT stack, game_id FROM render_crash_reports ORDER BY id DESC LIMIT 1").Scan(&stack, &gameID)
	require.NoError(t, err)
	assert.Nil(t, stack)
	assert.Nil(t, gameID)
}
