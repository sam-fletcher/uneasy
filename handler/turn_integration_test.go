//go:build integration

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// TestPassFocus_TwoPlansOnRow_DoesNotAdvancePastUnresolvedPlan is the
// end-to-end regression for the multi-plan-per-row sequencing fix.
//
// Setup: two plans share the current row. The first has resolved and its
// follow-scene is done; the focus player (its setter) is about to pass. The
// second plan is still pending.
//
// When the focus player passes, PassFocus must:
//   - move focus clockwise to the next player,
//   - auto-kick-off the second plan (pending → resolving) for them, and
//   - NOT advance the row past the still-unresolved second plan.
//
// The bug this guards against: broadcastRowState auto-kicks-off the pending
// plan as a side effect (pending → resolving). If PassFocus checked "are
// plans still pending?" *after* that broadcast, the freshly-kicked-off plan
// would no longer count as pending and the row would advance past it. The
// fix evaluates the advance decision from the pre-kickoff plan state.
func TestPassFocus_TwoPlansOnRow_DoesNotAdvancePastUnresolvedPlan(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	store := db.NewStore(pool)
	manager := hub.NewManager()
	// Register a hub so broadcastRowState's auto-kickoff path actually runs
	// (it no-ops when no hub exists for the game).
	manager.GetOrCreate(tg.Game.ID)

	// Session for Players[0] — the focus player finishing plan 1's turn.
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{
		Token: tok, AccountID: tg.Players[0].AccountID,
	})
	require.NoError(t, err)

	// Plan 1: resolved, with its follow-scene set and ended by Players[0].
	resolved1 := createResolvedPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	scene := startFollowScene(t, q, &tg.Game, &tg.Players[0], resolved1.ID)
	require.NoError(t, q.EndScene(ctx, scene.ID))

	// Plan 2: still pending on the same row.
	plan2 := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanSeekAnswers, model.CategoryKnowledge, tg.Game.CurrentRow)

	// Focus sits on the setter (Players[0]).
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID: tg.Game.ID, FocusPlayerID: &tg.Players[0].ID,
	}))
	startRow := tg.Game.CurrentRow

	// Act: Players[0] passes focus.
	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/pass-focus", PassFocus(store, manager))

	path := "/api/tables/" + strconv.FormatInt(tg.Game.ID, 10) + "/pass-focus"
	req := httptest.NewRequest("POST", path, nil)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusOK, rec.Code, "pass-focus failed: %s", rec.Body.String())

	// Assert: focus passed, row held, second plan kicked off.
	g, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.NotNil(t, g.FocusPlayerID)
	assert.Equal(t, tg.Players[1].ID, *g.FocusPlayerID, "focus passes clockwise")
	assert.Equal(t, startRow, g.CurrentRow,
		"row must NOT advance past the still-pending second plan")

	p2, err := q.GetPlanByID(ctx, plan2.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolving, p2.Status,
		"the second plan is auto-kicked off for the new focus player")
}
