//go:build integration

// handler/plan_make_demands_leverage_integration_test.go — end-to-end coverage
// for the two Make Demands PRE-ROLL option windows (/demand-leverage and
// /demand-retarget) and, crucially, their TIMING.
//
// A made demand's control_leverage winner decides how many of the target
// preparer's assets are leveraged onto the target plan's roll — including none,
// to deliberately guarantee its failure. Its keep_or_change_target winner
// decides where that plan is aimed — including keeping it exactly where it is.
// Each is a real decision with a do-nothing outcome that looks identical to
// having never acted, so each must be explicitly finalized, and the roll must
// WAIT for both — even when a winner has no dice of their own to commit (the
// failure mode the gates fix: such a winner used to be auto-readied at seed and
// the roll resolved without them; audit D5). These tests drive a real Spread
// Propaganda roll through its stage machine to assert the wait, the subset
// leverage, the "leverage none" finalize, and the "keep the target" finalize —
// plus the roll-less targets where control_leverage is inert (audit D7).

package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// rollPath builds an /api/rolls/{id}/{action} URL for the harness router.
func rollPath(rollID int64, action string) string {
	return "/api/rolls/" + strconv.FormatInt(rollID, 10) + "/" + action
}

// demandLeveragePath builds the target plan's /demand-leverage route.
func demandLeveragePath(planID int64) string {
	return "/api/plans/" + strconv.FormatInt(planID, 10) + "/demand-leverage"
}

// demandRetargetPath builds the target plan's /demand-retarget route.
func demandRetargetPath(planID int64) string {
	return "/api/plans/" + strconv.FormatInt(planID, 10) + "/demand-retarget"
}

// setupDemandPreRollWindow prepares a Spread Propaganda plan by the focus
// player, seeds a resolved made demand whose `option` winner is a DIFFERENT
// player stripped of their own committable dice, resolves the plan to open its
// roll, and advances that roll to the leverage stage with every non-winner
// participant readied. It returns the plan, the still-open roll, and the player
// indices. After it returns, the only thing keeping the roll open is the
// winner's outstanding decision.
//
// Both pre-roll options — control_leverage and keep_or_change_target — hold the
// roll the same way and are driven through this same setup.
func setupDemandPreRollWindow(
	t *testing.T, h *planLifecycle, option string,
) (plan dbgen.Plan, roll dbgen.DiceRoll, preparerIdx, winnerIdx, thirdIdx int) {
	t.Helper()
	ctx := context.Background()
	preparerIdx = h.focusPlayerIdx()
	winnerIdx = (preparerIdx + 1) % 3
	thirdIdx = (preparerIdx + 2) % 3

	notes := "test propaganda"
	plan = h.prepare(PreparePlanRequest{PlanType: model.PlanSpreadPropaganda, PreparationNotes: &notes})
	require.NotNil(t, plan.RowNumber)

	// A resolved, made demand hands the option to the winner. Seeded BEFORE the
	// roll is created so seedRollParticipants sees the winner and seeds them
	// unready regardless of their own dice.
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		option: h.tg.Players[winnerIdx].ID,
	})

	// Strip the winner's own committable dice (leverage their assets) so, absent
	// the gate, they'd be auto-readied at seed and the roll would resolve without
	// them (the failure mode this fix closes). Leveraging — not destroying —
	// keeps their main character intact so the main-character gate stays quiet.
	winnerAssets, err := h.q.ListAssetsByOwner(ctx, h.tg.Players[winnerIdx].ID)
	require.NoError(t, err)
	for _, a := range winnerAssets {
		require.NoError(t, h.q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID: a.ID, IsLeveraged: true,
		}))
	}

	h.jumpToRow(*plan.RowNumber)
	r := h.resolve(plan.ID)
	require.NotNil(t, r, "Spread Propaganda creates its roll on resolve")
	roll = *r

	// Advance to the leverage stage and ready everyone except the winner.
	code, body := h.post(preparerIdx, rollPath(roll.ID, "skip-vote"), nil)
	require.Equalf(t, http.StatusOK, code, "skip-vote: %v", body)
	for _, idx := range []int{preparerIdx, thirdIdx} {
		code, body = h.post(idx, rollPath(roll.ID, "ready"), map[string]any{"is_ready": true})
		require.Equalf(t, http.StatusOK, code, "ready idx %d: %v", idx, body)
	}
	return plan, roll, preparerIdx, winnerIdx, thirdIdx
}

// TestPlanLifecycle_MakeDemands_ControlLeverage_WinnerSetsSubset asserts the
// roll WAITS for the control_leverage winner (seeded unready despite no own
// dice; row-state names them), then resolves once the winner leverages a subset
// of the preparer's assets.
func TestPlanLifecycle_MakeDemands_ControlLeverage_WinnerSetsSubset(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, roll, preparerIdx, winnerIdx, _ := setupDemandPreRollWindow(t, h, game.DemandOptionControlLeverage)

	// The winner is seeded unready even with no dice of their own — the gate holds.
	wp, err := h.q.GetParticipant(ctx, dbgen.GetParticipantParams{
		RollID: roll.ID, PlayerID: h.tg.Players[winnerIdx].ID,
	})
	require.NoError(t, err)
	assert.False(t, wp.IsReady, "control_leverage winner must be seeded unready")

	// The waiting bar names the winner during the window. While the roll is
	// open, the top-of-chain await_dice_roll gate (Notifications Session 1)
	// supersedes await_demand_leverage — the winner is seeded unready
	// specifically so they still land in the roll's own unready-participants
	// acting set, matching what the client's roll override already showed
	// before that gate moved server-side.
	h.assertWaitees("leverage window", model.RowStateAwaitDiceRoll, h.tg.Players[winnerIdx].ID)

	// The roll has NOT resolved — every other participant is ready, but it waits.
	rl, err := h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	require.True(t, rollIsOpen(&rl), "roll must wait for the control_leverage winner")

	// A fresh preparer asset for the winner to leverage onto the roll.
	pAsset := h.seedPeer(preparerIdx, "Preparer leverage target")

	// Winner leverages a subset (this one asset) — this finalizes the decision.
	code, body := h.post(winnerIdx, demandLeveragePath(plan.ID),
		map[string]any{"asset_ids": []int64{pAsset}})
	require.Equalf(t, http.StatusOK, code, "demand-leverage subset: %v", body)

	// The chosen asset is now leveraged on the preparer's behalf.
	leveraged, err := h.q.GetAssetByID(ctx, pAsset)
	require.NoError(t, err)
	assert.True(t, leveraged.IsLeveraged, "winner's chosen asset must be leveraged")

	// The finalize flag is set and, with the winner now readied, the roll resolves.
	planAfter, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.True(t, loadResolutionData(planAfter.ResolutionData).DemandLeverageFinalized,
		"finalize flag must be set")
	rl, err = h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	assert.False(t, rollIsOpen(&rl), "roll resolves once the winner finalizes")
}

// TestPlanLifecycle_MakeDemands_ControlLeverage_WinnerLeveragesNone asserts the
// deliberate "leverage none" path: the winner finalizes with an empty asset
// list, no preparer asset is leveraged, the flag flips, and the roll still
// resolves (the winner stops blocking it).
func TestPlanLifecycle_MakeDemands_ControlLeverage_WinnerLeveragesNone(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, roll, preparerIdx, winnerIdx, _ := setupDemandPreRollWindow(t, h, game.DemandOptionControlLeverage)

	// Snapshot the preparer's asset leverage states before the finalize.
	before, err := h.q.ListAssetsByOwner(ctx, h.tg.Players[preparerIdx].ID)
	require.NoError(t, err)
	beforeLeveraged := map[int64]bool{}
	for _, a := range before {
		beforeLeveraged[a.ID] = a.IsLeveraged
	}

	// Roll still open before the winner acts.
	rl, err := h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	require.True(t, rollIsOpen(&rl), "roll waits for the winner")

	// Winner finalizes with an empty list — deliberately leverages none.
	code, body := h.post(winnerIdx, demandLeveragePath(plan.ID),
		map[string]any{"asset_ids": []int64{}})
	require.Equalf(t, http.StatusOK, code, "demand-leverage none: %v", body)

	// No preparer asset's leverage state changed.
	after, err := h.q.ListAssetsByOwner(ctx, h.tg.Players[preparerIdx].ID)
	require.NoError(t, err)
	for _, a := range after {
		if was, ok := beforeLeveraged[a.ID]; ok {
			assert.Equalf(t, was, a.IsLeveraged,
				"asset %d leverage state must be unchanged by 'leverage none'", a.ID)
		}
	}

	// The flag is set and the roll resolves even though no leverage was added.
	planAfter, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.True(t, loadResolutionData(planAfter.ResolutionData).DemandLeverageFinalized,
		"finalize flag must be set on 'leverage none'")
	rl, err = h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	assert.False(t, rollIsOpen(&rl), "roll resolves after a 'leverage none' finalize")
}

// ── keep_or_change_target (audit D2, D5) ──────────────────────────────────────

// TestPlanLifecycle_MakeDemands_Retarget_KeepHoldsThenReleasesTheRoll is D5's
// core assertion: the keep_or_change_target winner holds the roll open exactly
// as the leverage winner does, is named while they do, and releases it by
// submitting "keep the current target".
//
// Before the fix this option had no done-state and no seat in the roll's
// participant set at all, so on a target whose roll opens at kickoff (this
// Spread Propaganda, and equally Spread Rumors or Propose Decree) the roll could
// auto-resolve and close the window before the winner ever acted. "Keep" is the
// case that proves it: it writes nothing to the plan's target columns, so
// without an explicit finalize there is no way to tell it from silence.
func TestPlanLifecycle_MakeDemands_Retarget_KeepHoldsThenReleasesTheRoll(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, roll, _, winnerIdx, _ := setupDemandPreRollWindow(t, h, game.DemandOptionKeepOrChangeTarget)
	winnerID := h.tg.Players[winnerIdx].ID

	// Seeded unready despite having no dice of their own — the gate holds.
	wp, err := h.q.GetParticipant(ctx, dbgen.GetParticipantParams{
		RollID: roll.ID, PlayerID: winnerID,
	})
	require.NoError(t, err)
	assert.False(t, wp.IsReady, "keep_or_change_target winner must be seeded unready")

	rl, err := h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	require.True(t, rollIsOpen(&rl), "roll must wait for the keep_or_change_target winner")

	// While the roll is open the top-of-chain await_dice_roll gate supersedes
	// await_demand_retarget — the winner is seeded unready precisely so they land
	// in the roll's own unready-participant acting set either way.
	h.assertWaitees("retarget window", model.RowStateAwaitDiceRoll, winnerID)

	// The narrower kind underneath it names the same player, which is what the
	// bar falls back to in the gap before/after the roll is open.
	plans, err := h.q.ListPlansByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	rs, ok := planResolvingGate(ctx, h.q, plans)
	require.True(t, ok, "the target plan is resolving")
	assert.Equal(t, model.RowStateAwaitDemandRetarget, rs.Kind)
	assert.Equal(t, []int64{winnerID}, rs.ActingPlayerIDs)

	// The winner keeps the target — a submission, not silence.
	code, body := h.post(winnerIdx, demandRetargetPath(plan.ID), map[string]any{"keep": true})
	require.Equalf(t, http.StatusOK, code, "demand-retarget keep: %v", body)
	assert.Equal(t, true, body["kept"])

	planAfter, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.True(t, loadResolutionData(planAfter.ResolutionData).DemandRetargetFinalized,
		"finalize flag must be set by a 'keep' submission")
	rl, err = h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	assert.False(t, rollIsOpen(&rl), "roll resolves once the winner settles the target")

	// Even a purely narrative outcome gets its beat in the action log.
	posts := mdSystemPosts(t, h.q, h.tg.Game.ID)
	found := false
	for _, b := range posts {
		if strings.Contains(b, "stand exactly as aimed") {
			found = true
		}
	}
	assert.Truef(t, found, "a kept target must still be logged; got %v", posts)
}

// TestPlanLifecycle_MakeDemands_Retarget_TargetWithNoRollYet is audit D2: the
// route used to treat GetDiceRollByPlanID's empty result as an internal error
// and 500, which is every target during the window the winner would naturally
// act in — the target is still 'pending' then, and 7 of the 10 legal target
// types have no roll at that point anyway.
//
// It also pins the validation context: the re-aim replays the plan's own
// declared content (here Spread Rumors' rumor note), without which the target
// type's ValidatePreparation rejects the plan's own settled state — and Spread
// Rumors is the rulebook's worked example for this very option.
func TestPlanLifecycle_MakeDemands_Retarget_TargetWithNoRollYet(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	preparerIdx := h.focusPlayerIdx()
	winnerIdx := (preparerIdx + 1) % 3
	thirdIdx := (preparerIdx + 2) % 3

	// A Spread Rumors aimed at the winner's main character, prepared normally so
	// it sits pending on a future row with no roll of any kind.
	notes := "a rumor about the wine"
	firstTarget := h.mainCharacterID(winnerIdx)
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &firstTarget,
		PreparationNotes: &notes,
	})
	require.Equal(t, model.PlanPending, plan.Status)
	_, errRoll := h.q.GetDiceRollByPlanID(ctx, &plan.ID)
	require.Error(t, errRoll, "a pending Spread Rumors has no roll yet")

	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionKeepOrChangeTarget: h.tg.Players[winnerIdx].ID,
	})

	// Re-aim the rumor at the third player's main character instead.
	newTarget := h.mainCharacterID(thirdIdx)
	code, body := h.post(winnerIdx, demandRetargetPath(plan.ID), map[string]any{
		"target_player_id": h.tg.Players[thirdIdx].ID,
		"target_asset_id":  newTarget,
	})
	require.Equalf(t, http.StatusOK, code, "retarget against a roll-less pending plan: %v", body)
	assert.Equal(t, false, body["kept"])

	planAfter, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, planAfter.TargetAssetID)
	assert.Equal(t, newTarget, *planAfter.TargetAssetID, "the rumor now points at a different character")
	assert.True(t, loadResolutionData(planAfter.ResolutionData).DemandRetargetFinalized,
		"a re-aim finalizes the window too")
}

// ── control_leverage against a roll-less target (audit D7) ────────────────────

// TestPlanLifecycle_MakeDemands_ControlLeverage_InertAgainstFestivity pins D7 on
// the target that misfired worst. Every roll carrying a Host Festivity's plan_id
// belongs to a GUEST — the host never rolls — and GetDiceRollByPlanID just takes
// the most recent one. So a control_leverage winner used to block the first
// guest's roll, /demand-leverage would have spent the HOST's assets into that
// guest's roll as interference, and the lockout meanwhile stripped the host of
// their own assets on it.
//
// The option is now inert here, which the rules allow ("they'll each have a
// different impact depending on which plan is being targeted"). Inert means all
// three: an explanatory 409, no unready seed, and no lockout.
func TestPlanLifecycle_MakeDemands_ControlLeverage_InertAgainstFestivity(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, hostIdx := hfPrepareToSocializing(t, h)
	winnerIdx := (hostIdx + 1) % 3
	guestIdx := (hostIdx + 2) % 3
	winnerID := h.tg.Players[winnerIdx].ID

	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionControlLeverage: winnerID,
	})

	// Strip the winner's own committable dice, so if the blocker gate fired at all
	// it would be the only reason they were seeded unready below.
	winnerAssets, err := h.q.ListAssetsByOwner(ctx, winnerID)
	require.NoError(t, err)
	for _, a := range winnerAssets {
		require.NoError(t, h.q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
			ID: a.ID, IsLeveraged: true,
		}))
	}

	// 1. The route names the dud rather than reaching for a guest's roll.
	code, body := h.post(winnerIdx, demandLeveragePath(plan.ID),
		map[string]any{"asset_ids": []int64{}})
	require.Equalf(t, http.StatusConflict, code, "demand-leverage against a festivity: %v", body)
	assert.Contains(t, body["error"], "inert")

	// 2. A guest's roll is not held hostage — the winner seeds ready as normal.
	rollID := hfStartRoll(t, h, plan.ID, guestIdx)
	part, err := h.q.GetParticipant(ctx, dbgen.GetParticipantParams{
		RollID: rollID, PlayerID: winnerID,
	})
	require.NoError(t, err)
	assert.True(t, part.IsReady,
		"an inert control_leverage winner must not block a guest's roll")

	// 3. The host keeps control of their own assets on that roll: leverageRoll's
	// demand-winner lockout must stand down too, or the festivity's assets would
	// be frozen for everyone.
	code, body = h.post(hostIdx, rollPath(rollID, "intent"), map[string]any{"intent": "aid"})
	require.Equalf(t, http.StatusOK, code, "host sets intent on the guest's roll: %v", body)
	hostAsset := h.seedPeer(hostIdx, "Host's own stake")
	code, body = h.post(hostIdx, rollPath(rollID, "leverage"), map[string]any{"asset_id": hostAsset})
	require.Equalf(t, http.StatusOK, code, "host leverages their own asset: %v", body)
}
