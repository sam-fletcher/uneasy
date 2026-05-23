//go:build integration

package handler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// TestComputeRowState_PhaseNotMainEvent covers the trivial branch: anything
// other than main_event should report PhaseNotMainEvent regardless of what
// else is in the DB.
func TestComputeRowState_PhaseNotMainEvent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	for _, phase := range []model.GamePhase{
		model.PhaseLobby, model.PhasePrologue, model.PhaseShakeUp, model.PhaseEnded,
	} {
		require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID: tg.Game.ID, Phase: phase,
		}))
		state, err := ComputeRowState(ctx, q, tg.Game.ID)
		require.NoError(t, err)
		assert.Equal(t, model.RowStatePhaseNotMainEvent, state.Kind, "phase=%s", phase)
	}
}

// TestComputeRowState_DefaultSceneSetting: a fresh main-event game with a
// focus player and no scene/plans/wars lands in SceneSetting.
func TestComputeRowState_DefaultSceneSetting(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateSceneSetting, state.Kind)
	assert.Nil(t, state.PlanID)
	assert.Nil(t, state.SceneID)
}

// TestComputeRowState_SceneActive: an unstarted-by-name scene with
// ended_at IS NULL puts the row in SceneActive.
func TestComputeRowState_SceneActive(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	scene := startTurnScene(t, q, &tg.Game, &tg.Players[0])

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateSceneActive, state.Kind)
	require.NotNil(t, state.SceneID)
	assert.Equal(t, scene.ID, *state.SceneID)
}

// TestComputeRowState_PostSceneAction: once EndScene runs, the row enters
// the post-scene action step.
func TestComputeRowState_PostSceneAction(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	scene := startTurnScene(t, q, &tg.Game, &tg.Players[0])
	require.NoError(t, q.EndScene(ctx, scene.ID))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePostSceneAction, state.Kind)
}

// TestComputeRowState_PlanPending: a pending plan on the current row wins
// over the post-scene action step.
func TestComputeRowState_PlanPending_BeatsPostScene(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	scene := startTurnScene(t, q, &tg.Game, &tg.Players[0])
	require.NoError(t, q.EndScene(ctx, scene.ID))

	plan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanPending, state.Kind)
	require.NotNil(t, state.PlanID)
	assert.Equal(t, plan.ID, *state.PlanID)
}

// TestComputeRowState_PlanPending_IgnoresOtherRow: a pending plan on a
// future row doesn't dethrone the current focus player's step.
func TestComputeRowState_PlanPending_IgnoresOtherRow(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Pending plan on row 2 while we're on row 1 — should NOT show as
	// the current-row state; the row's still SceneSetting.
	_ = createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, 2)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateSceneSetting, state.Kind)
}

// TestComputeRowState_PlanResolving: a resolving plan wins over a pending
// plan even on the current row.
func TestComputeRowState_PlanResolving_BeatsPending(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	resolving := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: resolving.ID, Status: model.PlanResolving,
	}))
	_ = createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower, tg.Game.CurrentRow)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, state.Kind)
	require.NotNil(t, state.PlanID)
	assert.Equal(t, resolving.ID, *state.PlanID)
}

// TestComputeRowState_AwaitDelayReveal_MakeWar: a Make War plan whose
// row_number is still nil takes precedence over scene state but is
// dethroned by an actively-resolving plan.
func TestComputeRowState_AwaitDelayReveal_MakeWar(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	mw, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        tg.Game.ID,
		PlanType:      model.PlanMakeWar,
		Category:      model.CategoryPower,
		PreparerID:    tg.Players[0].ID,
		RowNumber:     nil,
		RowOrder:      0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDelayReveal, state.Kind)
	require.NotNil(t, state.PlanID)
	assert.Equal(t, mw.ID, *state.PlanID)

	// A resolving plan should still win — plan resolution is in-progress
	// play; the delay reveal is queued behind it.
	resolving := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: resolving.ID, Status: model.PlanResolving,
	}))

	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, state.Kind)
}

// TestComputeRowState_AwaitDelayReveal_Liaise: Clandestinely Liaise blocks
// the row identically to Make War while its delay reveal is open. The two
// plan types share the same row_state kind; the client dispatches to the
// right panel via the plan_id.
func TestComputeRowState_AwaitDelayReveal_Liaise(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	cl, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        tg.Game.ID,
		PlanType:      model.PlanClandestinelyLiaise,
		Category:      model.CategoryKnowledge,
		PreparerID:    tg.Players[0].ID,
		RowNumber:     nil,
		RowOrder:      0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDelayReveal, state.Kind,
		"CL delay reveal must block the row, same as Make War")
	require.NotNil(t, state.PlanID)
	assert.Equal(t, cl.ID, *state.PlanID)
}

// TestComputeRowState_SurrenderClaim_PreemptsPlans: per the rulebook,
// resolving an open surrender claim (a consequence of step 1's battle costs)
// blocks all play — including plan resolution and preparation.
func TestComputeRowState_SurrenderClaim_PreemptsPlans(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	originPlan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeWar, model.CategoryPower, tg.Game.CurrentRow)
	war, err := q.CreateWar(ctx, dbgen.CreateWarParams{
		GameID:       tg.Game.ID,
		OriginPlanID: originPlan.ID,
		StartedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)
	require.NoError(t, q.CreateSurrenderClaim(ctx, dbgen.CreateSurrenderClaimParams{
		WarID:         war.ID,
		SurrenderedID: tg.Players[0].ID,
		ClaimantID:    tg.Players[1].ID,
	}))

	// Even with a resolving plan present, the surrender claim must win —
	// it's a rulebook step 1 consequence that blocks all in-row play.
	resolving := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: resolving.ID, Status: model.PlanResolving,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitSurrenderClaim, state.Kind)
	require.NotNil(t, state.ClaimID)
}

// TestComputeRowState_BattleCostGate_PreemptsPlans: per the rulebook,
// step 1 (pay battle costs) must precede everything else in a row,
// including plan resolution. AwaitBattleCost sits above PlanResolving.
func TestComputeRowState_BattleCostGate_PreemptsPlans(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Move to row 2 so the war's started_at_row (1) is in the past — the
	// "missing battle costs" helper only checks rows >= started_at_row.
	require.NoError(t, q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: tg.Game.ID, CurrentRow: 2,
	}))
	tg.Game.CurrentRow = 2

	originPlan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeWar, model.CategoryPower, 1)
	// The Make War plan has to be in a non-pending state or its open
	// delay-reveal would preempt the battle-cost gate.
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: originPlan.ID, Status: model.PlanResolved,
	}))
	row1 := int16(1)
	require.NoError(t, q.SetPlanRowAndOrder(ctx, dbgen.SetPlanRowAndOrderParams{
		ID: originPlan.ID, RowNumber: &row1, RowOrder: 0,
	}))

	war, err := q.CreateWar(ctx, dbgen.CreateWarParams{
		GameID:       tg.Game.ID,
		OriginPlanID: originPlan.ID,
		StartedAtRow: 1,
	})
	require.NoError(t, err)
	require.NoError(t, q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
		WarID:       war.ID,
		PlayerID:    tg.Players[0].ID,
		Side:        gamepkg.WarSideDeclarer,
		JoinedAtRow: 1,
	}))
	require.NoError(t, q.SetWarParticipantEntryComplete(ctx, dbgen.SetWarParticipantEntryCompleteParams{
		WarID: war.ID, PlayerID: tg.Players[0].ID,
	}))
	require.NoError(t, q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
		WarID:       war.ID,
		PlayerID:    tg.Players[1].ID,
		Side:        gamepkg.WarSideEnemy,
		JoinedAtRow: 1,
	}))
	require.NoError(t, q.SetWarParticipantEntryComplete(ctx, dbgen.SetWarParticipantEntryCompleteParams{
		WarID: war.ID, PlayerID: tg.Players[1].ID,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitBattleCost, state.Kind)
	require.NotNil(t, state.WarID)
	assert.Equal(t, war.ID, *state.WarID)

	// Even a resolving plan must not displace the battle-cost gate.
	resolving := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: resolving.ID, Status: model.PlanResolving,
	}))

	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitBattleCost, state.Kind,
		"battle cost must preempt plan resolution per rulebook step 1")
}

// startTurnScene creates an active turn-scene for the focus player on the
// current row. Convenience wrapper that mirrors the StartScene handler's DB
// write but skips the location/peer machinery the tests don't need.
func startTurnScene(t *testing.T, q *dbgen.Queries, game *dbgen.Game, focus *dbgen.Player) dbgen.Scene {
	t.Helper()
	custom := "Test location"
	scene, err := q.CreateScene(context.Background(), dbgen.CreateSceneParams{
		GameID:         game.ID,
		RowNumber:      game.CurrentRow,
		FocusPlayerID:  focus.ID,
		LocationCustom: &custom,
		TimeElapsed:    model.TimeHours,
		Prompt:         "",
		ResolvedPlanID: nil,
	})
	require.NoError(t, err)
	return scene
}

// shim to keep `time` imported in case future tests want timestamps.
var _ = pgtype.Timestamptz{Time: time.Time{}, Valid: false}
