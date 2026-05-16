//go:build integration

// TEST_DATABASE_URL stored in .env

package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/hub"
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
// new demand target. synthesizeCounterDemand should create the counter plan at
// target.row - 1 (or the current row, whichever is later), wire the
// targeted_plan_id column, and bypass normal token/eligibility checks.
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

	assert.Equal(t, new(int16(6)), counter.RowNumber, "row = target.row - 1")
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
	assert.Equal(t, new(int16(5)), counter.RowNumber, "row = newPlan.row - 1")

	// Pending row is marked resolved.
	open, err := q.ListOpenPendingCounterDemandsForPlayer(ctx, tg.Players[0].ID)
	require.NoError(t, err)
	assert.Empty(t, open, "pending row should be marked resolved")
}
