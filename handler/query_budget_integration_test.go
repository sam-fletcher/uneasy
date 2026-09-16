//go:build integration

package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// Query-budget tests for the main-event paths that were rebuilt around
// batched reads (adr: the Sept 2026 DB round-trips work, step 3). Same idea as
// prologue_query_budget_integration_test.go: every statement through a pgx
// tracer (openCountingPool), and the count pinned — either to a budget, or to
// "does not grow with N" for the loops that used to be N+1.
//
// The counted harness drives real HTTP handlers (cookie auth included), so the
// numbers are what production pays per request, BEGIN/COMMIT and the
// session lookup included.

// countedLifecycle is a planLifecycle on a counting pool. openTestDB runs
// first so the schema is migrated and truncated before the second pool opens.
func countedLifecycle(t *testing.T, n int) (*planLifecycle, *queryCounter) {
	t.Helper()
	openTestDB(t)
	pool, counter := openCountingPool(t)
	h := newPlanLifecycleOn(t, pool, n)
	require.NoError(t, pool.Ping(context.Background()))
	return h, counter
}

// measure resets the counter, runs fn, and returns the statements it issued.
func measure(counter *queryCounter, fn func()) int64 {
	counter.n.Store(0)
	fn()
	return counter.n.Load()
}

func budgetPath(h *planLifecycle, suffix string) string {
	return "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + suffix
}

// ── GET /plan-eligibility ────────────────────────────────────────────────────

// planEligibilityQueryBudget: session, player+game (one join), peer count,
// and the three board reads (tokens, rankings, plans). It was 54 — four or
// five lookups for each of the twelve plan types.
const planEligibilityQueryBudget = 6

func countPlanEligibility(t *testing.T, n int) int64 {
	t.Helper()
	h, counter := countedLifecycle(t, n)
	var code int
	got := measure(counter, func() { code, _ = h.get(0, budgetPath(h, "/plan-eligibility")) })
	require.Equal(t, http.StatusOK, code)
	return got
}

func TestPlanEligibility_QueryBudget(t *testing.T) {
	got := countPlanEligibility(t, 5)
	assert.LessOrEqual(t, got, int64(planEligibilityQueryBudget),
		"plan-eligibility issued %d statements; budget is %d", got, planEligibilityQueryBudget)
	t.Logf("plan-eligibility: %d statements (budget %d)", got, planEligibilityQueryBudget)
}

// The grid's cost must not scale with the roster (the shield/rank checks read
// one rankings snapshot, never a rank per token holder).
func TestPlanEligibility_QueryCountIndependentOfRoster(t *testing.T) {
	assert.Equal(t, countPlanEligibility(t, 2), countPlanEligibility(t, 5))
}

// ── GET /state ───────────────────────────────────────────────────────────────

// The table load's serial chain is the session lookup, the player+game join,
// then one fan-out; the count here is the fan-out's width, not its depth.
// Pinned so a new per-player or per-plan read shows up as a failing number.
const gameStateQueryBudget = 19

func TestGetGameState_QueryBudget(t *testing.T) {
	h, counter := countedLifecycle(t, 5)
	var code int
	got := measure(counter, func() { code, _ = h.get(0, budgetPath(h, "/state")) })
	require.Equal(t, http.StatusOK, code)
	assert.LessOrEqual(t, got, int64(gameStateQueryBudget),
		"GET /state issued %d statements; budget is %d", got, gameStateQueryBudget)
	t.Logf("GET /state: %d statements (budget %d)", got, gameStateQueryBudget)
}

// ── POST /refresh-assets ─────────────────────────────────────────────────────

// leverageOwnAssets marks k of the focus player's non-peer assets leveraged
// and returns their ids.
func leverageOwnAssets(t *testing.T, h *planLifecycle, playerIdx, k int) []int64 {
	t.Helper()
	ctx := context.Background()
	assets, err := h.q.ListAssetsByOwner(ctx, h.tg.Players[playerIdx].ID)
	require.NoError(t, err)
	ids := make([]int64, 0, k)
	for _, a := range assets {
		if a.AssetType == model.AssetPeer || len(ids) == k {
			continue
		}
		require.NoError(t, h.q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{ID: a.ID, IsLeveraged: true}))
		ids = append(ids, a.ID)
	}
	require.Len(t, ids, k, "seeded player has fewer than %d non-peer assets", k)
	return ids
}

func countRefreshAssets(t *testing.T, k int) int64 {
	t.Helper()
	h, counter := countedLifecycle(t, 3)
	focus := h.focusPlayerIdx()
	// Row 3 so up to three assets may be refreshed.
	h.jumpToRow(3)
	ids := leverageOwnAssets(t, h, focus, k)
	var code int
	var body map[string]any
	got := measure(counter, func() {
		code, body = h.post(focus, budgetPath(h, "/refresh-assets"), map[string]any{"asset_ids": ids})
	})
	require.Equalf(t, http.StatusOK, code, "refresh-assets: %v", body)
	return got
}

// Refreshing three assets costs what refreshing one does: one batched read to
// validate, one UPDATE ... RETURNING to refresh. It was two lookups per asset.
func TestRefreshAssets_QueryCountIndependentOfAssetCount(t *testing.T) {
	assert.Equal(t, countRefreshAssets(t, 1), countRefreshAssets(t, 3))
}

// ── POST /scenes ─────────────────────────────────────────────────────────────

// otherPlayersMainCharacters returns the main-character peer of every player
// except idx, for use as a scene's present_peer_ids.
func otherPlayersMainCharacters(t *testing.T, h *planLifecycle, idx int) []int64 {
	t.Helper()
	var ids []int64
	for i, p := range h.tg.Players {
		if i == idx {
			continue
		}
		mc, err := h.q.GetMainCharacterByOwner(context.Background(), dbgen.GetMainCharacterByOwnerParams{GameID: h.tg.Game.ID, OwnerID: p.ID})
		require.NoError(t, err)
		ids = append(ids, mc.ID)
	}
	return ids
}

func countCreateScene(t *testing.T, n int) int64 {
	t.Helper()
	h, counter := countedLifecycle(t, n)
	focus := h.focusPlayerIdx()
	peers := otherPlayersMainCharacters(t, h, focus)
	var code int
	var body map[string]any
	got := measure(counter, func() {
		code, body = h.post(focus, budgetPath(h, "/scenes"), map[string]any{
			"location_custom": "The Long Gallery", "time_elapsed": "days", "present_peer_ids": peers,
		})
	})
	require.Equalf(t, http.StatusCreated, code, "create scene: %v", body)
	return got
}

// A scene with four present peers costs what a scene with one does: the
// peers are validated in one batched read, and named in the log from that
// same read. It was one lookup per peer before the insert and one after.
func TestCreateScene_QueryCountIndependentOfPeerCount(t *testing.T) {
	assert.Equal(t, countCreateScene(t, 2), countCreateScene(t, 5))
}

// ── POST /plans/{id}/select-stakes ───────────────────────────────────────────

func countSelectStakes(t *testing.T, k int) int64 {
	t.Helper()
	h, counter := countedLifecycle(t, 3)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 3
	targetID := h.tg.Players[targIdx].ID
	notes := "Courtyard duel at dawn"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanProposeDuel,
		TargetPlayerID:   &targetID,
		DuelType:         "arms",
		PreparationNotes: &notes,
	})
	// The preparer at esteem rank 1 may stake the most; the target at 5.
	pinEsteemRank(t, h, h.tg.Players[targIdx].ID, 5)
	pinEsteemRank(t, h, h.tg.Players[prepIdx].ID, 1)
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	for _, idx := range []int{prepIdx, targIdx} {
		code, body := h.post(idx, duelRoute(plan.ID, "elect-champion"), map[string]any{"asset_id": nil})
		require.Equalf(t, http.StatusOK, code, "elect-champion: %v", body)
	}

	assets, err := h.q.ListAssetsByOwner(context.Background(), h.tg.Players[prepIdx].ID)
	require.NoError(t, err)
	var stakes []int64
	for _, a := range assets {
		if a.AssetType != model.AssetPeer && len(stakes) < k {
			stakes = append(stakes, a.ID)
		}
	}
	require.Len(t, stakes, k)

	var code int
	var body map[string]any
	got := measure(counter, func() {
		code, body = h.post(prepIdx, duelRoute(plan.ID, "select-stakes"), map[string]any{"asset_ids": stakes})
	})
	require.Equalf(t, http.StatusOK, code, "select-stakes: %v", body)
	return got
}

// Validating three stakes is one batched read, like validating one. The
// per-stake CreateDuelStake inserts remain (each rolls its own hidden die),
// so the count grows by exactly one statement per extra stake — no more.
func TestSelectStakes_ValidationIsOneRead(t *testing.T) {
	one := countSelectStakes(t, 1)
	three := countSelectStakes(t, 3)
	assert.Equal(t, one+2, three, "select-stakes: 1 stake=%d, 3 stakes=%d (expected +1 per stake)", one, three)
}

// ── Table creation: tone topics ──────────────────────────────────────────────

// Seeding the default tone topics is one statement, however many there are.
// It was one INSERT per topic — 45 serial round trips inside CreateTable.
func TestSeedDefaultToneTopics_OneStatement(t *testing.T) {
	harness := openTestDB(t)
	ctx := context.Background()
	game, err := dbgen.New(harness).CreateGame(ctx, fmt.Sprintf("TONE%s", randSuffix()))
	require.NoError(t, err)

	pool, counter := openCountingPool(t)
	require.NoError(t, pool.Ping(ctx))
	got := measure(counter, func() {
		require.NoError(t, db.SeedDefaultToneTopics(ctx, dbgen.New(pool), game.ID))
	})
	assert.EqualValues(t, 1, got)

	topics, err := dbgen.New(harness).ListToneTopics(ctx, game.ID)
	require.NoError(t, err)
	assert.Greater(t, len(topics), 40, "the default topic list should be seeded in full")
}
