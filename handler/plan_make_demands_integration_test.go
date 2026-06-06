//go:build integration

// TEST_DATABASE_URL stored in .env

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// ── Rejection tests ───────────────────────────────────────────────────────────

func TestMakeDemands_RejectMakeWarTarget(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Target is a Make War plan prepared by a different player.
	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeWar, model.CategoryPower, 5)

	vc := &ValidationContext{
		Q:            q,
		Game:         &tg.Game,
		Player:       &tg.Players[0],
		TargetPlanID: &target.ID,
	}
	_, errMsg := mdHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg, "expected rejection")
	assert.Contains(t, errMsg, "Make War")
}

func TestMakeDemands_RejectAlreadyDemanded(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// P2 prepares a Propose Decree (legitimate demand target) on row 5.
	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, 5)

	// P3 has already demanded against it — unresolved.
	existingDemand := createPlanOnRow(t, q, &tg.Game, &tg.Players[2],
		model.PlanMakeDemands, model.CategoryPower, 4)
	require.NoError(t, q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID:             existingDemand.ID,
		TargetedPlanID: &target.ID,
	}))

	// P1 tries to demand against the same target — should reject.
	vc := &ValidationContext{
		Q:            q,
		Game:         &tg.Game,
		Player:       &tg.Players[0],
		TargetPlanID: &target.ID,
	}
	_, errMsg := mdHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg)
	assert.Contains(t, errMsg, "another demand already targets")
}

// ── Happy-path: asset recipient transfers on made demand ──────────────────────

// The target plan's preparer is P2 (power rank 2). The demander P3 (rank 3)
// wins all 4 drafts, which means P3 is keep_assets winner. After drafts are
// complete, AssetRecipientForPlan on the target should return P3's ID rather
// than the target's own preparer.
func TestMakeDemands_HappyPath_AssetRecipientTransfers(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, 5)

	demand := createPlanOnRow(t, q, &tg.Game, &tg.Players[2],
		model.PlanMakeDemands, model.CategoryPower, 4)
	require.NoError(t, q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: demand.ID, TargetedPlanID: &target.ID,
	}))

	// Simulate a made, resolved demand with P3 sweeping all four option wins.
	winners := game.DemandOptionWinners{
		game.DemandOptionControlLeverage:    tg.Players[2].ID,
		game.DemandOptionKeepOrChangeTarget: tg.Players[2].ID,
		game.DemandOptionKeepAssets:         tg.Players[2].ID,
		game.DemandOptionPerformSteps:       tg.Players[2].ID,
	}
	raw, err := json.Marshal(winners)
	require.NoError(t, err)
	require.NoError(t, q.SetDemandOptionWinners(ctx, dbgen.SetDemandOptionWinnersParams{
		ID: demand.ID, DemandOptionWinners: raw,
	}))
	madeResult := "make"
	require.NoError(t, q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
		ID: demand.ID, Result: &madeResult,
	}))

	reloaded, err := q.GetPlanByID(ctx, target.ID)
	require.NoError(t, err)
	recipient, err := game.AssetRecipientForPlan(ctx, q, &reloaded)
	require.NoError(t, err)
	assert.Equal(t, tg.Players[2].ID, recipient,
		"asset recipient should be the keep_assets winner (demander)")
}

// ── Immediate counter-demand: synthesizeCounterDemand ─────────────────────────

// After a marred demand, the target of the demand may immediately nominate a
// new demand target. synthesizeCounterDemand should create the counter plan on
// the *same* row as its target, slotted in immediately before it (so the
// counter resolves first), wire the targeted_plan_id column, and bypass
// normal token/eligibility checks.
func TestMakeDemands_ImmediateCounterDemand(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// P2 prepares a plan on row 7 that the counter-demand will target.
	counterTarget := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, 7)

	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: hub.NewManager()}
	counter, errMsg, _ := synthesizeCounterDemand(ctx, deps, &tg.Game,
		tg.Players[0].ID, counterTarget.ID)
	assert.Empty(t, errMsg, "synthesize should succeed")
	assert.NotNil(t, counter)

	assert.Equal(t, new(int16(7)), counter.RowNumber, "counter slots on the target's row")
	// Re-fetch target — its row_order should have been shifted up by one.
	refreshedTarget, err := q.GetPlanByID(ctx, counterTarget.ID)
	require.NoError(t, err)
	assert.Equal(t, counter.RowOrder+1, refreshedTarget.RowOrder,
		"target shifted up; counter takes the slot before it")
	assert.Equal(t, tg.Players[0].ID, counter.PreparerID)
	assert.Equal(t, model.PlanMakeDemands, counter.PlanType)
	assert.NotNil(t, counter.TargetedPlanID)
	assert.Equal(t, counterTarget.ID, *counter.TargetedPlanID)
}

// ── Pending counter-demand: consumePendingCounterDemandFor ───────────────────

// When the target of a marred demand defers their counter (no current plan to
// target), a pending_counter_demands row is created. The next time the
// original demander prepares any plan, that row is consumed and a free
// counter-demand is synthesized against the new plan.
func TestMakeDemands_PendingCounterDemandConsumed(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Original demand: P1 demanded against something that marred. We only
	// need the origin plan row to exist to satisfy the FK.
	origin := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower, 3)

	// P2 (the target of the marred demand) defers — pending row created.
	_, err := q.CreatePendingCounterDemand(ctx, dbgen.CreatePendingCounterDemandParams{
		GameID:            tg.Game.ID,
		DemandingPlayerID: tg.Players[0].ID, // original demander to watch
		TargetPlayerID:    tg.Players[1].ID, // deferred counter-demander
		OriginPlanID:      origin.ID,
	})
	require.NoError(t, err)

	// P1 now prepares some new plan on row 6. This is what the deferred
	// counter-demand will latch onto.
	newPlan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryPower, 6)

	manager := hub.NewManager()
	counterID := consumePendingCounterDemandFor(ctx, q, manager, &tg.Game, &newPlan)
	assert.NotNil(t, counterID, "pending row should have been consumed")

	counter, err := q.GetPlanByID(ctx, *counterID)
	require.NoError(t, err)
	assert.Equal(t, tg.Players[1].ID, counter.PreparerID,
		"counter is owned by the deferred counter-demander")
	assert.NotNil(t, counter.TargetedPlanID)
	assert.Equal(t, newPlan.ID, *counter.TargetedPlanID)
	assert.Equal(t, new(int16(6)), counter.RowNumber, "counter slots on newPlan's row")

	// Pending row is marked resolved.
	open, err := q.ListOpenPendingCounterDemandsForPlayer(ctx, tg.Players[0].ID)
	require.NoError(t, err)
	assert.Empty(t, open, "pending row should be marked resolved")
}

// ── HTTP-level tests for draft-choice and counter-demand ──────────────────────
//
// These exercise the actual HTTP handlers, not just the helpers underneath.
// They regression-guard a real bug that survived for a while: the old
// handlers gated on plan.Result, which is only written by SetPlanResult
// (atomically transitioning status → 'resolved'). Combined with the
// status==resolving check, both gates were unreachable and every call to
// /draft-choice and /counter-demand 409'd. The fix routes both through
// mdRollOutcome (dice-roll outcome lookup), so the tests below send a real
// HTTP request to confirm a 200 is returned end-to-end.

// mdHTTPHarness wires up the same middleware + plan routes as the real
// server, scoped to what these tests need.
type mdHTTPHarness struct {
	tg     testGame
	q      *dbgen.Queries
	router http.Handler
	tokens []string
}

func newMDHTTPHarness(t *testing.T, n int) *mdHTTPHarness {
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
	r.Route("/api/plans/{planId}", func(rr chi.Router) {
		deps := &PlanDeps{Store: store, Manager: manager}
		h, _ := GetHandler(model.PlanMakeDemands)
		for route, fn := range h.ExtraRoutes(deps) {
			rr.Post("/"+route, fn)
		}
	})
	return &mdHTTPHarness{tg: tg, q: q, router: r, tokens: tokens}
}

func (h *mdHTTPHarness) post(t *testing.T, playerIdx int, path string, body any) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest("POST", path, rdr)
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

// seedResolvingDemand creates a demand-with-target pair and a resolved
// dice roll with the given outcome, ready for draft-choice/counter-demand
// to be exercised. Returns (target, demand).
func seedResolvingDemand(
	t *testing.T,
	q *dbgen.Queries,
	tg *testGame,
	demanderIdx, targetPreparerIdx int,
	outcome string,
) (dbgen.Plan, dbgen.Plan) {
	t.Helper()
	ctx := context.Background()

	// Ranks are required because mdDraftPickers consults the power-rank
	// table to pick the first drafter.
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: &tg.Players[demanderIdx].ID,
		Category: model.CategoryPower, Rank: 1,
	}))
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: &tg.Players[targetPreparerIdx].ID,
		Category: model.CategoryPower, Rank: 2,
	}))

	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[targetPreparerIdx],
		model.PlanProposeDecree, model.CategoryPower, tg.Game.CurrentRow)
	demand := createPlanOnRow(t, q, &tg.Game, &tg.Players[demanderIdx],
		model.PlanMakeDemands, model.CategoryPower, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: demand.ID, TargetedPlanID: &target.ID,
	}))
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: demand.ID, Status: model.PlanResolving,
	}))

	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, PlanID: &demand.ID, RowNumber: &tg.Game.CurrentRow,
		ActorID: tg.Players[demanderIdx].ID, Difficulty: 4, Stage: "resolved",
	})
	require.NoError(t, err)
	res := int16(10)
	require.NoError(t, q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &res, Outcome: &outcome,
	}))

	// Re-fetch the demand so callers see the updated status/targeted_plan_id.
	reloaded, err := q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	return target, reloaded
}

// TestMakeDemandsHTTP_DraftChoice_AcceptedAfterMakeRoll: a made demand
// must accept /draft-choice (the first pick by the higher-ranked drafter).
// This regression-guards the unreachable-check bug.
func TestMakeDemandsHTTP_DraftChoice_AcceptedAfterMakeRoll(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "make")

	// Demander (P0, rank 1) picks first.
	status, body := h.post(t, 0,
		"/api/plans/"+itoa(demand.ID)+"/draft-choice",
		map[string]any{"option": game.DemandOptionControlLeverage})
	assert.Equal(t, http.StatusOK, status, "made demand should accept draft pick, got body=%v", body)
	assert.Equal(t, float64(1), body["picks_done"])
}

// TestMakeDemandsHTTP_DraftChoice_RejectedWithoutRoll: with no resolved
// roll, /draft-choice 409s with the "made demand" message. Guards the
// new mdRollOutcome path.
func TestMakeDemandsHTTP_DraftChoice_RejectedWithoutRoll(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	ctx := context.Background()

	// Set up just enough for the handler to reach the outcome check.
	target := createPlanOnRow(t, h.q, &h.tg.Game, &h.tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, h.tg.Game.CurrentRow)
	demand := createPlanOnRow(t, h.q, &h.tg.Game, &h.tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower, h.tg.Game.CurrentRow)
	require.NoError(t, h.q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: demand.ID, TargetedPlanID: &target.ID,
	}))
	require.NoError(t, h.q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: demand.ID, Status: model.PlanResolving,
	}))

	status, body := h.post(t, 0,
		"/api/plans/"+itoa(demand.ID)+"/draft-choice",
		map[string]any{"option": game.DemandOptionControlLeverage})
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, body["error"], "made demand")
}

// TestMakeDemandsHTTP_CounterDemand_AcceptedAfterMarRoll: a marred demand
// must accept /counter-demand from the target plan's preparer (the demand
// target). Regression guard for the same bug class.
func TestMakeDemandsHTTP_CounterDemand_AcceptedAfterMarRoll(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "mar")

	// Target-plan preparer (P1) defers the counter — simplest valid body.
	status, body := h.post(t, 1,
		"/api/plans/"+itoa(demand.ID)+"/counter-demand",
		map[string]any{"target_plan_id": nil})
	assert.Equal(t, http.StatusOK, status, "marred demand should accept counter, got body=%v", body)
	assert.Equal(t, true, body["deferred"])
}

// TestMakeDemandsHTTP_CounterDemand_RejectedAfterMakeRoll: a made demand
// must NOT accept /counter-demand. Guards the make/mar dispatch in
// mdRollOutcome's caller.
func TestMakeDemandsHTTP_CounterDemand_RejectedAfterMakeRoll(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "make")

	status, body := h.post(t, 1,
		"/api/plans/"+itoa(demand.ID)+"/counter-demand",
		map[string]any{"target_plan_id": nil})
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, body["error"], "marred demand")
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// mdSystemPosts returns the bodies of all plan.make_demands action-log posts in
// the game, newest first per ListGamePosts ordering.
func mdSystemPosts(t *testing.T, q *dbgen.Queries, gameID int64) []string {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "plan.make_demands" {
			out = append(out, p.Body)
		}
	}
	return out
}

// TestMakeDemandsHTTP_DraftComplete_EmitsActionLog walks a full four-pick draft
// and asserts the draft-complete action-log entry lands with a winners summary.
func TestMakeDemandsHTTP_DraftComplete_EmitsActionLog(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	// P0 (rank 1) demands; P1 (rank 2) is the target preparer. P0 picks on the
	// even picks (1st, 3rd), P1 on the odd (2nd, 4th).
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "make")
	path := "/api/plans/" + itoa(demand.ID) + "/draft-choice"

	picks := []struct {
		idx    int
		option string
	}{
		{0, game.DemandOptionControlLeverage},
		{1, game.DemandOptionKeepOrChangeTarget},
		{0, game.DemandOptionKeepAssets},
		{1, game.DemandOptionPerformSteps},
	}
	for i, p := range picks {
		status, body := h.post(t, p.idx, path, map[string]any{"option": p.option})
		require.Equalf(t, http.StatusOK, status, "pick %d: %v", i+1, body)
	}

	posts := mdSystemPosts(t, h.q, h.tg.Game.ID)
	var complete string
	for _, b := range posts {
		if strings.Contains(b, "draft complete") {
			complete = b
		}
	}
	require.NotEmptyf(t, complete, "expected a draft-complete action-log post; got %v", posts)
	// P0 took control_leverage + keep_assets; P1 took the other two.
	assert.Contains(t, complete, "leverage control")
	assert.Contains(t, complete, "perform make/mar steps")
}

// TestMakeDemandsHTTP_CounterDemand_EmitsActionLog asserts the deferred
// counter-demand path writes an action-log entry.
func TestMakeDemandsHTTP_CounterDemand_EmitsActionLog(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "mar")

	status, body := h.post(t, 1,
		"/api/plans/"+itoa(demand.ID)+"/counter-demand",
		map[string]any{"target_plan_id": nil})
	require.Equalf(t, http.StatusOK, status, "counter-demand: %v", body)

	posts := mdSystemPosts(t, h.q, h.tg.Game.ID)
	found := false
	for _, b := range posts {
		if strings.Contains(b, "deferred their counter-demand") {
			found = true
		}
	}
	assert.Truef(t, found, "expected a counter-demand action-log post; got %v", posts)
}
