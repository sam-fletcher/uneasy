//go:build integration

package handler

import (
	"bytes"
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
	gamepkg "uneasy/game"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// TestPlaceSetAsides_TopPlayerIsNotAlwaysRank1 is the regression for the
// prologue set-aside softlock.
//
// The ranking track is a fixed 1–5 scale; in a 3-player game dummy tokens
// occupy ranks 1 and 5 (dummyRanksForPlayerCount), so real players sit at
// ranks 2,3,4. PROLOGUE_RULES step 6 lets "the player at the top of the
// track" order the set-aside (zero-suit) players — that's the highest-status
// *real* player, i.e. the lowest-numbered rank with a non-dummy holder, which
// here is rank 2, NOT rank 1.
//
// The original auth check looked up rank == 1 literally, found the dummy
// (PlayerID = nil), and 403'd everyone — softlocking the prologue whenever a
// 2–3 player track produced ≥2 set-asides. This test sets up that exact board
// and asserts the rank-2 player (not the rank-1 dummy) is the one who places.
func TestPlaceSetAsides_TopPlayerIsNotAlwaysRank1(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 3)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)

	// Move the game into the prologue, parked at the power set-aside step.
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepPlaceSetAsidesPower
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: tg.Game.ID, PrologueRankingStep: &step,
	}))

	// Rebuild the power track as the prologue would have left it just before
	// set-asides: dummies at ranks 1 and 5, the single auto-ranked player at
	// rank 2, and the other two players still unranked (the set-asides).
	require.NoError(t, q.DeleteRankingsByCategory(ctx, dbgen.DeleteRankingsByCategoryParams{
		GameID: tg.Game.ID, Category: model.CategoryPower,
	}))
	topPlayer := tg.Players[0] // the real player at rank 2
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: nil, Category: model.CategoryPower, Rank: 1, // dummy
	}))
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: &topPlayer.ID, Category: model.CategoryPower, Rank: 2,
	}))
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: nil, Category: model.CategoryPower, Rank: 5, // dummy
	}))

	router := chi.NewRouter()
	router.Use(appMiddleware.EnsureSession(q))
	router.Post("/api/tables/{id}/prologue/place-set-asides", PlaceSetAsides(store, manager))
	path := "/api/tables/" + strconv.FormatInt(tg.Game.ID, 10) + "/prologue/place-set-asides"

	post := func(t *testing.T, actor dbgen.Player, ordering []int64) *httptest.ResponseRecorder {
		t.Helper()
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{
			Token: tok, AccountID: actor.AccountID,
		})
		require.NoError(t, err)

		raw, err := json.Marshal(map[string]any{"ordering": ordering})
		require.NoError(t, err)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	setAsides := []int64{tg.Players[1].ID, tg.Players[2].ID}

	// A non-top player may not place — but the failure must be a genuine 403,
	// not a "rank-1 is a dummy so nobody qualifies" lockout.
	t.Run("non-top player is forbidden", func(t *testing.T) {
		rec := post(t, tg.Players[1], setAsides)
		assert.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	})

	// The rank-2 real player IS the top of the track and may place the
	// set-asides. Before the fix this 403'd too, freezing the prologue.
	t.Run("top real player at rank 2 may place", func(t *testing.T) {
		rec := post(t, topPlayer, setAsides)
		require.Equalf(t, http.StatusOK, rec.Code, "place failed: %s", rec.Body.String())

		// Set-asides fill the remaining open ranks (3, then 4) in submitted order.
		rankings, err := q.ListRankingsByGame(ctx, tg.Game.ID)
		require.NoError(t, err)
		rankOf := map[int64]int16{}
		for _, rk := range rankings {
			if rk.Category == model.CategoryPower && rk.PlayerID != nil {
				rankOf[*rk.PlayerID] = rk.Rank
			}
		}
		assert.EqualValues(t, 2, rankOf[topPlayer.ID], "auto-ranked player stays at rank 2")
		assert.EqualValues(t, 3, rankOf[tg.Players[1].ID], "first set-aside → rank 3")
		assert.EqualValues(t, 4, rankOf[tg.Players[2].ID], "second set-aside → rank 4")
	})
}
