//go:build integration

// handler/tone_integration_test.go — coverage for UpdateToneTopic after it
// was folded into a single round trip (UpdateToneTopicStatusScoped).
//
// The phase gate now lives in SQL, fed the phase list that tonesLocked
// admits. TestUpdateToneTopic_PhaseGateMatchesTonesLocked is the guard that
// keeps those two honest: it drives the real endpoint once per phase and
// asserts the observed behaviour against tonesLocked directly, so the pair
// cannot drift apart without a red test — including when a new phase is
// added to model.AllGamePhases.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

type toneHarness struct {
	t      *testing.T
	q      *dbgen.Queries
	tg     testGame
	router http.Handler
	token  string
}

func newToneHarness(t *testing.T) *toneHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	store := db.NewStore(pool)
	manager := hub.NewManager()

	// The seeder builds a board, not a table — tone topics are seeded at
	// table creation in the real flow, so add them here.
	require.NoError(t, db.SeedDefaultToneTopics(context.Background(), q, tg.Game.ID))

	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: tg.Players[0].AccountID,
	})
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Get("/api/tables/{id}/tone", ListToneTopics(store))
	r.Put("/api/tables/{id}/tone/{topicId}", UpdateToneTopic(store, manager))
	r.Post("/api/tables/{id}/tone", AddToneTopic(store, manager))

	return &toneHarness{t: t, q: q, tg: tg, router: r, token: tok}
}

func (h *toneHarness) do(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.token})
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

func (h *toneHarness) setPhase(phase model.GamePhase) {
	h.t.Helper()
	require.NoError(h.t, h.q.SetGamePhase(context.Background(), dbgen.SetGamePhaseParams{
		ID: h.tg.Game.ID, Phase: phase,
	}))
}

// firstTopic returns a seeded topic on the harness game.
func (h *toneHarness) firstTopic() dbgen.ToneTopic {
	h.t.Helper()
	topics, err := h.q.ListToneTopics(context.Background(), h.tg.Game.ID)
	require.NoError(h.t, err)
	require.NotEmpty(h.t, topics, "seeded game should have default tone topics")
	return topics[0]
}

func (h *toneHarness) tonePath(topicID int64) string {
	return "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) +
		"/tone/" + strconv.FormatInt(topicID, 10)
}

// TestUpdateToneTopic_PhaseGateMatchesTonesLocked walks every phase and
// requires the endpoint to agree with tonesLocked. This is the drift guard
// for the phase list the scoped query is handed.
func TestUpdateToneTopic_PhaseGateMatchesTonesLocked(t *testing.T) {
	for _, phase := range model.AllGamePhases() {
		t.Run(string(phase), func(t *testing.T) {
			h := newToneHarness(t)
			topic := h.firstTopic()
			h.setPhase(phase)

			code, out := h.do(http.MethodPut, h.tonePath(topic.ID),
				map[string]any{"status": "include"})

			after, err := h.q.ListToneTopics(context.Background(), h.tg.Game.ID)
			require.NoError(t, err)
			var stored model.ToneTopicStatus
			for _, tt := range after {
				if tt.ID == topic.ID {
					stored = tt.Status
				}
			}

			if tonesLocked(phase) {
				require.Equalf(t, http.StatusConflict, code, "locked phase rejected: %v", out)
				require.Contains(t, out["error"], "locked")
				require.Equal(t, model.ToneDefault, stored,
					"a rejected write must not reach the row")
			} else {
				require.Equalf(t, http.StatusOK, code, "unlocked phase accepted: %v", out)
				require.Equal(t, model.ToneInclude, stored)
			}
		})
	}
}

func TestUpdateToneTopic_AcceptsEveryCycleStatus(t *testing.T) {
	h := newToneHarness(t)
	topic := h.firstTopic()
	h.setPhase(model.PhaseLobby) // the seeder builds a main-event board

	for _, status := range []model.ToneTopicStatus{
		model.ToneInclude, model.ToneAvoidDetail, model.ToneNever, model.ToneDefault,
	} {
		code, out := h.do(http.MethodPut, h.tonePath(topic.ID),
			map[string]any{"status": string(status)})
		require.Equalf(t, http.StatusOK, code, "%s accepted: %v", status, out)
		require.Equal(t, string(status), out["status"])
	}
}

func TestUpdateToneTopic_RejectsUnknownStatus(t *testing.T) {
	h := newToneHarness(t)
	topic := h.firstTopic()

	code, out := h.do(http.MethodPut, h.tonePath(topic.ID),
		map[string]any{"status": "maybe"})
	require.Equal(t, http.StatusBadRequest, code)
	require.Contains(t, out["error"], "invalid status")
}

func TestUpdateToneTopic_MissingTopicIs404(t *testing.T) {
	h := newToneHarness(t)

	code, out := h.do(http.MethodPut, h.tonePath(999999),
		map[string]any{"status": "include"})
	require.Equal(t, http.StatusNotFound, code)
	require.Contains(t, out["error"], "not found")
}

// TestUpdateToneTopic_ForeignTopicIs403 covers the case the scoped query
// separates from "no such topic": the id exists, but on another table.
func TestUpdateToneTopic_ForeignTopicIs403(t *testing.T) {
	h := newToneHarness(t)
	other := newTestGame(t, h.q, 2)
	require.NoError(t, db.SeedDefaultToneTopics(context.Background(), h.q, other.Game.ID))

	foreign, err := h.q.ListToneTopics(context.Background(), other.Game.ID)
	require.NoError(t, err)
	require.NotEmpty(t, foreign)

	code, out := h.do(http.MethodPut, h.tonePath(foreign[0].ID),
		map[string]any{"status": "include"})
	require.Equal(t, http.StatusForbidden, code)
	require.Contains(t, out["error"], "does not belong")

	// And the foreign row is untouched.
	after, err := h.q.ListToneTopics(context.Background(), other.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.ToneDefault, after[0].Status)
}
