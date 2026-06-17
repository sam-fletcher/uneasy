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

// hfGuestRollMar makes players[idx] roll and forces a mar outcome on their
// guest roll. Every player is a guest by default, so there is no join step.
// Returns once the roll is resolved as mar.
func hfGuestRollMar(t *testing.T, h *planLifecycle, planID int64, idx int) {
	t.Helper()
	rollPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/guest-roll"
	code, body := h.post(idx, rollPath, map[string]any{"action": "roll"})
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
	assert.True(t, destroyed.IsDestroyed, "tearing the last marginalia destroys the main character")
}

// TestHostFestivity_HostFreeMake_BenefitsHost proves the host's free make (the
// one they take for each guest who marred or opted out) benefits the HOST, not
// the guest: introduce_peer adds the new peer to the host's own retinue.
func TestHostFestivity_HostFreeMake_BenefitsHost(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)
	hostID := h.tg.Players[hostIdx].ID

	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	choicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-choice"

	// g1 rolls a mar and locks it in (owes the host a free make).
	hfGuestRollMar(t, h, plan.ID, g1)
	code, body := h.post(g1, choicePath, map[string]any{"choice": "accept_duels"})
	require.Equalf(t, http.StatusOK, code, "g1 mar choice: %v", body)

	// The host doesn't roll — they may not, and don't need to (their earned make
	// is recorded up front). Once g2 opts out, every guest has acted → host_choosing.
	code, body = h.post(hostIdx, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusForbidden, code, "host must not be allowed to roll/opt-out: %v", body)
	code, body = h.post(g2, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "g2 opt out: %v", body)

	// Host takes introduce_peer for g1's owed slot.
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"target_player_id": h.tg.Players[g1].ID,
		"choice":           "introduce_peer",
		"peer_name":        "Gilded Guest",
	})
	require.Equalf(t, http.StatusOK, code, "host-choice introduce_peer: %v", body)

	// The new peer belongs to the HOST, not g1.
	hostAssets, err := h.q.ListAssetsByOwner(ctx, hostID)
	require.NoError(t, err)
	var found bool
	for _, a := range hostAssets {
		if a.GameID == h.tg.Game.ID && a.AssetType == model.AssetPeer && a.Name == "Gilded Guest" {
			found = true
		}
	}
	assert.True(t, found, "the host's free make adds the peer to the host's own retinue")
}

// TestHostFestivity_HostEarnedMake proves the host is pre-recorded with the
// FestivityOutcomeHost outcome (so they never roll), and may take that earned
// make for themself via host-choice targeting their own id.
func TestHostFestivity_HostEarnedMake(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)
	hostID := h.tg.Players[hostIdx].ID

	// The host's earned make is recorded up front, before anyone acts.
	reloaded, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(reloaded.ResolutionData)
	st := rd.EnsureFestivity()
	assert.Equal(t, "host", st.Outcomes[strconv.FormatInt(hostID, 10)],
		"host outcome is pre-recorded as the earned free make")

	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"

	// The host cannot roll.
	code, body := h.post(hostIdx, rollPath, map[string]any{"action": "roll"})
	require.Equalf(t, http.StatusForbidden, code, "host roll must be forbidden: %v", body)

	// Both guests opt out → host_choosing (the host is already resolved).
	for _, idx := range []int{g1, g2} {
		code, body = h.post(idx, rollPath, map[string]any{"action": "opt_out"})
		require.Equalf(t, http.StatusOK, code, "guest opt out: %v", body)
	}

	// The host takes their own earned make, targeting themself.
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"target_player_id": hostID,
		"choice":           "introduce_peer",
		"peer_name":        "Host's Own Guest",
	})
	require.Equalf(t, http.StatusOK, code, "host self make: %v", body)

	// New peer belongs to the host.
	hostAssets, err := h.q.ListAssetsByOwner(ctx, hostID)
	require.NoError(t, err)
	var found bool
	for _, a := range hostAssets {
		if a.GameID == h.tg.Game.ID && a.AssetType == model.AssetPeer && a.Name == "Host's Own Guest" {
			found = true
		}
	}
	assert.True(t, found, "the host's earned make adds the peer to their own retinue")
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
