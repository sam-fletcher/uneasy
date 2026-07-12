//go:build integration

// handler/row_state_dice_roll_integration_test.go — coverage for Notifications
// Session 1 (adr/NOTIFICATIONS_PLAN.md): the new open-dice-roll top-of-chain
// gate, and the ActingPlayerIDs fills on the six kinds that previously left it
// nil (scene_setting/scene_active/post_scene_action, await_delay_reveal,
// await_battle_cost, await_surrender_claim).

package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── await_dice_roll ──────────────────────────────────────────────────────────

func TestComputeRowState_AwaitDiceRoll_DecideVote(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, RowNumber: &tg.Game.CurrentRow,
		ActorID: tg.Players[0].ID, Difficulty: 4, Stage: stageDecideVote,
	})
	require.NoError(t, err)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDiceRoll, state.Kind)
	require.NotNil(t, state.RollID)
	assert.Equal(t, roll.ID, *state.RollID)
	assert.Equal(t, []int64{tg.Players[0].ID}, state.ActingPlayerIDs, "decide_vote names only the actor")
}

func TestComputeRowState_AwaitDiceRoll_Voting(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, RowNumber: &tg.Game.CurrentRow,
		ActorID: tg.Players[0].ID, Difficulty: 4, Stage: stageVoting,
	})
	require.NoError(t, err)

	// Only players[1] has voted so far.
	require.NoError(t, q.CreateDifficultyVote(ctx, dbgen.CreateDifficultyVoteParams{
		RollID: roll.ID, PlayerID: tg.Players[1].ID, Vote: 1,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDiceRoll, state.Kind)
	assert.ElementsMatch(t, []int64{tg.Players[0].ID, tg.Players[2].ID}, state.ActingPlayerIDs,
		"voting names everyone who hasn't cast a vote yet")
}

func TestComputeRowState_AwaitDiceRoll_Leverage(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	roll, err := q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, RowNumber: &tg.Game.CurrentRow,
		ActorID: tg.Players[0].ID, Difficulty: 4, Stage: stageLeverage,
	})
	require.NoError(t, err)

	require.NoError(t, q.CreateRollParticipant(ctx, dbgen.CreateRollParticipantParams{
		RollID: roll.ID, PlayerID: tg.Players[0].ID, IsReady: true,
	}))
	require.NoError(t, q.CreateRollParticipant(ctx, dbgen.CreateRollParticipantParams{
		RollID: roll.ID, PlayerID: tg.Players[1].ID, IsReady: false,
	}))
	require.NoError(t, q.CreateRollParticipant(ctx, dbgen.CreateRollParticipantParams{
		RollID: roll.ID, PlayerID: tg.Players[2].ID, IsReady: false,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDiceRoll, state.Kind)
	assert.ElementsMatch(t, []int64{tg.Players[1].ID, tg.Players[2].ID}, state.ActingPlayerIDs,
		"leverage names the unready participants")
}

// TestComputeRowState_AwaitDiceRoll_PreemptsWarGates: an open roll wins over
// the war-conflict gates — matching the client-side override that already
// unconditionally superseded row-state before this gate moved server-side.
func TestComputeRowState_AwaitDiceRoll_PreemptsWarGates(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	require.NoError(t, q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: tg.Game.ID, CurrentRow: 2,
	}))
	tg.Game.CurrentRow = 2

	originPlan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeWar, model.CategoryPower, 1)
	require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: originPlan.ID, Status: model.PlanResolved,
	}))
	row1 := int16(1)
	require.NoError(t, q.SetPlanRowAndOrder(ctx, dbgen.SetPlanRowAndOrderParams{
		ID: originPlan.ID, RowNumber: &row1, RowOrder: 0,
	}))
	war, err := q.CreateWar(ctx, dbgen.CreateWarParams{
		GameID: tg.Game.ID, OriginPlanID: originPlan.ID, StartedAtRow: 1,
	})
	require.NoError(t, err)
	sides := []int16{gamepkg.WarSideDeclarer, gamepkg.WarSideEnemy}
	for i, p := range tg.Players {
		require.NoError(t, q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
			WarID: war.ID, PlayerID: p.ID, Side: sides[i], JoinedAtRow: 1,
		}))
		require.NoError(t, q.SetWarParticipantEntryComplete(ctx, dbgen.SetWarParticipantEntryCompleteParams{
			WarID: war.ID, PlayerID: p.ID,
		}))
	}

	// Sanity: without an open roll, the battle-cost gate fires as usual.
	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStateAwaitBattleCost, state.Kind)

	// An open roll (unrelated to the war) now preempts it.
	_, err = q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, RowNumber: &tg.Game.CurrentRow,
		ActorID: tg.Players[0].ID, Difficulty: 4, Stage: stageDecideVote,
	})
	require.NoError(t, err)

	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDiceRoll, state.Kind,
		"an open roll must preempt the battle-cost gate")
}

// TestComputeRowState_AwaitDiceRoll_PreemptsMainCharacterChoice: same
// top-of-chain precedence, above the replacement-main-character gate.
func TestComputeRowState_AwaitDiceRoll_PreemptsMainCharacterChoice(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[0].ID,
	})
	require.NoError(t, err)
	require.NoError(t, q.TransferAsset(ctx, dbgen.TransferAssetParams{
		ID: mc.ID, OwnerID: tg.Players[1].ID,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStateAwaitMainCharacterChoice, state.Kind, "sanity check")

	_, err = q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
		GameID: tg.Game.ID, RowNumber: &tg.Game.CurrentRow,
		ActorID: tg.Players[1].ID, Difficulty: 4, Stage: stageDecideVote,
	})
	require.NoError(t, err)

	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitDiceRoll, state.Kind,
		"an open roll must preempt the main-character-choice gate")
}

// ── Focus-player kinds now name their actor ─────────────────────────────────

func TestComputeRowState_SceneStates_NameFocusPlayer(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	ctx := context.Background()

	// scene_setting (no turn-scene yet for this row & focus player).
	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStateSceneSetting, state.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, state.ActingPlayerIDs)

	// scene_active.
	scene := startTurnScene(t, q, &tg.Game, &tg.Players[0])
	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStateSceneActive, state.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, state.ActingPlayerIDs)

	// post_scene_action.
	require.NoError(t, q.EndScene(ctx, scene.ID))
	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStatePostSceneAction, state.Kind)
	assert.Equal(t, []int64{tg.Players[0].ID}, state.ActingPlayerIDs)
}

// ── await_delay_reveal names pending submitters ─────────────────────────────

func TestComputeRowState_AwaitDelayReveal_NamesPendingSubmitters(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	mw, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID: tg.Game.ID, PlanType: model.PlanMakeWar, Category: model.CategoryPower,
		PreparerID: tg.Players[0].ID, RowNumber: nil, RowOrder: 0,
		PreparedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	reveal, err := q.CreateSimultaneousReveal(ctx, dbgen.CreateSimultaneousRevealParams{
		GameID: tg.Game.ID, PlanID: &mw.ID, RevealType: revealTypeMakeWarDelay,
	})
	require.NoError(t, err)
	for _, p := range tg.Players {
		require.NoError(t, q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
			RevealID: reveal.ID, PlayerID: p.ID,
		}))
	}

	resData := loadResolutionData(mw.ResolutionData)
	resData.EnsureMakeWar().DelayRevealID = &reveal.ID
	require.NoError(t, saveResolutionData(ctx, q, mw.ID, resData))

	// Nobody has submitted yet — everyone is pending.
	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStateAwaitDelayReveal, state.Kind)
	assert.ElementsMatch(t,
		[]int64{tg.Players[0].ID, tg.Players[1].ID, tg.Players[2].ID}, state.ActingPlayerIDs)

	// players[0] submits — drops out of the acting set.
	face := int16(3)
	require.NoError(t, q.SetRevealEntryFace(ctx, dbgen.SetRevealEntryFaceParams{
		RevealID: reveal.ID, PlayerID: tg.Players[0].ID, Face: &face,
	}))
	state, err = ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{tg.Players[1].ID, tg.Players[2].ID}, state.ActingPlayerIDs,
		"the submitter drops out of the pending set")
}

// ── War-gate ActingPlayerIDs union across every war/claim ───────────────────

func TestComputeRowState_AwaitBattleCost_UnionsAcrossWars(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 4)
	ctx := context.Background()

	require.NoError(t, q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: tg.Game.ID, CurrentRow: 2,
	}))
	tg.Game.CurrentRow = 2

	makeActiveWar := func(declarer, enemy dbgen.Player) dbgen.War {
		origin := createPlanOnRow(t, q, &tg.Game, &declarer,
			model.PlanMakeWar, model.CategoryPower, 1)
		require.NoError(t, q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID: origin.ID, Status: model.PlanResolved,
		}))
		row1 := int16(1)
		require.NoError(t, q.SetPlanRowAndOrder(ctx, dbgen.SetPlanRowAndOrderParams{
			ID: origin.ID, RowNumber: &row1, RowOrder: 0,
		}))
		war, err := q.CreateWar(ctx, dbgen.CreateWarParams{
			GameID: tg.Game.ID, OriginPlanID: origin.ID, StartedAtRow: 1,
		})
		require.NoError(t, err)
		sides := map[int64]int16{declarer.ID: gamepkg.WarSideDeclarer, enemy.ID: gamepkg.WarSideEnemy}
		for _, p := range []dbgen.Player{declarer, enemy} {
			require.NoError(t, q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
				WarID: war.ID, PlayerID: p.ID, Side: sides[p.ID], JoinedAtRow: 1,
			}))
			require.NoError(t, q.SetWarParticipantEntryComplete(ctx, dbgen.SetWarParticipantEntryCompleteParams{
				WarID: war.ID, PlayerID: p.ID,
			}))
		}
		return war
	}

	warA := makeActiveWar(tg.Players[0], tg.Players[1])
	warB := makeActiveWar(tg.Players[2], tg.Players[3])

	outstanding, err := mwOutstandingCostsForGame(ctx, q, tg.Game.ID, tg.Game.CurrentRow)
	require.NoError(t, err)
	require.Contains(t, outstanding, warA.ID, "war A must owe a cost for this scenario to be meaningful")
	require.Contains(t, outstanding, warB.ID, "war B must owe a cost for this scenario to be meaningful")
	wantPayers := uniquePayerIDs(outstanding)

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStateAwaitBattleCost, state.Kind)
	assert.ElementsMatch(t, wantPayers, state.ActingPlayerIDs,
		"ActingPlayerIDs must union payers across every war with an outstanding cost, not just state.WarID's war")
}

func TestComputeRowState_AwaitSurrenderClaim_UnionsClaimants(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	originPlan := createPlanOnRow(t, q, &tg.Game, &tg.Players[0],
		model.PlanMakeWar, model.CategoryPower, tg.Game.CurrentRow)
	war, err := q.CreateWar(ctx, dbgen.CreateWarParams{
		GameID: tg.Game.ID, OriginPlanID: originPlan.ID, StartedAtRow: tg.Game.CurrentRow,
	})
	require.NoError(t, err)

	// players[0] surrendered; both players[1] and players[2] have open claims.
	require.NoError(t, q.CreateSurrenderClaim(ctx, dbgen.CreateSurrenderClaimParams{
		WarID: war.ID, SurrenderedID: tg.Players[0].ID, ClaimantID: tg.Players[1].ID,
	}))
	require.NoError(t, q.CreateSurrenderClaim(ctx, dbgen.CreateSurrenderClaimParams{
		WarID: war.ID, SurrenderedID: tg.Players[0].ID, ClaimantID: tg.Players[2].ID,
	}))

	state, err := ComputeRowState(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.Equal(t, model.RowStateAwaitSurrenderClaim, state.Kind)
	assert.ElementsMatch(t, []int64{tg.Players[1].ID, tg.Players[2].ID}, state.ActingPlayerIDs,
		"ActingPlayerIDs must union claimants across every open claim")
}
