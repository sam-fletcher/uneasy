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

// createResolvedPlanOnRow inserts a plan already in the 'resolved' state
// (result + resolved_at set) on the given row, for tests that exercise the
// follow-scene turn between two plans on one row.
func createResolvedPlanOnRow(
	t *testing.T,
	q *dbgen.Queries,
	game *dbgen.Game,
	preparer *dbgen.Player,
	planType model.PlanType,
	category model.RankingCategory,
	row int16,
) dbgen.Plan {
	t.Helper()
	p := createPlanOnRow(t, q, game, preparer, planType, category, row)
	result := "make"
	require.NoError(t, q.SetPlanResult(context.Background(), dbgen.SetPlanResultParams{
		ID: p.ID, Result: &result,
	}))
	resolved, err := q.GetPlanByID(context.Background(), p.ID)
	require.NoError(t, err)
	return resolved
}

// startFollowScene creates the follow-scene attached to a resolved plan
// (resolved_plan_id set), mirroring what CreateScene writes after a
// resolution. Returns the scene so tests can end it.
func startFollowScene(t *testing.T, q *dbgen.Queries, game *dbgen.Game, focus *dbgen.Player, resolvedPlanID int64) dbgen.Scene {
	t.Helper()
	custom := "Follow-scene location"
	scene, err := q.CreateScene(context.Background(), dbgen.CreateSceneParams{
		GameID:         game.ID,
		RowNumber:      game.CurrentRow,
		FocusPlayerID:  focus.ID,
		LocationCustom: &custom,
		TimeElapsed:    model.TimeHours,
		Prompt:         "follow",
		ResolvedPlanID: &resolvedPlanID,
	})
	require.NoError(t, err)
	return scene
}

// TestComputeRowState_FollowSceneSetting_AfterFirstResolve: with two plans on
// a row, once the first resolves the focus player owes its follow-scene before
// the second plan resolves. The state must be SceneSetting — NOT PlanPending —
// even though a second plan is pending on the current row. This is the core of
// the multi-plan-per-row sequencing fix.
func TestComputeRowState_FollowSceneSetting_AfterFirstResolve(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	_ = createResolvedPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	pending := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeIntroductions, model.CategoryKnowledge, tg.Game.CurrentRow)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateSceneSetting, state.Kind,
		"focus player owes the first plan's follow-scene before the second plan resolves")
	assert.Nil(t, state.PlanID, "SceneSetting must not surface the pending second plan")
	_ = pending
}

// TestComputeRowState_FollowSceneActive: while the follow-scene for the
// just-resolved plan is in progress, the row is SceneActive — the pending
// second plan does not pre-empt it.
func TestComputeRowState_FollowSceneActive(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	resolved := createResolvedPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	_ = createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeIntroductions, model.CategoryKnowledge, tg.Game.CurrentRow)
	scene := startFollowScene(t, q, &tg.Game, &tg.Players[0], resolved.ID)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateSceneActive, state.Kind)
	require.NotNil(t, state.SceneID)
	assert.Equal(t, scene.ID, *state.SceneID)
}

// TestComputeRowState_FollowScenePostAction: once the follow-scene ends and
// its setter still holds focus, the row is PostSceneAction (prepare/refresh)
// — still ahead of the pending second plan.
func TestComputeRowState_FollowScenePostAction(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	resolved := createResolvedPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	_ = createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeIntroductions, model.CategoryKnowledge, tg.Game.CurrentRow)
	scene := startFollowScene(t, q, &tg.Game, &tg.Players[0], resolved.ID)
	require.NoError(t, q.EndScene(ctx, scene.ID))

	// Focus is still the setter (Players[0]).
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID: tg.Game.ID, FocusPlayerID: &tg.Players[0].ID,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePostSceneAction, state.Kind)
}

// TestComputeRowState_FollowSceneDone_NextPlanPending: after the follow-scene
// ends AND focus passes to another player, the resolved plan's turn is
// complete and the second pending plan finally surfaces as PlanPending for the
// new focus player to resolve.
func TestComputeRowState_FollowSceneDone_NextPlanPending(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	resolved := createResolvedPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	pending := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanMakeIntroductions, model.CategoryKnowledge, tg.Game.CurrentRow)
	scene := startFollowScene(t, q, &tg.Game, &tg.Players[0], resolved.ID)
	require.NoError(t, q.EndScene(ctx, scene.ID))

	// Focus passed from the setter (Players[0]) to Players[1].
	require.NoError(t, q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID: tg.Game.ID, FocusPlayerID: &tg.Players[1].ID,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanPending, state.Kind)
	require.NotNil(t, state.PlanID)
	assert.Equal(t, pending.ID, *state.PlanID)
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

// TestComputeRowState_LiaiseResolving: a resolving Clandestinely Liaise must
// never block on the focus player (who is often not even a participant). The
// preparer-only phases (together_at_last, done) ride the generic plan_resolving
// case — whose preparer-naming is the frontend's job — so the backend reports
// plain plan_resolving. The collaborative submit phases surface
// liaise_resolving with ActingPlayerIDs naming who still owes a submission, or
// the preparer once both are in.
func TestComputeRowState_LiaiseResolving(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// P0 preparer, P1 partner. P2 may hold focus — must never be named.
	cl := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanClandestinelyLiaise, model.CategoryKnowledge, tg.Game.CurrentRow)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: cl.ID, Status: model.PlanResolving,
	}))

	mutate := func(fn func(ld *gamepkg.LiaiseResolutionData)) {
		reloaded, err := q.GetPlanByID(ctx, cl.ID)
		require.NoError(t, err)
		resData := loadResolutionData(reloaded.ResolutionData)
		ld := resData.EnsureLiaise()
		ld.PartnerID = &tg.Players[1].ID
		fn(ld)
		require.NoError(t, saveResolutionData(ctx, q, cl.ID, resData))
	}

	// together_at_last → preparer-only → generic plan_resolving (no override).
	mutate(func(ld *gamepkg.LiaiseResolutionData) { ld.Phase = gamepkg.LiaisePhaseTogetherAtLast })
	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, got.Kind,
		"together_at_last rides the generic case (frontend names the preparer)")

	// secrets_we_keep, neither submitted → both participants.
	mutate(func(ld *gamepkg.LiaiseResolutionData) {
		ld.Phase = gamepkg.LiaisePhaseSecretsWeKeep
		ld.KeptSecrets = nil
	})
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateLiaiseResolving, got.Kind)
	assert.ElementsMatch(t, []int64{tg.Players[0].ID, tg.Players[1].ID}, got.ActingPlayerIDs,
		"neither has committed a secret → both owe one")

	// secrets_we_keep, both submitted → preparer owes the advance.
	mutate(func(ld *gamepkg.LiaiseResolutionData) {
		ld.KeptSecrets = []gamepkg.KeptSecret{
			{PlayerID: tg.Players[0].ID, AssetID: 1},
			{PlayerID: tg.Players[1].ID, AssetID: 2},
		}
	})
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateLiaiseResolving, got.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, got.ActingPlayerIDs,
		"both committed → preparer owes the advance click")

	// things_we_share, partner submitted → preparer still owes a pick.
	mutate(func(ld *gamepkg.LiaiseResolutionData) {
		ld.Phase = gamepkg.LiaisePhaseThingsWeShare
		ld.ShareSubmitterIDs = []int64{tg.Players[1].ID}
	})
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateLiaiseResolving, got.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, got.ActingPlayerIDs,
		"partner submitted their share-choice → only the preparer still owes one")

	// done → preparer-only → generic plan_resolving.
	mutate(func(ld *gamepkg.LiaiseResolutionData) { ld.Phase = gamepkg.LiaisePhaseDone })
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, got.Kind,
		"done rides the generic case (frontend names the preparer)")
}

// TestComputeRowState_AwaitCourtierResponse: a resolving Exchange Courtiers
// blocks on the TARGET during the target-driven sub-steps (offer, messy break,
// peer claims, mar choices) and rides the generic plan_resolving case for the
// preparer's steps.
func TestComputeRowState_AwaitCourtierResponse(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	row := tg.Game.CurrentRow
	ec, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:         tg.Game.ID,
		PlanType:       model.PlanExchangeCourtiers,
		Category:       model.CategoryPower,
		PreparerID:     tg.Players[0].ID,
		TargetPlayerID: &tg.Players[1].ID,
		RowNumber:      &row,
		RowOrder:       0,
		PreparedAtRow:  tg.Game.CurrentRow,
	})
	require.NoError(t, err)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: ec.ID, Status: model.PlanResolving,
	}))

	mutate := func(fn func(ec *gamepkg.ExchangeCourtiersResolutionData)) {
		reloaded, err := q.GetPlanByID(ctx, ec.ID)
		require.NoError(t, err)
		resData := loadResolutionData(reloaded.ResolutionData)
		fn(resData.EnsureExchangeCourtiers())
		require.NoError(t, saveResolutionData(ctx, q, ec.ID, resData))
	}
	expectTarget := func(msg string) {
		got, err := ComputeRowState(ctx, q, tg.Game.ID)
		require.NoError(t, err)
		assert.Equal(t, model.RowStateAwaitCourtierResponse, got.Kind, msg)
		require.NotNil(t, got.ActingPlayerID)
		assert.Equal(t, tg.Players[1].ID, *got.ActingPlayerID, msg)
	}
	expectGeneric := func(msg string) {
		got, err := ComputeRowState(ctx, q, tg.Game.ID)
		require.NoError(t, err)
		assert.Equal(t, model.RowStatePlanResolving, got.Kind, msg)
	}

	// No resolution_data yet → target owes the opening fair-trade offer.
	expectTarget("opening offer is the target's")

	// Offer made, no decision → preparer owes accept/decline (generic).
	assetID := int64(1)
	mutate(func(e *gamepkg.ExchangeCourtiersResolutionData) { e.FairTradeAssetID = &assetID })
	expectGeneric("accept/decline is the preparer's")

	// Declined + messy break outstanding → target.
	declined := false
	mutate(func(e *gamepkg.ExchangeCourtiersResolutionData) {
		e.FairTradeAccepted = &declined
		e.MessyBreakRequired = true
	})
	expectTarget("messy break is the target's")

	// Messy break done, peer claims outstanding → target.
	mutate(func(e *gamepkg.ExchangeCourtiersResolutionData) {
		e.MessyBreakDone = true
		e.PeerClaimsRequired = 2
		e.PeerClaimsDone = 1
	})
	expectTarget("peer claims are the target's")

	// All claims done, no mar roll → generic (preparer completes).
	mutate(func(e *gamepkg.ExchangeCourtiersResolutionData) { e.PeerClaimsDone = 2 })
	expectGeneric("nothing target-side outstanding → generic")

	// A marred roll with choices not yet submitted → target owes mar choices.
	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, PlanID: &ec.ID, RowNumber: &row,
		ActorID: tg.Players[0].ID, Difficulty: 4, Stage: "resolved",
	})
	require.NoError(t, err)
	res := int16(0)
	mar := marOutcome
	require.NoError(t, q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &res, Outcome: &mar,
	}))
	// Reset the post-claim flags so only the mar-choice gate is live.
	mutate(func(e *gamepkg.ExchangeCourtiersResolutionData) {
		e.PeerClaimsRequired = 0
		e.PeerClaimsDone = 0
		e.MarChoicesSubmitted = false
	})
	expectTarget("mar choices are the target's until submitted")

	mutate(func(e *gamepkg.ExchangeCourtiersResolutionData) { e.MarChoicesSubmitted = true })
	expectGeneric("mar choices submitted → generic (preparer completes)")
}

// TestComputeRowState_AwaitChronicleChoices: a marred Chronicle Histories blocks
// on every present player who hasn't yet submitted a mar choice.
func TestComputeRowState_AwaitChronicleChoices(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	row := tg.Game.CurrentRow
	ch := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanChronicleHistories, model.CategoryKnowledge, row)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: ch.ID, Status: model.PlanResolving,
	}))

	// A make roll keeps it generic (preparer-driven, no all-players gate).
	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, PlanID: &ch.ID, RowNumber: &row,
		ActorID: tg.Players[0].ID, Difficulty: 4, Stage: "resolved",
	})
	require.NoError(t, err)
	res := int16(9)
	make := makeOutcome
	require.NoError(t, q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &res, Outcome: &make,
	}))
	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, got.Kind, "make path is preparer-driven")

	// Flip the roll to mar → every present player owes a choice.
	mar := marOutcome
	require.NoError(t, q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &res, Outcome: &mar,
	}))
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitChronicleChoices, got.Kind)
	assert.ElementsMatch(t,
		[]int64{tg.Players[0].ID, tg.Players[1].ID, tg.Players[2].ID}, got.ActingPlayerIDs,
		"nobody has chosen yet → all present players owe a choice")

	// P0 and P2 submit → only P1 remains.
	reloaded, err := q.GetPlanByID(ctx, ch.ID)
	require.NoError(t, err)
	resData := loadResolutionData(reloaded.ResolutionData)
	resData.MakeMarChoices = []gamepkg.Choice{
		{PlayerID: &tg.Players[0].ID}, {PlayerID: &tg.Players[2].ID},
	}
	require.NoError(t, saveResolutionData(ctx, q, ch.ID, resData))
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitChronicleChoices, got.Kind)
	assert.Equal(t, []int64{tg.Players[1].ID}, got.ActingPlayerIDs,
		"only the player who hasn't chosen is named")

	// All three submit → generic (preparer completes).
	resData.MakeMarChoices = append(resData.MakeMarChoices, gamepkg.Choice{PlayerID: &tg.Players[1].ID})
	require.NoError(t, saveResolutionData(ctx, q, ch.ID, resData))
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, got.Kind, "all chose → generic")
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

// TestComputeRowState_AwaitDemandDraftPick: a made Make Demands plan with
// an in-progress draft surfaces as AwaitDemandDraftPick, with the acting
// player alternating between demander and target-plan preparer by power
// rank (higher-ranked = lower rank number picks first).
func TestComputeRowState_AwaitDemandDraftPick(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	// P0 is demander (rank 1, higher esteem → picks first).
	// P1 is target-plan preparer (rank 2).
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: &tg.Players[0].ID, Category: model.CategoryPower, Rank: 1,
	}))
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: &tg.Players[1].ID, Category: model.CategoryPower, Rank: 2,
	}))

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
		GameID: tg.Game.ID, PlanID: &demand.ID, RowNumber: &tg.Game.CurrentRow,
		ActorID: tg.Players[0].ID, Difficulty: 4, Stage: "resolved",
	})
	require.NoError(t, err)
	res := int16(10)
	make := makeOutcome
	require.NoError(t, q.ResolveDiceRoll(ctx, dbgen.ResolveDiceRollParams{
		ID: roll.ID, Result: &res, Outcome: &make,
	}))

	// 0 picks: blocks on first picker (P0, rank 1).
	got, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDemandDraftPick, got.Kind)
	require.NotNil(t, got.ActingPlayerID)
	assert.Equal(t, tg.Players[0].ID, *got.ActingPlayerID, "first pick = higher-ranked (P0)")

	// 1 pick → second picker (P1).
	resData := loadResolutionData(demand.ResolutionData)
	md := resData.EnsureMakeDemands()
	md.DraftChoices = []gamepkg.DraftChoice{
		{PlayerID: tg.Players[0].ID, Option: gamepkg.DemandOptionControlLeverage},
	}
	require.NoError(t, saveResolutionData(ctx, q, demand.ID, resData))
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDemandDraftPick, got.Kind)
	assert.Equal(t, tg.Players[1].ID, *got.ActingPlayerID, "second pick = lower-ranked (P1)")

	// 4 picks → draft complete, override clears (back to plan_resolving).
	md.DraftChoices = []gamepkg.DraftChoice{
		{PlayerID: tg.Players[0].ID, Option: gamepkg.DemandOptionControlLeverage},
		{PlayerID: tg.Players[1].ID, Option: gamepkg.DemandOptionKeepOrChangeTarget},
		{PlayerID: tg.Players[0].ID, Option: gamepkg.DemandOptionKeepAssets},
		{PlayerID: tg.Players[1].ID, Option: gamepkg.DemandOptionPerformSteps},
	}
	require.NoError(t, saveResolutionData(ctx, q, demand.ID, resData))
	got, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStatePlanResolving, got.Kind,
		"draft complete → revert to plan_resolving (CompletePlan handles next)")
}

// TestComputeRowState_AwaitDemandCounter_OnlyAfterMar: a made demand
// routes to the draft override, not the counter override. This guards
// the make/mar dispatch in demandSubPhase.
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
	assert.Equal(t, model.RowStateAwaitDemandDraftPick, state.Kind,
		"made demand routes to the draft override, not the counter override")
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
