//go:build integration

package handler

import (
	"context"
	"strconv"
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

// TestComputeRowState_AwaitDemandCounter: a resolving Make Demands plan
// whose dice roll outcome is 'mar' and whose CounterDemandPlaced flag is
// still false should report AwaitDemandCounter instead of PlanResolving,
// with ActingPlayerID = the demand target's preparer (= the player who
// must decide whether to counter).
func TestComputeRowState_AwaitDemandCounter(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Target plan owned by P2 (so the counter actor is non-focus).
	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, tg.Game.CurrentRow)
	demand := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: demand.ID, TargetedPlanID: &target.ID,
	}))
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: demand.ID, Status: model.PlanResolving,
	}))

	// Resolved dice roll for the demand with outcome=mar.
	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID:     tg.Game.ID,
		PlanID:     &demand.ID,
		RowNumber:  &tg.Game.CurrentRow,
		ActorID:    tg.Players[0].ID,
		Difficulty: 4,
		Stage:      "resolved",
	})
	require.NoError(t, err)
	res := int16(0)
	mar := marOutcome
	require.NoError(t, q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &res, Outcome: &mar,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDemandCounter, state.Kind,
		"marred demand with no counter yet must surface as await_demand_counter")
	require.NotNil(t, state.PlanID)
	assert.Equal(t, demand.ID, *state.PlanID)
	require.NotNil(t, state.ActingPlayerID)
	assert.Equal(t, tg.Players[1].ID, *state.ActingPlayerID,
		"acting player must be the target plan's preparer")

	// Once CounterDemandPlaced flips, the override stops firing and the
	// row falls back to plain plan_resolving.
	reloaded, err := q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	resData := loadResolutionData(reloaded.ResolutionData)
	resData.EnsureMakeDemands().CounterDemandPlaced = true
	require.NoError(t, saveResolutionData(ctx, q, demand.ID, resData))

	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, state.Kind,
		"counter placed → revert to plan_resolving")
	assert.Nil(t, state.ActingPlayerID)
}

// TestComputeRowState_AwaitDemandCounter_OnlyAfterMar: a made (or unresolved)
// roll on a Make Demands plan must NOT trigger the counter override.
func TestComputeRowState_AwaitDemandCounter_OnlyAfterMar(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	target := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryPower, tg.Game.CurrentRow)
	demand := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeDemands, model.CategoryPower, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
		ID: demand.ID, TargetedPlanID: &target.ID,
	}))
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: demand.ID, Status: model.PlanResolving,
	}))
	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID:     tg.Game.ID,
		PlanID:     &demand.ID,
		RowNumber:  &tg.Game.CurrentRow,
		ActorID:    tg.Players[0].ID,
		Difficulty: 4,
		Stage:      "resolved",
	})
	require.NoError(t, err)
	res := int16(10)
	make := makeOutcome
	require.NoError(t, q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &res, Outcome: &make,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, state.Kind,
		"made demand should not trigger counter override")
}

// TestComputeRowState_AwaitFestivityGuestTurn: a resolving Host Festivity
// in 'socializing' phase blocks on the next guest in esteem order — host
// is P0 (focus); P1 has lower esteem than P2, so P1 should go first.
func TestComputeRowState_AwaitFestivityGuestTurn(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// Esteem ranks: P0 rank 1 (highest), P2 rank 2, P1 rank 3 (lowest).
	// Lowest-esteem guest acts first, host last.
	for rank, pid := range []int64{tg.Players[0].ID, tg.Players[2].ID, tg.Players[1].ID} {
		p := pid
		require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: tg.Game.ID, PlayerID: &p, Category: model.CategoryEsteem, Rank: int16(rank + 1),
		}))
	}

	hf := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanHostFestivity, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: hf.ID, Status: model.PlanResolving,
	}))

	// Seed festivity state: all three players are guests, none have acted.
	resData := loadResolutionData(hf.ResolutionData)
	state := resData.EnsureFestivity()
	state.Phase = gamepkg.FestivityPhaseSocializing
	state.Guests = []int64{tg.Players[0].ID, tg.Players[1].ID, tg.Players[2].ID}
	require.NoError(t, saveResolutionData(ctx, q, hf.ID, resData))

	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitFestivityGuestTurn, got.Kind)
	require.NotNil(t, got.ActingPlayerID)
	assert.Equal(t, tg.Players[1].ID, *got.ActingPlayerID,
		"lowest-esteem guest (P1, rank 3) must act first")
	require.NotNil(t, got.PlanID)
	assert.Equal(t, hf.ID, *got.PlanID)

	// After P1 acts, the next-actor should be P2 (rank 2).
	reloaded, err := q.GetPlanByID(ctx, hf.ID)
	require.NoError(t, err)
	resData = loadResolutionData(reloaded.ResolutionData)
	state = resData.EnsureFestivity()
	state.Outcomes = map[string]string{
		strconv.FormatInt(tg.Players[1].ID, 10): gamepkg.FestivityOutcomeOptOut,
	}
	require.NoError(t, saveResolutionData(ctx, q, hf.ID, resData))

	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitFestivityGuestTurn, got.Kind)
	require.NotNil(t, got.ActingPlayerID)
	assert.Equal(t, tg.Players[2].ID, *got.ActingPlayerID,
		"after P1 acts, P2 (rank 2) goes next; host is last")

	// After both guests act, only host remains — host = focus, but kind
	// still surfaces so the WaitingOnBar's label reflects the festivity.
	state.Outcomes[strconv.FormatInt(tg.Players[2].ID, 10)] = gamepkg.FestivityOutcomeMake
	require.NoError(t, saveResolutionData(ctx, q, hf.ID, resData))
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitFestivityGuestTurn, got.Kind)
	require.NotNil(t, got.ActingPlayerID)
	assert.Equal(t, tg.Players[0].ID, *got.ActingPlayerID,
		"host acts last in the socializing phase")
}

// TestComputeRowState_AwaitFestivityChallengeResponse: an open challenge
// overrides the guest-turn kind regardless of whose turn it is.
func TestComputeRowState_AwaitFestivityChallengeResponse(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	hf := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanHostFestivity, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: hf.ID, Status: model.PlanResolving,
	}))

	resData := loadResolutionData(hf.ResolutionData)
	state := resData.EnsureFestivity()
	state.Phase = gamepkg.FestivityPhaseSocializing
	state.Guests = []int64{tg.Players[0].ID, tg.Players[1].ID, tg.Players[2].ID}
	state.PendingChallenge = &gamepkg.PendingChallenge{
		ChallengerID: tg.Players[1].ID,
		TargetID:     tg.Players[2].ID,
	}
	require.NoError(t, saveResolutionData(ctx, q, hf.ID, resData))

	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitFestivityChallengeResponse, got.Kind)
	require.NotNil(t, got.ActingPlayerID)
	assert.Equal(t, tg.Players[2].ID, *got.ActingPlayerID,
		"challenge target is the waitee")
}

// TestComputeRowState_FestivityHostChoosing_NoOverride: outside the
// socializing phase the festivity falls back to plain plan_resolving (the
// host is the focus player, so the default copy is already correct).
func TestComputeRowState_FestivityHostChoosing_NoOverride(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	hf := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanHostFestivity, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: hf.ID, Status: model.PlanResolving,
	}))

	resData := loadResolutionData(hf.ResolutionData)
	state := resData.EnsureFestivity()
	state.Phase = gamepkg.FestivityPhaseHostChoosing
	state.Guests = []int64{tg.Players[0].ID}
	require.NoError(t, saveResolutionData(ctx, q, hf.ID, resData))

	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, got.Kind)
	assert.Nil(t, got.ActingPlayerID)
}

// TestComputeRowState_AwaitDuelStaking: setup/staking phases surface as
// AwaitDuelStaking with no acting player (multiple waitees; client derives).
func TestComputeRowState_AwaitDuelStaking(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	row := tg.Game.CurrentRow
	duel, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:         tg.Game.ID,
		PlanType:       model.PlanProposeDuel,
		Category:       model.CategoryEsteem,
		PreparerID:     tg.Players[0].ID,
		TargetPlayerID: &tg.Players[1].ID,
		RowNumber:      &row,
		RowOrder:       0,
		PreparedAtRow:  tg.Game.CurrentRow,
	})
	require.NoError(t, err)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: duel.ID, Status: model.PlanResolving,
	}))
	resData := loadResolutionData(duel.ResolutionData)
	state := resData.EnsureDuel()
	state.Phase = gamepkg.DuelPhaseSetup
	require.NoError(t, saveResolutionData(ctx, q, duel.ID, resData))

	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDuelStaking, got.Kind, "setup → await_duel_staking")
	assert.Nil(t, got.ActingPlayerID, "staking has multiple waitees, client derives")
	require.NotNil(t, got.PlanID)
	assert.Equal(t, duel.ID, *got.PlanID)

	// 'staking' phase shares the same kind.
	reloaded, err := q.GetPlanByID(ctx, duel.ID)
	require.NoError(t, err)
	resData = loadResolutionData(reloaded.ResolutionData)
	resData.EnsureDuel().Phase = gamepkg.DuelPhaseStaking
	require.NoError(t, saveResolutionData(ctx, q, duel.ID, resData))
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDuelStaking, got.Kind, "staking → same kind as setup")
}

// TestComputeRowState_AwaitDuelBout: bouts phase blocks on the declarer
// (= InitiativePlayerID) until they declare, then on the responder until
// the bout resolves.
func TestComputeRowState_AwaitDuelBout(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	row := tg.Game.CurrentRow
	duel, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:         tg.Game.ID,
		PlanType:       model.PlanProposeDuel,
		Category:       model.CategoryEsteem,
		PreparerID:     tg.Players[0].ID,
		TargetPlayerID: &tg.Players[1].ID,
		RowNumber:      &row,
		RowOrder:       0,
		PreparedAtRow:  tg.Game.CurrentRow,
	})
	require.NoError(t, err)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: duel.ID, Status: model.PlanResolving,
	}))

	resData := loadResolutionData(duel.ResolutionData)
	state := resData.EnsureDuel()
	state.Phase = gamepkg.DuelPhaseBouts
	initiative := tg.Players[0].ID
	state.InitiativePlayerID = &initiative
	require.NoError(t, saveResolutionData(ctx, q, duel.ID, resData))

	// No bout yet → blocks on initiative-holder (declarer).
	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDuelBout, got.Kind)
	require.NotNil(t, got.ActingPlayerID)
	assert.Equal(t, tg.Players[0].ID, *got.ActingPlayerID,
		"no bout yet → declarer (initiative holder) acts")

	// Create a stake for each duellist and a declared-but-unresolved bout.
	asset0, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[0].ID, CreatorID: tg.Players[0].ID,
		AssetType: model.AssetPeer, Name: "P0 peer",
	})
	require.NoError(t, err)
	stake0, err := q.CreateDuelStake(ctx, dbgen.CreateDuelStakeParams{
		PlanID: duel.ID, PlayerID: tg.Players[0].ID, AssetID: asset0.ID, HiddenDie: 4,
	})
	require.NoError(t, err)

	decl := string(gamepkg.DeclHigh)
	die := int16(4)
	_, err = q.CreateDuelBout(ctx, dbgen.CreateDuelBoutParams{
		PlanID: duel.ID, BoutNumber: 1,
		DeclarerID: tg.Players[0].ID, DeclarerStakeID: stake0.ID,
		ResponderID: tg.Players[1].ID,
		Declaration: &decl, DeclarerDie: &die,
	})
	require.NoError(t, err)

	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDuelBout, got.Kind)
	require.NotNil(t, got.ActingPlayerID)
	assert.Equal(t, tg.Players[1].ID, *got.ActingPlayerID,
		"declared but unresolved bout → responder acts")
}

// TestComputeRowState_DuelRollPhase_NoOverride: 'roll' phase falls back to
// plain plan_resolving (standard dice flow; default copy is correct).
func TestComputeRowState_DuelRollPhase_NoOverride(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	duel := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDuel, model.CategoryEsteem, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: duel.ID, Status: model.PlanResolving,
	}))
	resData := loadResolutionData(duel.ResolutionData)
	resData.EnsureDuel().Phase = gamepkg.DuelPhaseRoll
	require.NoError(t, saveResolutionData(ctx, q, duel.ID, resData))

	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, got.Kind)
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
