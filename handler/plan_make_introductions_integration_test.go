//go:build integration

// handler/plan_make_introductions_integration_test.go — per-peer mar coverage
// for Make Introductions. The audit found MI's mar was a flat no-op with a
// bogus "center" option; the rules resolve each introduced peer individually:
//
//   - other_retinue  → peer joins another player's retinue
//   - broken_arrival → another player authors the peer's marginalia
//   - delayed        → arrival rescheduled d6 rows ahead
//   - broken_journey → focus writes a marginalia, then breaks the peer
//
// The make path lives in plan_lifecycle_examples_test.go.

package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/model"
)

// miToMar prepares an MI plan with peerCount peers, names them, forces a mar
// roll, and enters per-peer resolution. Returns the plan, the preparer's index,
// and the created peer asset IDs.
func miToMar(t *testing.T, h *planLifecycle, peerCount int) (planID int64, preparerIdx int, peerIDs []int64) {
	t.Helper()
	preparerIdx = h.focusPlayerIdx()
	notes := "introductions"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeIntroductions,
		PeerCount:        int16(peerCount),
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "MI defers its roll until peers are named")

	createPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/create-peer"
	for i := 0; i < peerCount; i++ {
		code, body := h.post(preparerIdx, createPath, map[string]any{"name": "Newcomer " + strconv.Itoa(i)})
		require.Equalf(t, http.StatusCreated, code, "create-peer[%d]: %v", i, body)
	}
	finalizePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/finalize-peers"
	code, body := h.post(preparerIdx, finalizePath, nil)
	require.Equalf(t, http.StatusCreated, code, "finalize-peers: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap)
	h.forceRoll(int64(rollMap["id"].(float64)), "mar", 0)

	// Enter per-peer mar resolution (focus player records the mar result).
	h.makeChoice(plan.ID, "mar", []string{})

	refreshed, err := h.q.GetPlanByID(context.Background(), plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	require.NotNil(t, rd.MakeIntroductions)
	return plan.ID, preparerIdx, rd.MakeIntroductions.CreatedPeerIDs
}

func TestMakeIntroductions_Mar_OtherRetinue_AndGating(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	planID, preparerIdx, peers := miToMar(t, h, 2)
	require.Len(t, peers, 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"

	// Resolve the first peer into another player's retinue.
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"peer_asset_id": peers[0], "outcome": "other_retinue",
		"target_player_id": h.tg.Players[otherIdx].ID,
	})
	require.Equalf(t, http.StatusOK, code, "other_retinue: %v", body)

	// Completion is blocked: the second peer is unresolved.
	completePath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/complete"
	code, body = h.post(h.focusPlayerIdx(), completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked: %v", body)

	// Resolving the same peer twice is rejected.
	code, _ = h.post(preparerIdx, marPath, map[string]any{
		"peer_asset_id": peers[0], "outcome": "delayed",
	})
	assert.Equal(t, http.StatusConflict, code, "double-resolving a peer should 409")

	// Resolve the second peer (delayed) and complete.
	code, body = h.post(preparerIdx, marPath, map[string]any{
		"peer_asset_id": peers[1], "outcome": "delayed",
	})
	require.Equalf(t, http.StatusOK, code, "delayed: %v", body)
	h.complete(planID)

	moved, err := h.q.GetAssetByID(ctx, peers[0])
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[otherIdx].ID, moved.OwnerID, "peer should be in the other player's retinue")
}

func TestMakeIntroductions_Mar_BrokenArrival_AuthorWritesMarginalia(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	planID, preparerIdx, peers := miToMar(t, h, 1)
	authorIdx := (preparerIdx + 1) % len(h.tg.Players)

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"peer_asset_id": peers[0], "outcome": "broken_arrival",
		"target_player_id": h.tg.Players[authorIdx].ID,
	})
	require.Equalf(t, http.StatusOK, code, "broken_arrival: %v", body)

	// Blocked until the assigned author writes the marginalia.
	completePath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/complete"
	code, body = h.post(h.focusPlayerIdx(), completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked: %v", body)

	margPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-marginalia"
	// Only the assigned author may write — the preparer cannot.
	code, _ = h.post(preparerIdx, margPath, map[string]any{"peer_asset_id": peers[0], "text": "not mine to write"})
	assert.Equal(t, http.StatusForbidden, code, "non-author write should 403")

	code, body = h.post(authorIdx, margPath, map[string]any{"peer_asset_id": peers[0], "text": "a cruel rumor"})
	require.Equalf(t, http.StatusOK, code, "author write: %v", body)

	h.complete(planID)

	margs, err := h.q.ListMarginaliaByAsset(ctx, peers[0])
	require.NoError(t, err)
	found := false
	for _, m := range margs {
		if m.Text == "a cruel rumor" && !m.IsTorn {
			found = true
		}
	}
	assert.True(t, found, "author's marginalia should be present and intact")
}

func TestMakeIntroductions_Mar_BrokenJourney_WritesThenBreaks(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	planID, preparerIdx, peers := miToMar(t, h, 1)

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"peer_asset_id": peers[0], "outcome": "broken_journey", "text": "limped in, half-starved",
	})
	require.Equalf(t, http.StatusOK, code, "broken_journey: %v", body)

	h.complete(planID)

	margs, err := h.q.ListMarginaliaByAsset(ctx, peers[0])
	require.NoError(t, err)
	require.NotEmpty(t, margs)
	torn := false
	for _, m := range margs {
		if m.Text == "limped in, half-starved" && m.IsTorn {
			torn = true
		}
	}
	assert.True(t, torn, "the written marginalia should have been broken")
}
