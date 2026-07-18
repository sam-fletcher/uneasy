//go:build integration

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/gametest"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// createLobbyPlayer creates a fresh account + player in a brand-new (still
// phase=lobby, per the games.phase column default) game, without going
// through seedBase — used for the below-minimum-players case newTestGame
// can't build (it requires n>=2).
func createLobbyPlayer(t *testing.T, q *dbgen.Queries) (dbgen.Game, dbgen.Player) {
	t.Helper()
	ctx := context.Background()
	code, err := db.GenerateJoinCode()
	require.NoError(t, err)
	game, err := q.CreateGame(ctx, code)
	require.NoError(t, err)
	acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "solo-" + randSuffix(), PasswordHash: "x",
	})
	require.NoError(t, err)
	player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID: game.ID, DisplayName: acct.Username, AccountID: acct.ID, IsFacilitator: true,
	})
	require.NoError(t, err)
	return game, player
}

// TestComputeWaitState_Lobby_BelowMinimum: a single-player lobby has no one
// else to wait on, so nobody is named — even though that lone player is the
// facilitator.
func TestComputeWaitState_Lobby_BelowMinimum(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	game, _ := createLobbyPlayer(t, q)
	ctx := context.Background()

	ws, err := ComputeWaitState(ctx, q, game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindNobody, ws.Kind)
	assert.Empty(t, ws.ActingPlayerIDs)
}

// TestComputeWaitState_Lobby_Facilitator: once the lobby has its minimum two
// players, the facilitator (players[0], per seedBase) is named.
func TestComputeWaitState_Lobby_Facilitator(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhaseLobby,
	}))

	ws, err := ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindLobbyFacilitator, ws.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, ws.ActingPlayerIDs)
}

// TestComputeWaitState_Ended: nobody is ever named once the game has ended.
func TestComputeWaitState_Ended(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhaseEnded,
	}))

	ws, err := ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindNobody, ws.Kind)
	assert.Empty(t, ws.ActingPlayerIDs)
}

// TestComputeWaitState_MainEvent_DelegatesToRowState: for main_event,
// ComputeWaitState must carry ComputeRowState's kind and acting-player set
// verbatim — proving it truly delegates rather than reimplementing.
func TestComputeWaitState_MainEvent_DelegatesToRowState(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// Default freshly-seeded board: scene_setting, focus player named.
	rs, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	ws, err := ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitStateKind(rs.Kind), ws.Kind)
	assert.Equal(t, rs.ActingPlayerIDs, ws.ActingPlayerIDs)
	assert.Equal(t, model.RowStateSceneSetting, rs.Kind)

	// A pending plan on the current row: plan_pending, preparer named.
	plan := createPlanOnRow(t, q, &tg.Game, &tg.Players[1],
		model.PlanProposeDecree, model.CategoryEsteem, tg.Game.CurrentRow)
	rs, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	ws, err = ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitStateKind(rs.Kind), ws.Kind)
	assert.Equal(t, rs.ActingPlayerIDs, ws.ActingPlayerIDs)
	assert.Equal(t, model.RowStatePlanPending, rs.Kind)
	_ = plan
}

// TestComputeWaitState_Prologue_Choosing: with no ranking step set yet (the
// choosing sub-phase), the on-turn player is named — ties broken by join
// order, so a fresh game blocks on players[0]. Completing their three turns
// advances the turn to players[1].
func TestComputeWaitState_Prologue_Choosing(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))

	ws, err := ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindPrologueChoosing, ws.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, ws.ActingPlayerIDs)

	for i := int16(1); i <= prologueTurnsPerPlayer; i++ {
		_, err := q.CreatePrologueChoice(ctx, dbgen.CreatePrologueChoiceParams{
			GameID: tg.Game.ID, PlayerID: tg.Players[0].ID, TurnNumber: i,
			SheetType: "titles", ChoiceName: "choice-" + string(rune('a'+i)),
		})
		require.NoError(t, err)
	}
	ws, err = ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindPrologueChoosing, ws.Kind)
	assert.Equal(t, []int64{tg.Players[1].ID}, ws.ActingPlayerIDs,
		"players[0] has taken all 3 turns → the turn passes to players[1]")
}

// TestComputeWaitState_Prologue_Declare: every player who hasn't yet marked
// themselves done spending hearts on the track is named; once all have, no
// one is (the transient beat right before the track auto-resolves).
func TestComputeWaitState_Prologue_Declare(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepDeclarePower
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: tg.Game.ID, PrologueRankingStep: &step,
	}))

	ws, err := ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindPrologueDeclare, ws.Kind)
	assert.ElementsMatch(t, []int64{tg.Players[0].ID, tg.Players[1].ID}, ws.ActingPlayerIDs,
		"nobody has marked done yet → both owe it")

	require.NoError(t, q.SetTrackDone(ctx, dbgen.SetTrackDoneParams{
		GameID: tg.Game.ID, PlayerID: tg.Players[0].ID, Track: gamepkg.PrologueTrackPower, Done: true,
	}))
	ws, err = ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{tg.Players[1].ID}, ws.ActingPlayerIDs)

	require.NoError(t, q.SetTrackDone(ctx, dbgen.SetTrackDoneParams{
		GameID: tg.Game.ID, PlayerID: tg.Players[1].ID, Track: gamepkg.PrologueTrackPower, Done: true,
	}))
	ws, err = ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, ws.ActingPlayerIDs, "both done → no one left to wait on")
}

// TestComputeWaitState_Prologue_PlaceSetAsides: names the track's top-ranked
// REAL player, matching PlaceSetAsides's own auth check — exercising the
// dummy-rank-aware TopOfTrackPlayer with a 3-player game, where dummies
// occupy ranks 1 and 5 (see gamepkg.DummyRanks), so the top real player sits
// at rank 2, not rank 1. seedRankings fills the open ranks (2,3,4) in seat
// order, so players[0] is that top player.
func TestComputeWaitState_Prologue_PlaceSetAsides(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepPlaceSetAsidesPower
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: tg.Game.ID, PrologueRankingStep: &step,
	}))

	ws, err := ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindProloguePlaceSetAsides, ws.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, ws.ActingPlayerIDs,
		"players[0] holds rank 2 — the top REAL player once dummies occupy rank 1")
}

// TestComputeWaitState_Prologue_Closing: every player who hasn't marked
// themselves ready for the closing step is named.
func TestComputeWaitState_Prologue_Closing(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepClosing
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: tg.Game.ID, PrologueRankingStep: &step,
	}))

	ws, err := ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindPrologueClosing, ws.Kind)
	assert.ElementsMatch(t, []int64{tg.Players[0].ID, tg.Players[1].ID}, ws.ActingPlayerIDs)

	require.NoError(t, q.SetClosingReady(ctx, dbgen.SetClosingReadyParams{
		GameID: tg.Game.ID, PlayerID: tg.Players[0].ID, Ready: true,
	}))

	ws, err = ComputeWaitState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{tg.Players[1].ID}, ws.ActingPlayerIDs)
}

// TestComputeWaitState_ShakeUp_Rolling: step 1 names the open roll's actor.
// seedRankings' default esteem order gives players[0] rank 2 and players[1]
// rank 4 (n=2 → open ranks 2,4, dummies 1,3,5); reverse-rank turn order
// (lowest status first) is therefore [players[1], players[0]], so
// SeedShakeUp's seeded first roll belongs to players[1].
func TestComputeWaitState_ShakeUp_Rolling(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	usernames := []string{"su1-" + randSuffix(), "su2-" + randSuffix()}
	seeded, err := gametest.SeedShakeUp(context.Background(), q, usernames)
	require.NoError(t, err)
	ctx := context.Background()

	ws, err := ComputeWaitState(ctx, q, seeded.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindShakeUpRolling, ws.Kind)
	assert.Equal(t, []int64{seeded.Players[1].ID}, ws.ActingPlayerIDs)
}

// TestComputeWaitState_ShakeUp_Spending_NoOpenSpend: with tokens granted but
// no spend announced yet, whoever's turn it is to announce is named — the
// same reverse-rank order as step 1.
func TestComputeWaitState_ShakeUp_Spending_NoOpenSpend(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	usernames := []string{"su1-" + randSuffix(), "su2-" + randSuffix()}
	seeded, err := gametest.SeedShakeUp(context.Background(), q, usernames,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(3))
	require.NoError(t, err)
	ctx := context.Background()

	ws, err := ComputeWaitState(ctx, q, seeded.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindShakeUpSpending, ws.Kind)
	assert.Equal(t, []int64{seeded.Players[1].ID}, ws.ActingPlayerIDs)
}

// TestComputeWaitState_ShakeUp_Spending_OpenSpend: once a spend is announced,
// the pending reactor (the non-spender, still holding tokens) is named
// instead of the spender; once they pass, the wait shifts to the spender's
// commit.
func TestComputeWaitState_ShakeUp_Spending_OpenSpend(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	usernames := []string{"su1-" + randSuffix(), "su2-" + randSuffix()}
	seeded, err := gametest.SeedShakeUp(context.Background(), q, usernames,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(3))
	require.NoError(t, err)
	ctx := context.Background()

	spender := seeded.Players[1] // currentShakeUpActor's first pick, per the rolling-step order above
	spend, err := q.CreateShakeUpSpend(ctx, dbgen.CreateShakeUpSpendParams{
		GameID: seeded.Game.ID, PlayerID: spender.ID,
		Category: gamepkg.ShakeUpCategoryEsteem, OptionKey: gamepkg.ShakeUpOptBumpKnowledge, BaseCost: 1,
	})
	require.NoError(t, err)

	ws, err := ComputeWaitState(ctx, q, seeded.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.WaitKindShakeUpSpending, ws.Kind)
	assert.Equal(t, []int64{seeded.Players[0].ID}, ws.ActingPlayerIDs,
		"the other token-holder hasn't reacted yet")

	_, err = q.CreateShakeUpPass(ctx, dbgen.CreateShakeUpPassParams{
		SpendID: spend.ID, PlayerID: seeded.Players[0].ID,
	})
	require.NoError(t, err)

	ws, err = ComputeWaitState(ctx, q, seeded.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{spender.ID}, ws.ActingPlayerIDs,
		"every reactor has passed → the spender must commit")
}
