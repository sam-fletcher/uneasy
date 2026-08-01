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

// TestMakeDemands_RejectAlreadyDemanded_EvenWhenResolved: audit D4. A demand
// that has already resolved still holds its target's one demand slot. Two plans
// on a row have a full focus-player turn between them, so without this a third
// player could prepare a second demand on the same still-pending target during
// that turn; both would end up resolved+made, and DemandWinnersForTargetPlan —
// first match in id order — would honour the older one, quietly throwing away
// the second pair's four draft picks.
//
// Also guards the DB backstop: migration 053's uq_one_demand_per_target must
// reject the insert even if the Go check is bypassed.
func TestMakeDemands_RejectAlreadyDemanded_EvenWhenResolved(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, 5)

	// P3's demand against it has already resolved.
	spent := createPlanOnRow(t, q, &tg.Game, &tg.Players[2],
		model.PlanMakeDemands, model.CategoryPower, 5)
	require.NoError(t, q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: spent.ID, TargetedPlanID: &target.ID,
	}))
	res := makeOutcome
	require.NoError(t, q.SetPlanResult(ctx, dbgen.SetPlanResultParams{
		ID: spent.ID, Result: &res,
	}))

	vc := &ValidationContext{
		Q: q, Game: &tg.Game, Player: &tg.Players[0], TargetPlanID: &target.ID,
	}
	_, errMsg := mdHandler{}.ValidatePreparation(ctx, vc)
	assert.Contains(t, errMsg, "another demand already targets")

	// The prep grid must agree, or the card offers a target that submit rejects.
	eligible, _, err := mdHandler{}.CheckPrepEligibility(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, eligible, "a spent target is not an eligible demand target")

	// DB backstop: a second demand row pointing at the same target is refused.
	second := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower, 5)
	err = q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: second.ID, TargetedPlanID: &target.ID,
	})
	require.Error(t, err, "uq_one_demand_per_target must reject the second demand")
}

func TestMakeDemands_RejectDemandTarget(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Target is another player's (unresolved) Make Demands plan. Demand-on-
	// demand is rejected: the Stage-4 winner routes can't service it.
	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeDemands, model.CategoryPower, 5)

	vc := &ValidationContext{
		Q:            q,
		Game:         &tg.Game,
		Player:       &tg.Players[0],
		TargetPlanID: &target.ID,
	}
	_, errMsg := mdHandler{}.ValidatePreparation(ctx, vc)
	assert.NotEmpty(t, errMsg, "expected rejection")
	assert.Contains(t, errMsg, "another demand")
}

func TestMakeDemands_CounterDemand_RejectDemandTarget(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// P1 (the original demander) has an unresolved Make Demands plan; P0 tries
	// to aim their counter-demand at it. The synthesis path must apply the same
	// restriction as ValidatePreparation.
	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeDemands, model.CategoryPower, 5)

	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: hub.NewManager()}
	_, errMsg, status := synthesizeCounterDemand(
		ctx, deps, &tg.Game, tg.Players[0].ID, tg.Players[1].ID, target.ID)
	assert.Contains(t, errMsg, "another demand")
	assert.Equal(t, http.StatusBadRequest, status)
}

// TestMakeDemands_CounterDemand_RejectThirdPartyTarget: the rules put the
// counter on "one of YOUR plans" — the original demander's. A plan belonging to
// an uninvolved player is not a legal counter target (audit D3); the server used
// to accept it, leaving the rule to the client's picker filter alone.
func TestMakeDemands_CounterDemand_RejectThirdPartyTarget(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 4)
	ctx := context.Background()

	// P2 is uninvolved: not the demander (P1), not the counterer (P0).
	thirdParty := createPlanOnRow(t, q, &tg.Game, &tg.Players[2],
		model.PlanProposeDecree, model.CategoryPower, 5)

	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: hub.NewManager()}
	_, errMsg, status := synthesizeCounterDemand(
		ctx, deps, &tg.Game, tg.Players[0].ID, tg.Players[1].ID, thirdParty.ID)
	assert.Contains(t, errMsg, "may only target a plan prepared by")
	assert.Equal(t, http.StatusBadRequest, status)

	// The same call against the demander's own plan is accepted.
	demanderPlan := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, 6)
	counter, errMsg, _ := synthesizeCounterDemand(
		ctx, deps, &tg.Game, tg.Players[0].ID, tg.Players[1].ID, demanderPlan.ID)
	require.Empty(t, errMsg, "the demander's own plan is a legal counter target")
	assert.Equal(t, demanderPlan.ID, *counter.TargetedPlanID)
}

// ── CheckPrepEligibility (PrepEligibilityChecker hook) ────────────────────────

func TestMakeDemands_CheckPrepEligibility_NoPlansOnRecord(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	eligible, reason, err := mdHandler{}.CheckPrepEligibility(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, eligible, "empty public record → nothing to demand against")
	assert.Contains(t, reason, "demanded against")
}

func TestMakeDemands_CheckPrepEligibility_TargetExists(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Another player's pending plan with a row → demandable.
	createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, 5)

	eligible, reason, err := mdHandler{}.CheckPrepEligibility(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.True(t, eligible, "expected eligible; reason: %s", reason)
}

func TestMakeDemands_CheckPrepEligibility_ExcludedTargets(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Own plan: not demandable.
	createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryPower, 5)
	// Make War: never demandable.
	createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeWar, model.CategoryPower, 6)
	// Another demand: not demandable (demand-on-demand unsupported).
	createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeDemands, model.CategoryPower, 7)
	// Resolving plan: a demand can't slot in before it anymore.
	resolving := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanSpreadRumors, model.CategoryEsteem, 8)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: resolving.ID, Status: model.PlanResolving,
	}))
	// Already targeted by an unresolved demand: taken.
	taken := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanChronicleHistories, model.CategoryKnowledge, 9)
	demand := createPlanOnRow(t, q, &tg.Game, &tg.Players[2],
		model.PlanMakeDemands, model.CategoryPower, 9)
	require.NoError(t, q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: demand.ID, TargetedPlanID: &taken.ID,
	}))

	eligible, reason, err := mdHandler{}.CheckPrepEligibility(ctx, q, tg.Game.ID, tg.Players[0].ID)
	require.NoError(t, err)
	assert.False(t, eligible, "every plan on the record is excluded")
	assert.Contains(t, reason, "demanded against")
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
	recipient, err := AssetRecipientForPlan(ctx, q, &reloaded)
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

	// P1 — the original demander, whose plans are the only legal counter
	// targets — prepares a plan on row 7 that the counter-demand will target.
	counterTarget := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, 7)

	deps := &PlanDeps{Store: &db.Store{Q: q}, Manager: hub.NewManager()}
	counter, errMsg, _ := synthesizeCounterDemand(ctx, deps, &tg.Game,
		tg.Players[0].ID, tg.Players[1].ID, counterTarget.ID)
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
		// The generic lifecycle endpoint, so the draft/counter tests can walk all
		// the way to a resolved plan. Its absence is how D1 shipped: every route
		// this harness mounted was an ExtraRoute, so nothing ever exercised
		// CanComplete for this plan type.
		rr.Post("/complete", CompletePlan(store, manager))
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

	// Stand in for finalizeRoll's tail. Resolving the roll row directly skips it,
	// but in real play every roll passes through applyAutoChoiceOnRoll, which is
	// what records the outcome on the demand (mdHandler.AutoApplyChoiceOnRoll) —
	// and that recorded outcome is what CanComplete gates on. Seeding without it
	// would leave these tests exercising a state the server never produces.
	resolvedRoll, err := q.GetDiceRollByPlanID(ctx, &demand.ID)
	require.NoError(t, err)
	require.NoError(t, applyAutoChoiceOnRoll(ctx, q, hub.NewManager(), &resolvedRoll))

	// Re-fetch the demand so callers see the updated status/targeted_plan_id.
	reloaded, err := q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	return target, reloaded
}

// TestMakeDemandsHTTP_CompleteAfterDraft walks the full made-demand lifecycle to
// a RESOLVED plan: roll → four draft picks → /complete.
//
// This is the regression guard for audit D1. CanComplete used to gate on
// plan.Result, which the server only ever writes in the same statement that sets
// status='resolved' — so it was nil for every plan CompletePlan could be called
// on, /complete always 409'd, and a demand sat in 'resolving' forever with the
// row behind it frozen. It shipped because no test called /complete for this
// plan type; the file's other HTTP tests stop at the sub-flow route.
func TestMakeDemandsHTTP_CompleteAfterDraft(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	ctx := context.Background()
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "make")

	// The outcome is recorded the moment the dice land, before any draft pick.
	seeded, err := h.q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	assert.Equal(t, makeOutcome, loadResolutionData(seeded.ResolutionData).MakeDemands.Outcome,
		"AutoApplyChoiceOnRoll should record the outcome on the plan")

	completePath := "/api/plans/" + itoa(demand.ID) + "/complete"

	// Mid-draft the plan is not completable yet.
	status, body := h.post(t, 0, completePath, nil)
	assert.Equal(t, http.StatusConflict, status)
	assert.Contains(t, body["error"], "draft incomplete")

	draftPath := "/api/plans/" + itoa(demand.ID) + "/draft-choice"
	for _, p := range []struct {
		idx    int
		option string
	}{
		{0, game.DemandOptionControlLeverage},
		{1, game.DemandOptionKeepOrChangeTarget},
		{0, game.DemandOptionKeepAssets},
		{1, game.DemandOptionPerformSteps},
	} {
		st, b := h.post(t, p.idx, draftPath, map[string]any{"option": p.option})
		require.Equal(t, http.StatusOK, st, "draft pick failed: %v", b)
	}

	status, body = h.post(t, 0, completePath, nil)
	require.Equal(t, http.StatusOK, status, "completing a fully-drafted demand: %v", body)
	assert.Equal(t, makeOutcome, body["result"])

	resolved, err := h.q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status, "the demand must leave 'resolving'")
	require.NotNil(t, resolved.Result)
	assert.Equal(t, makeOutcome, *resolved.Result)
}

// TestMakeDemandsHTTP_CompleteAfterCounter is D1's marred half: the demand
// completes once the target has placed or deferred their counter, and not before.
func TestMakeDemandsHTTP_CompleteAfterCounter(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	ctx := context.Background()
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "mar")

	completePath := "/api/plans/" + itoa(demand.ID) + "/complete"
	status, body := h.post(t, 0, completePath, nil)
	assert.Equal(t, http.StatusConflict, status, "no counter placed yet")
	assert.Contains(t, body["error"], "counter-demand")

	st, b := h.post(t, 1, "/api/plans/"+itoa(demand.ID)+"/counter-demand",
		map[string]any{"target_plan_id": nil})
	require.Equal(t, http.StatusOK, st, "deferring the counter: %v", b)

	status, body = h.post(t, 0, completePath, nil)
	require.Equal(t, http.StatusOK, status, "completing a countered demand: %v", body)
	assert.Equal(t, marOutcome, body["result"])

	resolved, err := h.q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status)
}

// TestMakeDemandsHTTP_DraftChoice_AcceptedAfterMakeRoll: a made demand
// must accept /draft-choice (the first pick by the higher-ranked drafter).
// This regression-guards the unreachable-check bug.
func TestMakeDemandsHTTP_DraftChoice_AcceptedAfterMakeRoll(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "make")

	// C3 attribution: a made demand opens the alternating draft. The bar names
	// the higher-ranked drafter first (P0, power rank 1), through the real flow.
	rs, err := ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDemandDraftPick, rs.Kind, "made demand → draft")
	assert.Equal(t, []int64{h.tg.Players[0].ID}, rs.ActingPlayerIDs, "first pick = rank-1 demander")

	// Demander (P0, rank 1) picks first.
	status, body := h.post(t, 0,
		"/api/plans/"+itoa(demand.ID)+"/draft-choice",
		map[string]any{"option": game.DemandOptionControlLeverage})
	assert.Equal(t, http.StatusOK, status, "made demand should accept draft pick, got body=%v", body)
	assert.Equal(t, float64(1), body["picks_done"])

	// After the first pick the draft alternates to the lower-ranked drafter (the
	// target plan's preparer, P1) — the bar follows the turn.
	rs, err = ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{h.tg.Players[1].ID}, rs.ActingPlayerIDs, "second pick = target preparer")
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

	// C3 attribution: a marred demand blocks on the TARGET plan's preparer (P1),
	// who decides whether to counter — not the demander, not the focus player.
	rs, err := ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDemandCounter, rs.Kind, "marred demand → counter window")
	assert.Equal(t, []int64{h.tg.Players[1].ID}, rs.ActingPlayerIDs, "counter actor = target preparer")

	// Target-plan preparer (P1) defers the counter — simplest valid body.
	status, body := h.post(t, 1,
		"/api/plans/"+itoa(demand.ID)+"/counter-demand",
		map[string]any{"target_plan_id": nil})
	assert.Equal(t, http.StatusOK, status, "marred demand should accept counter, got body=%v", body)
	assert.Equal(t, true, body["deferred"])

	// Counter placed → the override clears; the row rides generic resolution,
	// naming the demand's own preparer (P0).
	rs, err = ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, rs.Kind, "counter placed → generic")
	assert.Equal(t, []int64{h.tg.Players[0].ID}, rs.ActingPlayerIDs, "generic names the demander")
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
