//go:build integration

// handler/plan_exchange_courtiers_integration_test.go — mar-path coverage for
// Exchange Courtiers. The audit found EC's mar was a no-op; the rules make it
// target-driven:
//
//   - fair_trade → the trade goes through (targeted peer → preparer)
//   - forfeit    → the target claims one of the preparer's peers
//   - riposte    → like forfeit, but the preparer may break the peer first
//
// The make path + messy-break restriction live in plan_lifecycle_examples_test.go.

package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// ecToMar drives an EC plan to a marred roll: prepare (preparer = current
// focus), target the next player's seeded peer, jump to the row, decline the
// fair trade to force the dice roll, then mark it mar. Returns the plan and the
// preparer/target indices.
func ecToMar(t *testing.T, h *planLifecycle) (dbgen.Plan, int, int) {
	t.Helper()
	preparerIdx := h.focusPlayerIdx()
	targetIdx := (preparerIdx + 1) % len(h.tg.Players)
	targetPeer := h.seedPeer(targetIdx, "coveted peer")

	notes := "exchange"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[targetIdx].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "EC defers its roll behind the fair-trade step")

	declinePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/fair-trade"
	code, body := h.post(preparerIdx, declinePath, map[string]any{"action": "decline"})
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap, "decline should create a roll")
	h.forceRoll(int64(rollMap["id"].(float64)), "mar", 0)
	return plan, preparerIdx, targetIdx
}

// ecMakeChoice posts make-choice as a specific player (EC's mar choice is made
// by the target, who is a non-focus player).
func ecMakeChoice(t *testing.T, h *planLifecycle, playerIdx int, planID int64, choices []string) (int, map[string]any) {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/make-choice"
	return h.post(playerIdx, path, map[string]any{"result": "mar", "choices": choices})
}

func TestExchangeCourtiers_Mar_FairTrade_PeerStillPasses(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, _, targetIdx := ecToMar(t, h)
	require.NotNil(t, plan.TargetAssetID)

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"fair_trade"})
	require.Equalf(t, http.StatusOK, code, "target make-choice: %v", body)
	h.complete(plan.ID)

	// Despite the mar, the targeted peer ends up with the preparer.
	asset, err := h.q.GetAssetByID(ctx, *plan.TargetAssetID)
	require.NoError(t, err)
	assert.Equal(t, plan.PreparerID, asset.OwnerID)
}

func TestExchangeCourtiers_Mar_Forfeit_TargetClaimsPreparerPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, targetIdx := ecToMar(t, h)
	spoils := h.seedPeer(preparerIdx, "preparer's prize peer")

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"forfeit"})
	require.Equalf(t, http.StatusOK, code, "target make-choice: %v", body)

	// Completion is blocked until the target claims a peer.
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body = h.post(h.focusPlayerIdx(), completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked: %v", body)

	// A non-target player cannot claim.
	claimPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/claim-peer"
	code, _ = h.post(preparerIdx, claimPath, map[string]any{"asset_id": spoils})
	assert.Equal(t, http.StatusForbidden, code, "only the target may claim")

	// Target claims the preparer's peer.
	code, body = h.post(targetIdx, claimPath, map[string]any{"asset_id": spoils})
	require.Equalf(t, http.StatusOK, code, "claim-peer: %v", body)

	h.complete(plan.ID)

	claimed, err := h.q.GetAssetByID(ctx, spoils)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targetIdx].ID, claimed.OwnerID, "peer should now belong to the target")
}

func TestExchangeCourtiers_Mar_Riposte_PreparerBreaksThenTargetClaims(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, targetIdx := ecToMar(t, h)
	// Two marginalia so the riposte break damages but doesn't destroy the peer.
	peer := h.seedPeer(preparerIdx, "battered peer")
	m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: peer, Position: 1, Text: "a proud note",
	})
	require.NoError(t, err)
	_, err = h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: peer, Position: 2, Text: "a second note",
	})
	require.NoError(t, err)

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"riposte"})
	require.Equalf(t, http.StatusOK, code, "target make-choice: %v", body)

	// Preparer breaks the peer before it changes hands.
	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/riposte-break"
	code, body = h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": m.ID})
	require.Equalf(t, http.StatusOK, code, "riposte-break: %v", body)
	torn, err := h.q.GetMarginaliaByID(ctx, m.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "marginalium should be torn by the riposte break")

	// Target claims the (now-damaged) peer.
	claimPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/claim-peer"
	code, body = h.post(targetIdx, claimPath, map[string]any{"asset_id": peer})
	require.Equalf(t, http.StatusOK, code, "claim-peer: %v", body)

	h.complete(plan.ID)
	claimed, err := h.q.GetAssetByID(ctx, peer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targetIdx].ID, claimed.OwnerID)
}
