//go:build integration

// handler/plan_host_festivity_integration_test.go — guest make/mar effect
// coverage for Host Festivity. Guards the rules-correct behaviour added after
// the audit:
//
//   - break_self tears the acting player's CHOSEN marginalia on their main
//     character via breakMarginalia (auto-destroy on the last).
//   - disagreement must target one of the acting player's OWN peers.

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

// hfPrepareToSocializing prepares a Host Festivity and kicks it off, leaving it
// in the socializing phase. Returns the plan and the host (preparer) index.
func hfPrepareToSocializing(t *testing.T, h *planLifecycle) (dbgen.Plan, int) {
	t.Helper()
	notes := "a grand ball"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanHostFestivity,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "Host Festivity has no plan-level roll")
	return plan, h.preparerIdxFor(plan.ID)
}

// hfGuestRollMar makes players[idx] join, roll, and forces a mar outcome on
// their guest roll. Returns once the roll is resolved as mar.
func hfGuestRollMar(t *testing.T, h *planLifecycle, planID int64, idx int) {
	t.Helper()
	join := "/api/plans/" + strconv.FormatInt(planID, 10) + "/join-festivity"
	code, body := h.post(idx, join, nil)
	require.Equalf(t, http.StatusOK, code, "join-festivity: %v", body)

	rollPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/guest-roll"
	code, body = h.post(idx, rollPath, map[string]any{"action": "roll"})
	require.Equalf(t, http.StatusCreated, code, "guest-roll: %v", body)

	// Force the just-created guest roll to a mar outcome.
	rollBlob := body["roll"].(map[string]any)
	rollID := int64(rollBlob["id"].(float64))
	h.forceRoll(rollID, "mar", 0)
}

// hfSeedMCMarginalia adds `n` marginalia to players[idx]'s main character and
// returns their ids in order.
func hfSeedMCMarginalia(t *testing.T, h *planLifecycle, idx, n int) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	mc, err := h.q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[idx].ID,
	})
	require.NoError(t, err)
	ids := make([]int64, n)
	for i := range n {
		m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: mc.ID, Position: int16(i + 1), Text: "note",
		})
		require.NoError(t, err)
		ids[i] = m.ID
	}
	return mc.ID, ids
}

// TestHostFestivity_Mar_BreakSelf_AutoDestroysOnLast proves a guest's break_self
// tears their chosen marginalia and destroys the MC when it was the last.
func TestHostFestivity_Mar_BreakSelf_AutoDestroysOnLast(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	guestIdx := (hostIdx + 1) % len(h.tg.Players)

	mcID, margIDs := hfSeedMCMarginalia(t, h, guestIdx, 1)
	hfGuestRollMar(t, h, plan.ID, guestIdx)

	choicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-choice"
	code, body := h.post(guestIdx, choicePath, map[string]any{
		"choice": "break_self", "marginalia_id": margIDs[0],
	})
	require.Equalf(t, http.StatusOK, code, "guest-choice break_self: %v", body)

	destroyed, err := h.q.GetAssetByID(ctx, mcID)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed, "tearing the last marginalium destroys the main character")
}

// TestHostFestivity_Mar_Disagreement_RejectsForeignPeer proves disagreement must
// target one of the acting player's own peers.
func TestHostFestivity_Mar_Disagreement_RejectsForeignPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	guestIdx := (hostIdx + 1) % len(h.tg.Players)
	otherIdx := (hostIdx + 2) % len(h.tg.Players)

	// A peer owned by a different player than the acting guest.
	foreignPeer := h.seedPeer(otherIdx, "someone else's peer")
	hfGuestRollMar(t, h, plan.ID, guestIdx)

	choicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-choice"
	code, body := h.post(guestIdx, choicePath, map[string]any{
		"choice": "disagreement", "asset_id": foreignPeer,
	})
	assert.Equalf(t, http.StatusBadRequest, code,
		"disagreement on a peer you don't own should 400: %v", body)
}
