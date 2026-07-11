//go:build integration

// handler/feedback_integration_test.go — Session 1 coverage for
// adr/FEEDBACK_AND_RESET_PLAN.md: POST /api/feedback (login-gated) and
// POST /api/reset-requests (logged-out), both backed by the shared
// feedback_submissions table.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

// feedbackHarness wires just the two feedback/reset-request routes, plus a
// logged-in session for the account-holding player, tg.Players[0].
type feedbackHarness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	tg     testGame
	router http.Handler
	token  string
}

func newFeedbackHarness(t *testing.T) *feedbackHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	store := db.NewStore(pool)

	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: tg.Players[0].AccountID,
	})
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/feedback", CreateFeedback(store))
	r.Post("/api/reset-requests", CreateResetRequest(store))

	return &feedbackHarness{t: t, pool: pool, q: q, tg: tg, router: r, token: tok}
}

func (h *feedbackHarness) do(method, path string, body any, withAuth bool) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	if withAuth {
		req.AddCookie(&http.Cookie{Name: "player_token", Value: h.token})
	}
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

// countSubmissions returns the number of feedback_submissions rows of kind.
func (h *feedbackHarness) countSubmissions(kind model.FeedbackKind) int {
	h.t.Helper()
	var n int
	err := h.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM feedback_submissions WHERE kind = $1", string(kind)).Scan(&n)
	require.NoError(h.t, err)
	return n
}

func TestCreateFeedback_HappyPath(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/feedback", map[string]any{
		"body":    "the dice roller is confusing",
		"contact": "alice@example.com",
		"game_id": h.tg.Game.ID,
		"route":   "/tables/1/rolls",
		"phase":   "main_event",
	}, true)
	require.Equalf(t, http.StatusCreated, code, "happy path accepted: %v", out)

	sub, ok := out["submission"].(map[string]any)
	require.True(t, ok, "response has a submission object: %v", out)
	assert.Equal(t, "feedback", sub["kind"])
	assert.Equal(t, float64(h.tg.Game.ID), sub["game_id"])
	assert.Equal(t, "alice@example.com", sub["contact"])
	assert.Equal(t, 1, h.countSubmissions(model.FeedbackKindFeedback))
}

func TestCreateFeedback_RequiresLogin(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/feedback", map[string]any{"body": "hello"}, false)
	require.Equalf(t, http.StatusUnauthorized, code, "no session cookie: %v", out)
	assert.Zero(t, h.countSubmissions(model.FeedbackKindFeedback))
}

func TestCreateFeedback_RequiresNonBlankBody(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/feedback", map[string]any{"body": "   "}, true)
	require.Equalf(t, http.StatusBadRequest, code, "blank body rejected: %v", out)
}

func TestCreateFeedback_RateLimitedAfterFive(t *testing.T) {
	h := newFeedbackHarness(t)
	acctID := h.tg.Players[0].AccountID

	// Seed 5 prior submissions directly, bypassing the rate-limit check
	// itself, so the 6th request through the real handler is the one under
	// test.
	for i := 0; i < 5; i++ {
		_, err := h.q.InsertFeedbackSubmission(context.Background(), dbgen.InsertFeedbackSubmissionParams{
			Kind: model.FeedbackKindFeedback, AccountID: &acctID, Body: "prior feedback",
		})
		require.NoError(t, err)
	}

	code, out := h.do(http.MethodPost, "/api/feedback", map[string]any{"body": "one too many"}, true)
	require.Equalf(t, http.StatusTooManyRequests, code, "6th submission within the hour rate-limited: %v", out)
	assert.Equal(t, 5, h.countSubmissions(model.FeedbackKindFeedback), "rejected submission must not be inserted")
}

func TestCreateFeedback_AllowsUpToFive(t *testing.T) {
	h := newFeedbackHarness(t)
	acctID := h.tg.Players[0].AccountID

	for i := 0; i < 4; i++ {
		_, err := h.q.InsertFeedbackSubmission(context.Background(), dbgen.InsertFeedbackSubmissionParams{
			Kind: model.FeedbackKindFeedback, AccountID: &acctID, Body: "prior feedback",
		})
		require.NoError(t, err)
	}

	code, out := h.do(http.MethodPost, "/api/feedback", map[string]any{"body": "the fifth one"}, true)
	require.Equalf(t, http.StatusCreated, code, "5th submission (4 prior) accepted: %v", out)
	assert.Equal(t, 5, h.countSubmissions(model.FeedbackKindFeedback))
}

func TestCreateResetRequest_HappyPath(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/reset-requests", map[string]any{
		"username": "some-locked-out-player",
		"contact":  "player@example.com",
		"body":     "forgot my password",
	}, false)
	require.Equalf(t, http.StatusOK, code, "happy path accepted: %v", out)
	assert.Equal(t, true, out["ok"])
	assert.Equal(t, 1, h.countSubmissions(model.FeedbackKindResetRequest))
}

func TestCreateResetRequest_HoneypotDiscardsSilently(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/reset-requests", map[string]any{
		"username": "a-bot",
		"contact":  "bot@example.com",
		"website":  "http://spam.example", // honeypot — real users never fill this in
	}, false)
	require.Equalf(t, http.StatusOK, code, "honeypot trip still responds 200: %v", out)
	assert.Equal(t, true, out["ok"])
	assert.Equal(t, 0, h.countSubmissions(model.FeedbackKindResetRequest), "honeypot trip must not insert a row")
}

func TestCreateResetRequest_MissingContactRejected(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/reset-requests", map[string]any{
		"username": "someone",
	}, false)
	require.Equalf(t, http.StatusBadRequest, code, "missing contact rejected: %v", out)
	assert.Equal(t, 0, h.countSubmissions(model.FeedbackKindResetRequest))
}

func TestCreateResetRequest_MissingUsernameRejected(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/reset-requests", map[string]any{
		"contact": "player@example.com",
	}, false)
	require.Equalf(t, http.StatusBadRequest, code, "missing username rejected: %v", out)
}

// TestFeedbackGameID_SurvivesDeleteGame proves the ON DELETE SET NULL FK
// (migration 046) detaches a feedback submission from a deleted game rather
// than blocking or cascading the delete — feedback must survive
// dev/delete-game, per the migration-041 cascade audit.
func TestFeedbackGameID_SurvivesDeleteGame(t *testing.T) {
	h := newFeedbackHarness(t)

	code, out := h.do(http.MethodPost, "/api/feedback", map[string]any{
		"body":    "context before the game is gone",
		"game_id": h.tg.Game.ID,
	}, true)
	require.Equalf(t, http.StatusCreated, code, "feedback accepted: %v", out)
	sub := out["submission"].(map[string]any)
	subID := int64(sub["id"].(float64))

	_, err := h.q.DeleteGame(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)

	var gameID *int64
	err = h.pool.QueryRow(context.Background(),
		"SELECT game_id FROM feedback_submissions WHERE id = $1", subID).Scan(&gameID)
	require.NoError(t, err)
	assert.Nil(t, gameID, "game_id must be detached (SET NULL) after the game is deleted")
	assert.Equal(t, 1, h.countSubmissions(model.FeedbackKindFeedback), "the submission row itself must survive")
}

// TestCreateFeedback_CommitsRowEvenWhenWebhookFails proves the DB commit
// (the durable record) doesn't depend on the Discord webhook succeeding —
// NotifyDiscord runs in a goroutine strictly after the row is already
// committed and the response already prepared.
func TestCreateFeedback_CommitsRowEvenWhenWebhookFails(t *testing.T) {
	h := newFeedbackHarness(t)
	t.Cleanup(func() { SetDiscordWebhookURL("") })

	// A closed listener: any POST to this URL fails outright.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	badURL := srv.URL
	srv.Close()
	SetDiscordWebhookURL(badURL)

	code, out := h.do(http.MethodPost, "/api/feedback", map[string]any{"body": "webhook is down"}, true)
	require.Equalf(t, http.StatusCreated, code, "row commits regardless of webhook outcome: %v", out)
	assert.Equal(t, 1, h.countSubmissions(model.FeedbackKindFeedback))
}
