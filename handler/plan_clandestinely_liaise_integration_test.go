//go:build integration

// handler/plan_clandestinely_liaise_integration_test.go — Things We Share
// mechanical-effect coverage for Clandestinely Liaise. Guards the rules-correct
// behaviour added after the audit:
//
//   - all five options target the PARTNER's assets (validated server-side).
//   - break_peer tears the breaker's chosen marginalia on the partner's peer
//     via breakMarginalia (auto-destroy on the last), and the breaker may be a
//     different player than the owner.
//   - take_gift transfers a partner NON-peer and broadcasts AssetTakenPayload.
//   - foreign / wrong-type targets are rejected.

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

// clDriveToThingsWeShare prepares a Clandestinely Liaise between players[0]
// (preparer) and players[1] (partner), runs the delay reveal, and advances to
// the things_we_share phase. Returns the plan.
func clDriveToThingsWeShare(t *testing.T, h *planLifecycle) dbgen.Plan {
	t.Helper()
	ctx := context.Background()

	notes := "a meeting under the bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanClandestinelyLiaise,
		TargetPlayerID:   &partnerID,
		PreparationNotes: &notes,
	})

	// Delay reveal: both participants submit a face; the plan then gets a row.
	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	require.NotNil(t, ld.DelayRevealID, "delay reveal must be created at prep")
	clSubmitReveal(t, h, *ld.DelayRevealID, 0, 2) // preparer
	clSubmitReveal(t, h, *ld.DelayRevealID, 1, 2) // partner

	// Jump to the resolved row and kick off resolution.
	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.RowNumber, "row should be set after delay reveal")
	h.jumpToRow(*refreshed.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "CL has no dice roll")

	// together_at_last → secrets_we_keep.
	clAdvance(t, h, plan.ID)
	// Both keep a secret (any owned un-leveraged asset; the seeded MC works).
	clKeepSecret(t, h, plan.ID, 0)
	clKeepSecret(t, h, plan.ID, 1)
	// secrets_we_keep → things_we_share.
	clAdvance(t, h, plan.ID)

	refreshed, err = h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd2 := loadResolutionData(refreshed.ResolutionData)
	require.Equal(t, string(LiaiseThingsWeShare), string(rd2.EnsureLiaise().Phase))
	return refreshed
}

// clSubmitReveal drives the real reveal-submit endpoint so the delay-reveal
// completion side effects (row assignment) run.
func clSubmitReveal(t *testing.T, h *planLifecycle, revealID int64, playerIdx int, face int16) {
	t.Helper()
	path := "/api/reveals/" + strconv.FormatInt(revealID, 10) + "/submit"
	code, body := h.post(playerIdx, path, map[string]any{"face": face})
	require.Equalf(t, http.StatusOK, code, "reveal submit: %v", body)
}

func clAdvance(t *testing.T, h *planLifecycle, planID int64) {
	t.Helper()
	// advance-liaise is preparer-gated.
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/advance-liaise"
	code, body := h.post(0, path, nil)
	require.Equalf(t, http.StatusOK, code, "advance-liaise: %v", body)
}

func clKeepSecret(t *testing.T, h *planLifecycle, planID int64, playerIdx int) {
	t.Helper()
	// Use the player's main character (seeded by the harness) to bear the secret.
	mc, err := h.q.GetMainCharacterByOwner(context.Background(), dbgen.GetMainCharacterByOwnerParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[playerIdx].ID,
	})
	require.NoError(t, err)
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/keep-secret"
	code, body := h.post(playerIdx, path, map[string]any{"asset_id": mc.ID})
	require.Equalf(t, http.StatusOK, code, "keep-secret: %v", body)
}

// clSeedPeerWithMarginalia creates a peer owned by players[ownerIdx] with one
// intact marginalium and returns the asset + marginalia ids.
func clSeedPeerWithMarginalia(t *testing.T, h *planLifecycle, ownerIdx int, name string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	a, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[ownerIdx].ID,
		CreatorID: h.tg.Players[ownerIdx].ID, AssetType: model.AssetPeer, Name: name,
	})
	require.NoError(t, err)
	m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: a.ID, Position: 1, Text: "note",
	})
	require.NoError(t, err)
	return a.ID, m.ID
}

// TestLiaise_ShareChoice_RejectsForeignTarget proves Things We Share options
// must target the PARTNER's assets — a third party's asset is rejected.
func TestLiaise_ShareChoice_RejectsForeignTarget(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan := clDriveToThingsWeShare(t, h)

	// players[2] is not a participant; their asset is an invalid look_at_secret
	// target (look_at_secret accepts any asset type, so ownership is what trips).
	foreign, _ := clSeedPeerWithMarginalia(t, h, 2, "outsider's peer")

	sharePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/share-choice"
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "look_at_secret", "target_asset_id": foreign,
	})
	assert.Equalf(t, http.StatusForbidden, code,
		"targeting a non-partner's asset should 403: %v", body)
}

// TestLiaise_ShareChoice_TakeGift_RejectsPeer proves take_gift must be a
// non-peer.
func TestLiaise_ShareChoice_TakeGift_RejectsPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan := clDriveToThingsWeShare(t, h)

	partnerPeer, _ := clSeedPeerWithMarginalia(t, h, 1, "partner's peer")
	sharePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/share-choice"
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "take_gift", "target_asset_id": partnerPeer,
	})
	assert.Equalf(t, http.StatusBadRequest, code, "gift must be non-peer: %v", body)
}

// TestLiaise_ShareChoice_BreakPartnerPeer_AutoDestroys proves break_peer tears
// the chosen marginalia on the PARTNER's peer and auto-destroys on the last,
// applied once both players submit.
func TestLiaise_ShareChoice_BreakPartnerPeer_AutoDestroys(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan := clDriveToThingsWeShare(t, h)

	// Preparer (player 0) will break the PARTNER's (player 1) peer.
	partnerPeer, partnerMarg := clSeedPeerWithMarginalia(t, h, 1, "partner's fragile peer")
	// Partner (player 1) takes a gift from the preparer (player 0) — needs a
	// non-peer owned by player 0.
	gift, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[0].ID,
		CreatorID: h.tg.Players[0].ID, AssetType: model.AssetArtifact, Name: "preparer's trinket",
	})
	require.NoError(t, err)
	giftID := gift.ID

	sharePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/share-choice"

	// Preparer: break partner's peer at the chosen marginalia.
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "break_peer", "target_asset_id": partnerPeer, "target_marginalia_id": partnerMarg,
	})
	require.Equalf(t, http.StatusOK, code, "preparer break_peer: %v", body)

	// Effects haven't applied yet (partner hasn't submitted).
	stillThere, err := h.q.GetMarginaliaByID(ctx, partnerMarg)
	require.NoError(t, err)
	assert.False(t, stillThere.IsTorn, "effects apply only once both submit")

	// Partner: take the preparer's non-peer gift. This is the second submission,
	// so both effects now apply.
	code, body = h.post(1, sharePath, map[string]any{
		"choice": "take_gift", "target_asset_id": giftID,
	})
	require.Equalf(t, http.StatusOK, code, "partner take_gift: %v", body)

	// Partner's peer lost its last marginalium → destroyed.
	destroyed, err := h.q.GetAssetByID(ctx, partnerPeer)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed, "breaking the last marginalium destroys the peer")

	// Gift transferred to the partner (player 1).
	movedGift, err := h.q.GetAssetByID(ctx, giftID)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[1].ID, movedGift.OwnerID, "gift should belong to the partner now")
}
