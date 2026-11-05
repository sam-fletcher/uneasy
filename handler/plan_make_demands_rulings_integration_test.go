//go:build integration

// handler/plan_make_demands_rulings_integration_test.go — the owner's
// 2026-11-03 Make Demands rulings: the keep-or-change target option is live only
// on the plans that have a "who", and on a duel or a liaison its aim is settled
// before the first step.

package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// A duel has no roll until its bouts end, so the old "closes when the roll
// lands" guard left /demand-retarget open for the whole duel. Re-aiming the
// opponent after stakes are down must be refused.
func TestMakeDemandsRulings_DuelOpponentCannotChangeMidBouts(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	winnerIdx := (prepIdx + 2) % 5
	newOppIdx := (prepIdx + 3) % 5

	prepPeer := h.seedPeer(prepIdx, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")
	planID, _ := seedDuelToBouts(t, h, targIdx, []int64{prepPeer}, []int64{targPeer})

	h.seedMadeDemand(winnerIdx, planID, game.DemandOptionWinners{
		game.DemandOptionKeepOrChangeTarget: h.tg.Players[winnerIdx].ID,
	})

	code, body := h.post(winnerIdx, demandRetargetPath(planID), map[string]any{
		"target_player_id": h.tg.Players[newOppIdx].ID,
	})
	assert.Equalf(t, http.StatusConflict, code, "mid-bouts re-aim must be refused: %v", body)

	plan, err := h.q.GetPlanByID(t.Context(), planID)
	require.NoError(t, err)
	require.NotNil(t, plan.TargetPlayerID)
	assert.Equal(t, h.tg.Players[targIdx].ID, *plan.TargetPlayerID, "the duel's opponent must not have changed")
	assert.Equal(t, model.PlanProposeDuel, plan.PlanType)
}

// seedDuelResolving prepares a duel by the focus player, hands keep_or_change_
// target to winnerIdx, and resolves the plan so it sits in setup.
func seedDuelResolving(t *testing.T, h *planLifecycle, targIdx, winnerIdx int) (planID int64, prepIdx int) {
	t.Helper()
	prepIdx = h.focusPlayerIdx()
	targetID := h.tg.Players[targIdx].ID
	notes := "Courtyard duel at dawn"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanProposeDuel, TargetPlayerID: &targetID, DuelType: "arms", PreparationNotes: &notes,
	})
	pinEsteemRank(t, h, targetID, 5)
	pinEsteemRank(t, h, h.tg.Players[prepIdx].ID, 1)
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionKeepOrChangeTarget: h.tg.Players[winnerIdx].ID,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	return plan.ID, prepIdx
}

// A duel waits for the Make Demands winner to settle the opponent before its first step; the
// bar names them, the first-step routes refuse everyone, and once they have
// settled it the retarget route closes for good.
func TestMakeDemandsRulings_DuelWaitsForTheAimThenClosesIt(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	winnerIdx := (prepIdx + 2) % 5
	newOppIdx := (prepIdx + 3) % 5
	winnerID := h.tg.Players[winnerIdx].ID

	planID, _ := seedDuelResolving(t, h, targIdx, winnerIdx)
	h.assertWaitees("the winner settles the aim first", model.RowStateAwaitDemandRetarget, winnerID)

	for _, idx := range []int{prepIdx, targIdx} {
		code, body := h.post(idx, duelRoute(planID, "elect-champion"), map[string]any{"asset_id": nil})
		assert.Equalf(t, http.StatusConflict, code, "the first step is held until the aim is settled: %v", body)
	}

	// Re-aim: the opponent changes, and who declares first follows the new opponent.
	pinEsteemRank(t, h, h.tg.Players[newOppIdx].ID, 1)
	code, body := h.post(winnerIdx, demandRetargetPath(planID), map[string]any{"target_player_id": h.tg.Players[newOppIdx].ID})
	require.Equalf(t, http.StatusOK, code, "re-aim: %v", body)
	assert.Equal(t, h.tg.Players[newOppIdx].ID, duelInitiative(t, h, planID),
		"initiative was computed for the old opponent and must follow the new one")

	// Settled: the form's route closes, and the duel's first step opens.
	code, body = h.post(winnerIdx, demandRetargetPath(planID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusConflict, code, "once settled the aim cannot change again: %v", body)
	for _, idx := range []int{prepIdx, newOppIdx} {
		code, body = h.post(idx, duelRoute(planID, "elect-champion"), map[string]any{"asset_id": nil})
		require.Equalf(t, http.StatusOK, code, "the first step opens: %v", body)
	}
	// And a Make Demands winner who never finalised can't reach back in once the duel is under way.
}

// Doing nothing is not a decision the Make Demands winner can skip: keeping is a call too.
func TestMakeDemandsRulings_DuelKeepReleasesTheHold(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	winnerIdx := (prepIdx + 2) % 5
	planID, _ := seedDuelResolving(t, h, targIdx, winnerIdx)

	code, body := h.post(winnerIdx, demandRetargetPath(planID), map[string]any{"keep": true})
	require.Equalf(t, http.StatusOK, code, "keep: %v", body)
	plan, err := h.q.GetPlanByID(t.Context(), planID)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targIdx].ID, *plan.TargetPlayerID, "keep leaves the opponent alone")
	for _, idx := range []int{prepIdx, targIdx} {
		code, body = h.post(idx, duelRoute(planID, "elect-champion"), map[string]any{"asset_id": nil})
		require.Equalf(t, http.StatusOK, code, "the first step opens after keep: %v", body)
	}
}

// A preparer who wins the option re-aims freely, with no hold, and doing
// nothing means keep. Pinned on a duel: no wait on them, the duel starts, and
// the route shuts once the first step has been taken.
func TestMakeDemandsRulings_PreparerWinnerIsNeverHeldAndDoingNothingKeeps(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	newOppIdx := (prepIdx + 3) % 5
	planID, _ := seedDuelResolving(t, h, targIdx, prepIdx)

	plan, err := h.q.GetPlanByID(t.Context(), planID)
	require.NoError(t, err)
	assert.Zero(t, pendingDemandRetargetChooser(t.Context(), h.q, &plan), "a preparer winner is never made to wait on themselves")

	// Re-aim freely, with no prior finalize.
	code, body := h.post(prepIdx, demandRetargetPath(planID), map[string]any{"target_player_id": h.tg.Players[newOppIdx].ID})
	require.Equalf(t, http.StatusOK, code, "the preparer re-aims freely: %v", body)
	// Doing nothing means keep: the duel starts without any retarget call.
	for _, idx := range []int{prepIdx, newOppIdx} {
		code, body = h.post(idx, duelRoute(planID, "elect-champion"), map[string]any{"asset_id": nil})
		require.Equalf(t, http.StatusOK, code, "the duel starts without a keep call: %v", body)
	}
	code, body = h.post(prepIdx, demandRetargetPath(planID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusConflict, code, "the first step has begun, so the aim is closed: %v", body)
}

// On a plan with a roll the preparer who wins it holds nothing either.
func TestMakeDemandsRulings_PreparerWinnerDoesNotHoldARoll(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	preparerIdx := h.focusPlayerIdx()
	subject := h.mainCharacterID((preparerIdx + 1) % 3)
	notes := "a rumor"
	plan := h.prepare(PreparePlanRequest{PlanType: model.PlanSpreadRumors, TargetAssetID: &subject, PreparationNotes: &notes})
	h.seedMadeDemand((preparerIdx+1)%3, plan.ID, game.DemandOptionWinners{
		game.DemandOptionKeepOrChangeTarget: h.tg.Players[preparerIdx].ID,
	})
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolveAndCast(plan.ID)
	require.NotNil(t, roll)

	p, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Empty(t, pendingDemandRollBlockers(ctx, h.q, &p), "no hold on the roll for a preparer winner")
}

// On every plan outside the four with a "who", the option is a plain dud: no
// roll-hold and the route refuses.
func TestMakeDemandsRulings_KeepOrChangeIsADudOffTheFourPlans(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	plan, roll, _, winnerIdx, _ := setupDemandPreRollWindow(t, h, game.DemandOptionKeepOrChangeTarget)
	winnerID := h.tg.Players[winnerIdx].ID

	wp, err := h.q.GetParticipant(ctx, dbgen.GetParticipantParams{RollID: roll.ID, PlayerID: winnerID})
	require.NoError(t, err)
	assert.True(t, wp.IsReady, "a dud winner must not be seeded unready")
	rl, err := h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	assert.False(t, rollIsOpen(&rl), "nobody holds a Spread Propaganda roll for a dud option")

	code, body := h.post(winnerIdx, demandRetargetPath(plan.ID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusConflict, code, "the route refuses on a dud plan: %v", body)
	assert.Contains(t, body["error"], "inert")
}

func TestMakeDemandsRulings_LiaiseWaitsForTheAimThenClosesIt(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's confidant")
	notes := "a meeting under the bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanClandestinelyLiaise, TargetPlayerID: &partnerID,
		PreparerPeerID: &preparerPeer, PartnerPeerID: &partnerPeer, PreparationNotes: &notes,
	})
	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	clSubmitReveal(t, h, *ld.DelayRevealID, 0, 2)
	clSubmitReveal(t, h, *ld.DelayRevealID, 1, 2)
	winnerIdx := 2
	winnerID := h.tg.Players[winnerIdx].ID
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{game.DemandOptionKeepOrChangeTarget: winnerID})
	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	h.jumpToRow(*refreshed.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	h.assertWaitees("the winner settles the aim first", model.RowStateAwaitDemandRetarget, winnerID)
	code, body := h.post(0, clPath(plan.ID, "advance-liaise"), nil)
	assert.Equalf(t, http.StatusConflict, code, "the first step is held: %v", body)

	code, body = h.post(winnerIdx, demandRetargetPath(plan.ID), map[string]any{"keep": true})
	require.Equalf(t, http.StatusOK, code, "keep: %v", body)
	code, body = h.post(winnerIdx, demandRetargetPath(plan.ID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusConflict, code, "settled: the route closes: %v", body)
	code, body = h.post(0, clPath(plan.ID, "advance-liaise"), nil)
	require.Equalf(t, http.StatusOK, code, "the first step opens: %v", body)
}

// A duel or liaison that has been prepared but has not started resolving has no
// Phase yet. The Make Demands winner must still be able to settle its aim then.
func TestMakeDemandsRulings_PendingDuelAndLiaisonAimStaysOpen(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	winnerIdx := (prepIdx + 2) % 5
	targetID := h.tg.Players[targIdx].ID
	notes := "Courtyard duel at dawn"
	duel := h.prepare(PreparePlanRequest{
		PlanType: model.PlanProposeDuel, TargetPlayerID: &targetID, DuelType: "arms", PreparationNotes: &notes,
	})
	h.seedMadeDemand(winnerIdx, duel.ID, game.DemandOptionWinners{
		game.DemandOptionKeepOrChangeTarget: h.tg.Players[winnerIdx].ID,
	})
	code, body := h.post(winnerIdx, demandRetargetPath(duel.ID),
		map[string]any{"target_player_id": h.tg.Players[(prepIdx+3)%5].ID})
	assert.Equalf(t, http.StatusOK, code, "re-aim a pending duel: %v", body)
}

func TestMakeDemandsRulings_PendingLiaisonKeepStaysOpen(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's confidant")
	notes := "a meeting under the bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanClandestinelyLiaise, TargetPlayerID: &partnerID,
		PreparerPeerID: &preparerPeer, PartnerPeerID: &partnerPeer, PreparationNotes: &notes,
	})
	h.seedMadeDemand(2, plan.ID, game.DemandOptionWinners{game.DemandOptionKeepOrChangeTarget: h.tg.Players[2].ID})
	code, body := h.post(2, demandRetargetPath(plan.ID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusOK, code, "keep on a pending liaison: %v", body)
}

// A game in flight when this shipped can hold a duel in its bouts, or a liaison
// past its first step, with a Make Demands winner who never settled the aim. The
// plan must carry on rather than wait on a call the route now refuses.
func TestMakeDemandsRulings_DuelUnderWayIsNotHeldHostage(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	winnerIdx := (prepIdx + 2) % 5
	prepPeer := h.seedPeer(prepIdx, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")
	planID, _ := seedDuelToBouts(t, h, targIdx, []int64{prepPeer}, []int64{targPeer})
	h.seedMadeDemand(winnerIdx, planID, game.DemandOptionWinners{
		game.DemandOptionKeepOrChangeTarget: h.tg.Players[winnerIdx].ID,
	})
	plan, err := h.q.GetPlanByID(t.Context(), planID)
	require.NoError(t, err)
	assert.Zero(t, pendingDemandRetargetChooser(t.Context(), h.q, &plan), "a duel in its bouts waits on nobody's aim")
	runDuelBouts(t, h, planID, prepIdx, targIdx) // play continues
}

func TestMakeDemandsRulings_LiaisonUnderWayIsNotHeldHostage(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clPrepareAndResolve(t, h)
	clAdvance(t, h, m.plan.ID) // together_at_last → secrets_we_keep
	h.seedMadeDemand(2, m.plan.ID, game.DemandOptionWinners{game.DemandOptionKeepOrChangeTarget: h.tg.Players[2].ID})
	plan, err := h.q.GetPlanByID(t.Context(), m.plan.ID)
	require.NoError(t, err)
	assert.Zero(t, pendingDemandRetargetChooser(t.Context(), h.q, &plan), "a liaison past its first step waits on nobody's aim")
	clKeepSecret(t, h, m.plan.ID, 0) // play continues
}

// Once a duellist has staked, the opponent can't change: the stake would be stranded.
func TestMakeDemandsRulings_StakesLockTheAim(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	newOppIdx := (prepIdx + 3) % 5
	planID, _ := seedDuelResolving(t, h, targIdx, prepIdx) // the preparer holds the option
	prepPeer := h.seedPeer(prepIdx, "Preparer stake")
	code, body := h.post(prepIdx, duelRoute(planID, "select-stakes"), map[string]any{"asset_ids": []int64{prepPeer}})
	require.Equalf(t, http.StatusOK, code, "stake: %v", body)
	code, body = h.post(prepIdx, demandRetargetPath(planID), map[string]any{"target_player_id": h.tg.Players[newOppIdx].ID})
	assert.Equalf(t, http.StatusConflict, code, "re-aim after a stake is down: %v", body)
}
