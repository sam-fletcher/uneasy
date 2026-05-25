//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

// ── rolls HTTP harness ────────────────────────────────────────────────────────

// rollsHarness wraps a test game with an HTTP router wired to the rolls
// endpoints + cookie-based session middleware, plus per-player session
// tokens for issuing authenticated requests.
type rollsHarness struct {
	tg      testGame
	q       *dbgen.Queries
	store   *db.Store
	manager *hub.Manager
	router  http.Handler
	tokens  []string // tokens[i] authenticates as tg.Players[i]
}

func newRollsHarness(t *testing.T, n int) *rollsHarness {
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
	r.Post("/api/tables/{id}/rolls", CreateRoll(store, manager))
	r.Get("/api/tables/{id}/rolls/active", GetActiveRollForGame(store))
	r.Route("/api/rolls/{rollId}", func(rr chi.Router) {
		rr.Get("/", GetRoll(store))
		rr.Post("/leverage", LeverageRoll(store, manager))
		rr.Post("/use-banked-die", UseBankedDie(store, manager))
		rr.Post("/call-vote", CallVote(store, manager))
		rr.Post("/skip-vote", SkipVote(store, manager))
		rr.Post("/vote", Vote(store, manager))
		rr.Post("/intent", SetIntent(store, manager))
		rr.Post("/ready", SetReady(store, manager))
	})
	return &rollsHarness{tg: tg, q: q, store: store, manager: manager, router: r, tokens: tokens}
}

// do issues an authenticated request as players[playerIdx]. body may be nil.
// Returns (status, decoded JSON).
func (h *rollsHarness) do(
	t *testing.T,
	playerIdx int,
	method, path string,
	body any,
) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

// asset returns the JSON-decoded body, treating the value at `key` as a
// map. Fatals if the shape is wrong.
func asMap(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key].(map[string]any)
	require.True(t, ok, "expected %q to be an object, got %T", key, m[key])
	return v
}

func asInt64(t *testing.T, v any) int64 {
	t.Helper()
	f, ok := v.(float64)
	require.True(t, ok, "expected number, got %T", v)
	return int64(f)
}

// makeAsset is a tiny helper that creates an asset for the given player.
func (h *rollsHarness) makeAsset(t *testing.T, playerIdx int, name string) dbgen.Asset {
	return createTestHolding(t, h.q, h.tg.Game.ID, h.tg.Players[playerIdx].ID, name)
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestRollFlow_SkipVote_AutoResolveOnLastReady(t *testing.T) {
	h := newRollsHarness(t, 2)
	// Each player gets one asset so neither is locked-ready at start.
	h.makeAsset(t, 0, "Sigil")
	h.makeAsset(t, 1, "Cloak")

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)
	status, body := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 2,
	})
	require.Equal(t, http.StatusCreated, status, body)
	rollID := asInt64(t, asMap(t, body, "roll")["id"])

	// Skip vote → leverage stage.
	skip := fmt.Sprintf("/api/rolls/%d/skip-vote", rollID)
	status, _ = h.do(t, 0, "POST", skip, nil)
	require.Equal(t, http.StatusOK, status)

	// Active roll should now be in leverage stage with 2 participants.
	status, active := h.do(t, 0, "GET", fmt.Sprintf("/api/tables/%d/rolls/active", h.tg.Game.ID), nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "leverage", asMap(t, active, "roll")["stage"])
	parts := active["participants"].([]any)
	require.Len(t, parts, 2)

	// Player 1 (non-actor) cannot ready without intent? Actually plan says yes
	// they can ready as "checking." Confirm they can ready with no intent.
	ready := fmt.Sprintf("/api/rolls/%d/ready", rollID)
	status, _ = h.do(t, 1, "POST", ready, map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)

	// Actor readies → last unready → auto-resolve fires.
	status, _ = h.do(t, 0, "POST", ready, map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)

	// Roll should now be resolved.
	roll, err := h.q.GetDiceRollByID(context.Background(), rollID)
	require.NoError(t, err)
	assert.Equal(t, "resolved", roll.Stage)
	require.NotNil(t, roll.Result)
	require.NotNil(t, roll.Outcome)
}

// Regression: actor readies first, while a non-actor with assets has not
// yet picked intent. Resolution must NOT fire — the non-actor is still
// unready (because their participant row starts is_ready=false when they
// have at least one unleveraged asset).
func TestRollFlow_ActorReadyFirst_DoesNotAutoResolve(t *testing.T) {
	h := newRollsHarness(t, 2)
	h.makeAsset(t, 0, "ActorAsset")
	h.makeAsset(t, 1, "OpponentAsset")

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)
	_, body := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 2,
	})
	rollID := asInt64(t, asMap(t, body, "roll")["id"])
	status, _ := h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/skip-vote", rollID), nil)
	require.Equal(t, http.StatusOK, status)

	// Actor readies first — P1 has not picked intent and is still unready.
	status, _ = h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/ready", rollID),
		map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)

	roll, err := h.q.GetDiceRollByID(context.Background(), rollID)
	require.NoError(t, err)
	assert.Equal(t, "leverage", roll.Stage,
		"roll should still be in leverage stage; P1 hasn't readied yet")
	require.Nil(t, roll.Result)
}

func TestRollFlow_HiddenBallot_RevealOnLastVote(t *testing.T) {
	h := newRollsHarness(t, 3)
	for i := range h.tg.Players {
		h.makeAsset(t, i, fmt.Sprintf("Asset%d", i))
	}

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)
	status, body := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 3,
	})
	require.Equal(t, http.StatusCreated, status, body)
	rollID := asInt64(t, asMap(t, body, "roll")["id"])

	// Actor calls vote.
	status, _ = h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/call-vote", rollID), nil)
	require.Equal(t, http.StatusOK, status)

	vote := fmt.Sprintf("/api/rolls/%d/vote", rollID)
	// P0 votes +1; P1 votes -1. P0 fetching the active roll should see only
	// their own vote, not P1's.
	status, _ = h.do(t, 0, "POST", vote, map[string]any{"vote": 1})
	require.Equal(t, http.StatusOK, status)
	status, _ = h.do(t, 1, "POST", vote, map[string]any{"vote": -1})
	require.Equal(t, http.StatusOK, status)

	status, active := h.do(t, 0, "GET",
		fmt.Sprintf("/api/tables/%d/rolls/active", h.tg.Game.ID), nil)
	require.Equal(t, http.StatusOK, status)
	votes, ok := active["votes"].([]any)
	require.True(t, ok)
	require.Len(t, votes, 2)
	for _, v := range votes {
		vm := v.(map[string]any)
		pid := asInt64(t, vm["player_id"])
		if pid == h.tg.Players[0].ID {
			require.Equal(t, float64(1), vm["vote"], "viewer should see own vote")
		} else {
			_, hasVote := vm["vote"]
			require.False(t, hasVote, "other players' votes must be redacted during voting")
		}
	}

	// Last voter (P2) casts -1 → reveal + advance to leverage.
	status, _ = h.do(t, 2, "POST", vote, map[string]any{"vote": -1})
	require.Equal(t, http.StatusOK, status)

	roll, err := h.q.GetDiceRollByID(context.Background(), rollID)
	require.NoError(t, err)
	assert.Equal(t, "leverage", roll.Stage)
	require.NotNil(t, roll.AdjustedDifficulty)
	// 3 + (1 + -1 + -1) = 2.
	assert.Equal(t, int16(2), *roll.AdjustedDifficulty)
}

func TestRollFlow_IntentLockedOnCommit(t *testing.T) {
	h := newRollsHarness(t, 2)
	h.makeAsset(t, 0, "ActorAsset")
	asset := h.makeAsset(t, 1, "OpponentAsset")

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)
	_, body := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 2,
	})
	rollID := asInt64(t, asMap(t, body, "roll")["id"])

	// Actor skips vote → leverage stage.
	status, _ := h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/skip-vote", rollID), nil)
	require.Equal(t, http.StatusOK, status)

	// P1 picks interfere intent.
	status, _ = h.do(t, 1, "POST", fmt.Sprintf("/api/rolls/%d/intent", rollID),
		map[string]any{"intent": "interfere"})
	require.Equal(t, http.StatusOK, status)

	// P1 leverages an asset (commits a die).
	status, _ = h.do(t, 1, "POST", fmt.Sprintf("/api/rolls/%d/leverage", rollID),
		map[string]any{"asset_id": asset.ID})
	require.Equal(t, http.StatusOK, status)

	// P1 tries to switch intent — must be rejected (locked).
	status, _ = h.do(t, 1, "POST", fmt.Sprintf("/api/rolls/%d/intent", rollID),
		map[string]any{"intent": "aid"})
	require.Equal(t, http.StatusConflict, status)
}

func TestRollFlow_AutoUnreadyOnOpposingLeverage(t *testing.T) {
	h := newRollsHarness(t, 2)
	h.makeAsset(t, 0, "ActorAsset")
	h.makeAsset(t, 0, "ActorAsset2")
	interferer := h.makeAsset(t, 1, "InterferenceAsset")

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)
	_, body := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 2,
	})
	rollID := asInt64(t, asMap(t, body, "roll")["id"])

	status, _ := h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/skip-vote", rollID), nil)
	require.Equal(t, http.StatusOK, status)

	// Actor readies (checking).
	status, _ = h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/ready", rollID),
		map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)

	// Confirm actor is ready.
	part0, err := h.q.GetParticipant(context.Background(), dbgen.GetParticipantParams{
		RollID: rollID, PlayerID: h.tg.Players[0].ID,
	})
	require.NoError(t, err)
	assert.True(t, part0.IsReady)

	// P1 sets intent + interferes.
	status, _ = h.do(t, 1, "POST", fmt.Sprintf("/api/rolls/%d/intent", rollID),
		map[string]any{"intent": "interfere"})
	require.Equal(t, http.StatusOK, status)
	status, _ = h.do(t, 1, "POST", fmt.Sprintf("/api/rolls/%d/leverage", rollID),
		map[string]any{"asset_id": interferer.ID})
	require.Equal(t, http.StatusOK, status)

	// Actor should now be auto-unreadied (still has an unleveraged asset).
	part0After, err := h.q.GetParticipant(context.Background(), dbgen.GetParticipantParams{
		RollID: rollID, PlayerID: h.tg.Players[0].ID,
	})
	require.NoError(t, err)
	assert.False(t, part0After.IsReady, "actor with assets left should be auto-unreadied by interference")
}

func TestRollFlow_SkipLeverageWhenNoOneCanCommit(t *testing.T) {
	h := newRollsHarness(t, 2)
	// Nobody has any assets or banked dice.

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)
	_, body := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 2,
	})
	rollID := asInt64(t, asMap(t, body, "roll")["id"])

	// Actor skips vote → leverage entry → short-circuit → auto-resolve.
	status, _ := h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/skip-vote", rollID), nil)
	require.Equal(t, http.StatusOK, status)

	roll, err := h.q.GetDiceRollByID(context.Background(), rollID)
	require.NoError(t, err)
	assert.Equal(t, "resolved", roll.Stage)
	require.NotNil(t, roll.Result)
}

func TestRollFlow_BankedDieRollsRandomFace(t *testing.T) {
	h := newRollsHarness(t, 2)
	h.makeAsset(t, 1, "P1Asset") // P1 has an asset so the leverage stage opens

	// Give the actor a banked die.
	banked, err := h.q.CreateBankedDie(context.Background(), dbgen.CreateBankedDieParams{
		GameID: h.tg.Game.ID, PlayerID: h.tg.Players[0].ID, Source: "liaise",
	})
	require.NoError(t, err)

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)
	_, body := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 1,
	})
	rollID := asInt64(t, asMap(t, body, "roll")["id"])

	status, _ := h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/skip-vote", rollID), nil)
	require.Equal(t, http.StatusOK, status)

	// Actor spends banked die — should not need an intent, should commit
	// without a pre-set face.
	status, _ = h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/use-banked-die", rollID),
		map[string]any{"banked_die_id": banked.ID})
	require.Equal(t, http.StatusOK, status, "banked-die spend should succeed for actor")

	// The die row should exist with no face yet (face gets set at resolution).
	dice, err := h.q.ListDiceByRoll(context.Background(), rollID)
	require.NoError(t, err)
	bankedDieRows := 0
	for _, d := range dice {
		if d.LeveragedAssetID == nil && d.PlayerID == h.tg.Players[0].ID && !d.IsInterference {
			// could be base die or banked die; total base dice is 2.
			bankedDieRows++
		}
	}
	require.Equal(t, 3, bankedDieRows, "expected 2 base dice + 1 banked die for actor")

	// Drive resolution: P1 readies, actor readies → auto-resolve.
	_, _ = h.do(t, 1, "POST", fmt.Sprintf("/api/rolls/%d/ready", rollID),
		map[string]any{"is_ready": true})
	_, _ = h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/ready", rollID),
		map[string]any{"is_ready": true})

	roll, err := h.q.GetDiceRollByID(context.Background(), rollID)
	require.NoError(t, err)
	assert.Equal(t, "resolved", roll.Stage)

	// All dice now have a face assigned by the random roll.
	dice, err = h.q.ListDiceByRoll(context.Background(), rollID)
	require.NoError(t, err)
	for _, d := range dice {
		require.NotNil(t, d.Face, "die %d has no face after resolution", d.ID)
		assert.GreaterOrEqual(t, *d.Face, int16(1))
		assert.LessOrEqual(t, *d.Face, int16(6))
	}
}

// Regression: a roll with no participant rows (e.g. one created before
// migration 031 and not backfilled) must NOT auto-resolve on a Ready
// click. Otherwise the empty-table "unready count = 0" reads vacuously
// as "everyone's ready" and resolves a roll nobody has engaged with.
func TestRollFlow_EmptyParticipants_DoesNotAutoResolve(t *testing.T) {
	h := newRollsHarness(t, 2)
	ctx := context.Background()
	// Hand-roll a roll without seeding participants — mimics a legacy roll
	// that pre-dates the participants table.
	roll, err := h.q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID:     h.tg.Game.ID,
		RowNumber:  &h.tg.Game.CurrentRow,
		ActorID:    h.tg.Players[0].ID,
		Difficulty: 2,
		Stage:      "leverage",
	})
	require.NoError(t, err)
	for range 2 {
		_, err := h.q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
			RollID: roll.ID, PlayerID: h.tg.Players[0].ID, IsInterference: false,
		})
		require.NoError(t, err)
	}

	// Actor clicks Ready. Without participants, the legacy roll must not
	// auto-resolve — the defensive check in maybeAutoResolve treats empty
	// as malformed.
	status, _ := h.do(t, 0, "POST", fmt.Sprintf("/api/rolls/%d/ready", roll.ID),
		map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)

	got, err := h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	assert.Equal(t, "leverage", got.Stage, "legacy roll without participants must not resolve")
	require.Nil(t, got.Result)
}

func TestRollFlow_ActorContextValidation(t *testing.T) {
	h := newRollsHarness(t, 2)
	ctx := context.Background()
	// Make a scene with P1 as focus.
	customLoc := "somewhere"
	scene, err := h.q.CreateScene(ctx, dbgen.CreateSceneParams{
		GameID:         h.tg.Game.ID,
		RowNumber:      h.tg.Game.CurrentRow,
		FocusPlayerID:  h.tg.Players[1].ID,
		LocationCustom: &customLoc,
		TimeElapsed:    model.TimeHours,
	})
	require.NoError(t, err)

	path := fmt.Sprintf("/api/tables/%d/rolls", h.tg.Game.ID)

	// Actor P0 with scene whose focus is P1 → reject.
	status, _ := h.do(t, 0, "POST", path, map[string]any{
		"actor_id": h.tg.Players[0].ID, "difficulty": 1, "scene_id": scene.ID,
	})
	assert.Equal(t, http.StatusConflict, status)

	// Actor P1 with the same scene → OK.
	status, _ = h.do(t, 1, "POST", path, map[string]any{
		"actor_id": h.tg.Players[1].ID, "difficulty": 1, "scene_id": scene.ID,
	})
	assert.Equal(t, http.StatusCreated, status)
}
