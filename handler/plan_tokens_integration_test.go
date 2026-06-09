//go:build integration

package handler

// handler/plan_tokens_integration_test.go — integration coverage for the
// GET /api/tables/{id}/plan-tokens endpoint that drives the prep-grid pips.
// The endpoint exposes the plan_tokens table (one per plan_type/player) to
// every viewer, so the response must list each holder regardless of who asks.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

func TestListPlanTokens(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	store := db.NewStore(pool)
	ctx := context.Background()

	// Two players hold a token on the same Power plan (Make War — multiple
	// preparers are allowed when they don't outrank each other), and a third
	// holds one on a Knowledge plan. The grid renders these as pips.
	makePlanWithToken(t, q, &tg.Game, &tg.Players[0], model.PlanMakeWar, model.CategoryPower)
	makePlanWithToken(t, q, &tg.Game, &tg.Players[1], model.PlanMakeWar, model.CategoryPower)
	makePlanWithToken(t, q, &tg.Game, &tg.Players[2], model.PlanSeekAnswers, model.CategoryKnowledge)

	// Seed a session for a player and call the endpoint as them — the data is
	// global, so any seated player sees every holder.
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: tok, AccountID: tg.Players[2].AccountID})
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Get("/api/tables/{id}/plan-tokens", ListPlanTokens(store))

	req := httptest.NewRequest("GET", "/api/tables/"+strconv.FormatInt(tg.Game.ID, 10)+"/plan-tokens", nil)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Tokens []struct {
			PlanType string `json:"plan_type"`
			PlayerID int64  `json:"player_id"`
		} `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Tokens, 3)

	// Group by plan type so the assertion doesn't depend on row order.
	byType := map[string][]int64{}
	for _, tk := range body.Tokens {
		byType[tk.PlanType] = append(byType[tk.PlanType], tk.PlayerID)
	}
	assert.ElementsMatch(t, []int64{tg.Players[0].ID, tg.Players[1].ID}, byType[string(model.PlanMakeWar)])
	assert.ElementsMatch(t, []int64{tg.Players[2].ID}, byType[string(model.PlanSeekAnswers)])
}
