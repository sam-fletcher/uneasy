//go:build integration

// handler/plan_make_demands_leverage_integration_test.go — end-to-end coverage
// for the Make Demands control_leverage window (the /demand-leverage route and,
// crucially, its TIMING).
//
// A made demand's control_leverage winner gets to decide how many of the target
// preparer's assets are leveraged onto the target plan's roll — including none,
// to deliberately guarantee its failure. The roll must therefore WAIT for that
// winner before resolving, even when the winner has no dice of their own to
// commit (the failure mode the gate fixes: such a winner used to be auto-readied
// at seed and the roll resolved without them). These tests drive a real Spread
// Propaganda roll through its stage machine to assert the wait, the subset
// leverage, and the "leverage none" finalize.

package handler

import (
	"context"
	"net/http"
	"strconv"
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

// setupControlLeverageWindow prepares a Spread Propaganda plan by the focus
// player, seeds a resolved made demand whose control_leverage winner is a
// DIFFERENT player stripped of their own committable dice, resolves the plan to
// open its roll, and advances that roll to the leverage stage with every
// non-winner participant readied. It returns the plan, the still-open roll, and
// the player indices. After it returns, the only thing keeping the roll open is
// the winner's outstanding leverage decision.
func setupControlLeverageWindow(
	t *testing.T, h *planLifecycle,
) (plan dbgen.Plan, roll dbgen.DiceRoll, preparerIdx, winnerIdx, thirdIdx int) {
	t.Helper()
	ctx := context.Background()
	preparerIdx = h.focusPlayerIdx()
	winnerIdx = (preparerIdx + 1) % 3
	thirdIdx = (preparerIdx + 2) % 3

	notes := "test propaganda"
	plan = h.prepare(PreparePlanRequest{PlanType: model.PlanSpreadPropaganda, PreparationNotes: &notes})
	require.NotNil(t, plan.RowNumber)

	// A resolved, made demand hands control_leverage to the winner. Seeded
	// BEFORE the roll is created so seedRollParticipants sees the winner and
	// seeds them unready regardless of their own dice.
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionControlLeverage: h.tg.Players[winnerIdx].ID,
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
	plan, roll, preparerIdx, winnerIdx, _ := setupControlLeverageWindow(t, h)

	// The winner is seeded unready even with no dice of their own — the gate holds.
	wp, err := h.q.GetParticipant(ctx, dbgen.GetParticipantParams{
		RollID: roll.ID, PlayerID: h.tg.Players[winnerIdx].ID,
	})
	require.NoError(t, err)
	assert.False(t, wp.IsReady, "control_leverage winner must be seeded unready")

	// The waiting bar names the winner during the window.
	h.assertWaitees("leverage window", model.RowStateAwaitDemandLeverage, h.tg.Players[winnerIdx].ID)

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
	plan, roll, preparerIdx, winnerIdx, _ := setupControlLeverageWindow(t, h)

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
