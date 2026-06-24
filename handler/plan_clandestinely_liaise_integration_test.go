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
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// clMeeting bundles a prepared liaison plan with the two meeting peers (one per
// player) and a tearable marginalia on each — what most Things We Share tests
// need to drive update_peer / break_peer against the partner's MEETING peer.
type clMeeting struct {
	plan           dbgen.Plan
	preparerPeerID int64
	preparerMargID int64
	partnerPeerID  int64
	partnerMargID  int64
}

// clDriveToThingsWeShare prepares a Clandestinely Liaise between players[0]
// (preparer) and players[1] (partner) with a meeting peer from each retinue,
// runs the delay reveal, and advances to the things_we_share phase.
func clDriveToThingsWeShare(t *testing.T, h *planLifecycle) clMeeting {
	t.Helper()
	ctx := context.Background()

	// Each player brings a specific peer to the meeting.
	preparerPeer, preparerMarg := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, partnerMarg := h.seedPeerWithMarginalia(1, "partner's confidant")

	notes := "a meeting under the bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanClandestinelyLiaise,
		TargetPlayerID:   &partnerID,
		PreparerPeerID:   &preparerPeer,
		PartnerPeerID:    &partnerPeer,
		PreparationNotes: &notes,
	})

	// Delay reveal: both participants submit a face; the plan then gets a row.
	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	require.NotNil(t, ld.DelayRevealID, "delay reveal must be created at prep")
	require.Equal(t, preparerPeer, *ld.PreparerPeerID, "preparer peer stored at prep")
	require.Equal(t, partnerPeer, *ld.PartnerPeerID, "partner peer stored at prep")
	clSubmitReveal(t, h, *ld.DelayRevealID, 0, 2) // preparer
	clSubmitReveal(t, h, *ld.DelayRevealID, 1, 2) // partner

	// Jump to the resolved row and kick off resolution.
	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.RowNumber, "row should be set after delay reveal")
	h.jumpToRow(*refreshed.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "CL has no dice roll")

	// together_at_last → secrets_we_keep (the only manually-advanced phase).
	clAdvance(t, h, plan.ID)
	// Both keep a secret (any owned un-leveraged asset; the seeded MC works).
	// The second keep-secret auto-advances secrets_we_keep → things_we_share.
	clKeepSecret(t, h, plan.ID, 0)
	clKeepSecret(t, h, plan.ID, 1)

	refreshed, err = h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd2 := loadResolutionData(refreshed.ResolutionData)
	require.Equal(t, string(LiaiseThingsWeShare), string(rd2.EnsureLiaise().Phase),
		"the second keep-secret submission must auto-advance to things_we_share")
	return clMeeting{
		plan:           refreshed,
		preparerPeerID: preparerPeer,
		preparerMargID: preparerMarg,
		partnerPeerID:  partnerPeer,
		partnerMargID:  partnerMarg,
	}
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

// TestLiaise_KeepSecret_RejectsDuplicateSubmission proves the keep-secret
// handler guards against a double-write: a second submission by the same
// participant (a stale client after a refresh, a retry, or a direct API call) is
// rejected with 409 and no second secret is written. Without the guard a second
// secret would land on a second asset and a duplicate KeptSecrets entry would be
// appended.
func TestLiaise_KeepSecret_RejectsDuplicateSubmission(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Drive a liaison into the secrets_we_keep phase (mirrors the first half of
	// clDriveToThingsWeShare, stopping before both secrets are kept).
	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's confidant")

	notes := "a meeting under the bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanClandestinelyLiaise,
		TargetPlayerID:   &partnerID,
		PreparerPeerID:   &preparerPeer,
		PartnerPeerID:    &partnerPeer,
		PreparationNotes: &notes,
	})

	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	require.NotNil(t, ld.DelayRevealID, "delay reveal must be created at prep")
	clSubmitReveal(t, h, *ld.DelayRevealID, 0, 2) // preparer
	clSubmitReveal(t, h, *ld.DelayRevealID, 1, 2) // partner

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.RowNumber, "row should be set after delay reveal")
	h.jumpToRow(*refreshed.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "CL has no dice roll")

	// together_at_last → secrets_we_keep.
	clAdvance(t, h, plan.ID)

	// The preparer's main character bears the secret on the first submission.
	mc, err := h.q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[0].ID,
	})
	require.NoError(t, err)
	secretsBefore, err := h.q.ListSecretsByAsset(ctx, mc.ID)
	require.NoError(t, err)

	keepPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/keep-secret"

	// First submission succeeds.
	code, body := h.post(0, keepPath, map[string]any{"asset_id": mc.ID})
	require.Equalf(t, http.StatusOK, code, "first keep-secret should succeed: %v", body)

	// Second submission by the SAME player is rejected.
	code, body = h.post(0, keepPath, map[string]any{"asset_id": mc.ID})
	assert.Equalf(t, http.StatusConflict, code,
		"a second keep-secret by the same player must be rejected: %v", body)

	// Exactly one secret was written on the asset (one more than before).
	secretsAfter, err := h.q.ListSecretsByAsset(ctx, mc.ID)
	require.NoError(t, err)
	assert.Equal(t, len(secretsBefore)+1, len(secretsAfter),
		"exactly one secret must be written despite the duplicate submission")

	// resolution_data carries a single KeptSecrets entry for the preparer.
	refreshed, err = h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rdAfter := loadResolutionData(refreshed.ResolutionData)
	ldAfter := rdAfter.EnsureLiaise()
	preparerEntries := 0
	for _, ks := range ldAfter.KeptSecrets {
		if ks.PlayerID == h.tg.Players[0].ID {
			preparerEntries++
		}
	}
	assert.Equal(t, 1, preparerEntries, "exactly one KeptSecrets entry for the preparer")
}

// TestLiaise_ShareChoice_RejectsResubmitAfterBothIn proves the share-choice
// phase guard makes the Things We Share step refresh-safe. Once both
// participants have submitted, the phase auto-advances to "when will I see you
// again", so a stale client re-prompted after a refresh (or a retry) cannot
// re-run a share-choice — the second pass rides the phase check and is rejected
// with 409, applying no further effect. This is the share-choice analogue of
// the keep-secret double-write guard (TestLiaise_KeepSecret_RejectsDuplicateSubmission).
func TestLiaise_ShareChoice_RejectsResubmitAfterBothIn(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m := clDriveToThingsWeShare(t, h)

	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"

	// Both participants look at their partner's meeting peer — narrative-only
	// (no asset is torn or moved). The second submission auto-advances the
	// phase out of things_we_share.
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "look_at_secret", "target_asset_id": m.partnerPeerID,
	})
	require.Equalf(t, http.StatusOK, code, "preparer look_at_secret: %v", body)
	code, body = h.post(1, sharePath, map[string]any{
		"choice": "look_at_secret", "target_asset_id": m.preparerPeerID,
	})
	require.Equalf(t, http.StatusOK, code, "partner look_at_secret: %v", body)

	// The phase advanced past things_we_share once both were in.
	refreshed, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	phase := rd.EnsureLiaise().Phase
	require.NotEqualf(t, string(LiaiseThingsWeShare), string(phase),
		"both submissions must auto-advance out of things_we_share (got %q)", phase)

	// A stale re-submit by a participant is now rejected by the phase guard.
	code, body = h.post(0, sharePath, map[string]any{
		"choice": "look_at_secret", "target_asset_id": m.partnerPeerID,
	})
	assert.Equalf(t, http.StatusConflict, code,
		"a re-submitted share-choice after the phase advanced must 409: %v", body)
}

// TestLiaise_ShareChoice_RejectsForeignTarget proves Things We Share options
// must target the PARTNER's assets — a third party's asset is rejected.
func TestLiaise_ShareChoice_RejectsForeignTarget(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clDriveToThingsWeShare(t, h)

	// players[2] is not a participant; their asset is an invalid look_at_secret
	// target (look_at_secret accepts any asset type, so ownership is what trips).
	foreign, _ := h.seedPeerWithMarginalia(2, "outsider's peer")

	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"
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
	m := clDriveToThingsWeShare(t, h)

	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's peer")
	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"
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
	m := clDriveToThingsWeShare(t, h)

	// Preparer (player 0) breaks the PARTNER's (player 1) MEETING peer.
	partnerPeer, partnerMarg := m.partnerPeerID, m.partnerMargID
	// Partner (player 1) takes a gift from the preparer (player 0) — needs a
	// non-peer owned by player 0.
	gift, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[0].ID,
		CreatorID: h.tg.Players[0].ID, AssetType: model.AssetArtifact, Name: "preparer's trinket",
	})
	require.NoError(t, err)
	giftID := gift.ID

	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"

	// Preparer: break partner's meeting peer at the chosen marginalia.
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

	// Partner's peer lost its last marginalia → destroyed.
	destroyed, err := h.q.GetAssetByID(ctx, partnerPeer)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed, "breaking the last marginalia destroys the peer")

	// Gift transferred to the partner (player 1).
	movedGift, err := h.q.GetAssetByID(ctx, giftID)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[1].ID, movedGift.OwnerID, "gift should belong to the partner now")
}

// TestLiaise_ShareChoice_UpdatePeer_RewritesMarginalia proves update_peer edits
// (rewrites) one marginalia on the partner's meeting peer — it does NOT tear it
// (tearing is reserved for break_peer). The authored text applies once both
// players submit.
func TestLiaise_ShareChoice_UpdatePeer_RewritesMarginalia(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m := clDriveToThingsWeShare(t, h)

	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"

	// Preparer (0) rewrites a note on the partner's (1) meeting peer.
	const newText = "now sworn to a rival house"
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "update_peer", "target_asset_id": m.partnerPeerID,
		"target_marginalia_id": m.partnerMargID, "update_text": newText,
	})
	require.Equalf(t, http.StatusOK, code, "preparer update_peer: %v", body)

	// Not applied until both submit.
	before, err := h.q.GetMarginaliaByID(ctx, m.partnerMargID)
	require.NoError(t, err)
	assert.NotEqual(t, newText, before.Text, "effects apply only once both submit")

	// Partner (1) rewrites a note on the preparer's (0) meeting peer.
	code, body = h.post(1, sharePath, map[string]any{
		"choice": "update_peer", "target_asset_id": m.preparerPeerID,
		"target_marginalia_id": m.preparerMargID, "update_text": "a closely guarded ally",
	})
	require.Equalf(t, http.StatusOK, code, "partner update_peer: %v", body)

	// The note was rewritten, not torn, and the peer survives (no destruction).
	after, err := h.q.GetMarginaliaByID(ctx, m.partnerMargID)
	require.NoError(t, err)
	assert.Equal(t, newText, after.Text)
	assert.False(t, after.IsTorn, "update rewrites, never tears")
	peer, err := h.q.GetAssetByID(ctx, m.partnerPeerID)
	require.NoError(t, err)
	assert.False(t, peer.IsDestroyed, "updating a note never destroys the peer")

	// The second share submission auto-advances things_we_share →
	// when_will_i_see_you_again and creates the redelay reveal — no manual
	// advance click. (Pre-fix the preparer had to press "Advance" here.)
	refreshed, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	ld := rd.EnsureLiaise()
	assert.Equal(t, string(LiaiseWhenWillISeeYouAgain), string(ld.Phase),
		"both share-choices in → auto-advance to when_will_i_see_you_again")
	require.NotNil(t, ld.RedelayRevealID, "redelay reveal must be created on auto-advance")
}

// TestLiaise_ShareChoice_UpdatePeer_RequiresMarginaliaAndText proves update_peer
// is rejected without a target marginalia or without replacement text.
func TestLiaise_ShareChoice_UpdatePeer_RequiresMarginaliaAndText(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clDriveToThingsWeShare(t, h)
	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"

	// Missing marginalia.
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "update_peer", "target_asset_id": m.partnerPeerID, "update_text": "x",
	})
	assert.Equalf(t, http.StatusBadRequest, code, "update_peer needs a marginalia: %v", body)

	// Missing replacement text.
	code, body = h.post(0, sharePath, map[string]any{
		"choice": "update_peer", "target_asset_id": m.partnerPeerID,
		"target_marginalia_id": m.partnerMargID,
	})
	assert.Equalf(t, http.StatusBadRequest, code, "update_peer needs update_text: %v", body)
}

// TestLiaise_ShareChoice_BreakPeer_RejectsNonMeetingPeer proves break_peer must
// target the partner's MEETING peer specifically — another partner-owned peer
// (not the one brought to the liaison) is rejected.
func TestLiaise_ShareChoice_BreakPeer_RejectsNonMeetingPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clDriveToThingsWeShare(t, h)

	// A second peer the partner owns that is NOT the meeting peer.
	otherPeer, otherMarg := h.seedPeerWithMarginalia(1, "partner's other peer")

	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "break_peer", "target_asset_id": otherPeer, "target_marginalia_id": otherMarg,
	})
	assert.Equalf(t, http.StatusBadRequest, code,
		"break_peer must target the meeting peer, not an arbitrary partner peer: %v", body)

	// update_peer is likewise pinned to the meeting peer.
	code, body = h.post(0, sharePath, map[string]any{
		"choice": "update_peer", "target_asset_id": otherPeer,
	})
	assert.Equalf(t, http.StatusBadRequest, code,
		"update_peer must target the meeting peer: %v", body)
}

// TestLiaise_Prepare_RejectsPeerNotOwned proves prep validates each meeting peer
// belongs to the right player — a partner_peer_id the partner doesn't own fails.
func TestLiaise_Prepare_RejectsPeerNotOwned(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's peer")
	// A peer owned by the PREPARER, wrongly passed as the partner's meeting peer.
	notPartnersPeer, _ := h.seedPeerWithMarginalia(0, "preparer's second peer")

	notes := "a meeting"
	partnerID := h.tg.Players[1].ID
	path := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/prepare-plan"
	code, body := h.post(h.focusPlayerIdx(), path, PreparePlanRequest{
		PlanType:         model.PlanClandestinelyLiaise,
		TargetPlayerID:   &partnerID,
		PreparerPeerID:   &preparerPeer,
		PartnerPeerID:    &notPartnersPeer,
		PreparationNotes: &notes,
	})
	assert.Equalf(t, http.StatusBadRequest, code,
		"partner's meeting peer must be owned by the partner: %v", body)
}

// TestLiaise_PrepareResponse_CarriesPartnerAndReveal proves the prepare-plan
// HTTP response carries the resolution_data fields OnPrepare writes —
// partner_id and delay_reveal_id. The plan.prepared broadcast reuses the same
// struct, and non-preparer clients rely solely on that broadcast (they don't
// refetch). Regression for a bug where the response/broadcast plan was a
// pre-OnPrepare snapshot, so observers saw a "?" partner and no waiting-on bar.
func TestLiaise_PrepareResponse_CarriesPartnerAndReveal(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's confidant")

	notes := "under a bridge"
	partnerID := h.tg.Players[1].ID
	path := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/prepare-plan"
	code, body := h.post(h.focusPlayerIdx(), path, PreparePlanRequest{
		PlanType:         model.PlanClandestinelyLiaise,
		TargetPlayerID:   &partnerID,
		PreparerPeerID:   &preparerPeer,
		PartnerPeerID:    &partnerPeer,
		PreparationNotes: &notes,
	})
	require.Equalf(t, http.StatusCreated, code, "prepare-plan failed: %v", body)

	// Inspect the response plan directly — NOT a DB refetch — since that is what
	// the broadcast ships to other clients.
	planBlob, _ := json.Marshal(body["plan"])
	var p dbgen.Plan
	require.NoError(t, json.Unmarshal(planBlob, &p))

	rd := loadResolutionData(p.ResolutionData)
	ld := rd.EnsureLiaise()
	require.NotNil(t, ld.PartnerID, "response plan must include partner_id")
	assert.Equal(t, partnerID, *ld.PartnerID)
	require.NotNil(t, ld.DelayRevealID, "response plan must include delay_reveal_id")
	require.NotNil(t, ld.PreparerPeerID, "response plan must include preparer_peer_id")
	assert.Equal(t, preparerPeer, *ld.PreparerPeerID)
	require.NotNil(t, ld.PartnerPeerID, "response plan must include partner_peer_id")
	assert.Equal(t, partnerPeer, *ld.PartnerPeerID)
}

// TestLiaise_GetReveal_PartialSubmission_ExposesRevealedAtNotFace proves the
// GET reveal endpoint lets clients tell who has submitted (revealed_at) before
// the reveal completes, WITHOUT leaking the hidden faces. Regression for a bug
// where clients keyed "has submitted" off face (always null pre-completion), so
// a submitting player got no feedback and the other player's waiting-on list
// never shrank.
func TestLiaise_GetReveal_PartialSubmission_ExposesRevealedAtNotFace(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's confidant")

	notes := "under a bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanClandestinelyLiaise,
		TargetPlayerID:   &partnerID,
		PreparerPeerID:   &preparerPeer,
		PartnerPeerID:    &partnerPeer,
		PreparationNotes: &notes,
	})
	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	require.NotNil(t, ld.DelayRevealID)
	revealID := *ld.DelayRevealID

	// Only the preparer (player 0) submits so far.
	clSubmitReveal(t, h, revealID, 0, 4)

	// The partner (player 1) reads the reveal.
	code, body := h.get(1, "/api/reveals/"+strconv.FormatInt(revealID, 10))
	require.Equalf(t, http.StatusOK, code, "get reveal: %v", body)

	assert.Equal(t, false, body["is_complete"], "reveal not complete after one submission")

	entries, ok := body["entries"].([]any)
	require.Truef(t, ok, "entries should be a list: %v", body["entries"])
	require.Len(t, entries, 2)

	preparerPID := float64(h.tg.Players[0].ID)
	partnerPID := float64(h.tg.Players[1].ID)
	for _, raw := range entries {
		e := raw.(map[string]any)
		switch e["player_id"] {
		case preparerPID:
			assert.NotNil(t, e["revealed_at"], "submitter's revealed_at must be set")
			assert.Nil(t, e["face"], "face stays hidden until the reveal completes")
		case partnerPID:
			assert.Nil(t, e["revealed_at"], "non-submitter's revealed_at must be null")
			assert.Nil(t, e["face"], "non-submitter has no face")
		default:
			t.Fatalf("unexpected entry player_id: %v", e["player_id"])
		}
	}

	// The submitter reading their OWN reveal sees their own face (it leaks
	// nothing) so the UI can keep the pick highlighted, but still not the
	// partner's, and the reveal is still incomplete.
	code, body = h.get(0, "/api/reveals/"+strconv.FormatInt(revealID, 10))
	require.Equalf(t, http.StatusOK, code, "get reveal as submitter: %v", body)
	assert.Equal(t, false, body["is_complete"])
	for _, raw := range body["entries"].([]any) {
		e := raw.(map[string]any)
		if e["player_id"] == preparerPID {
			assert.EqualValues(t, 4, e["face"], "submitter sees their own face")
		} else {
			assert.Nil(t, e["face"], "submitter still can't see the partner's face")
		}
	}
}

// TestLiaise_ShareChoice_MeetingPeerDestroyed_Graceful proves that if the
// meeting peer is destroyed before the liaison resolves, break_peer/update_peer
// fail gracefully (a clear 4xx, never a 500) rather than crashing.
func TestLiaise_ShareChoice_MeetingPeerDestroyed_Graceful(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m := clDriveToThingsWeShare(t, h)

	// The partner's meeting peer is destroyed in some other plan before the
	// liaison's Things We Share resolves.
	require.NoError(t, h.q.DestroyAsset(ctx, m.partnerPeerID))

	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "break_peer", "target_asset_id": m.partnerPeerID, "target_marginalia_id": m.partnerMargID,
	})
	assert.GreaterOrEqualf(t, code, http.StatusBadRequest, "should reject: %v", body)
	assert.Lessf(t, code, http.StatusInternalServerError,
		"a destroyed meeting peer must be handled gracefully, not as a 500: %v", body)
}

// clDriveToRedelay drives a liaison through Things We Share (both players read
// the other's secret — the lowest-friction share choice) into the
// when_will_i_see_you_again phase, and returns the meeting plus the redelay
// reveal id ready for both faces.
func clDriveToRedelay(t *testing.T, h *planLifecycle) (clMeeting, int64) {
	t.Helper()
	m := clDriveToThingsWeShare(t, h)
	sharePath := "/api/plans/" + strconv.FormatInt(m.plan.ID, 10) + "/share-choice"

	// look_at_secret targets any partner-owned asset — each player peeks at the
	// other's meeting peer. The second submission auto-advances to the redelay.
	code, body := h.post(0, sharePath, map[string]any{
		"choice": "look_at_secret", "target_asset_id": m.partnerPeerID,
	})
	require.Equalf(t, http.StatusOK, code, "preparer look_at_secret: %v", body)
	code, body = h.post(1, sharePath, map[string]any{
		"choice": "look_at_secret", "target_asset_id": m.preparerPeerID,
	})
	require.Equalf(t, http.StatusOK, code, "partner look_at_secret: %v", body)

	refreshed, err := h.q.GetPlanByID(context.Background(), m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	ld := rd.EnsureLiaise()
	require.Equal(t, string(LiaiseWhenWillISeeYouAgain), string(ld.Phase),
		"both share-choices in → auto-advance to when_will_i_see_you_again")
	require.NotNil(t, ld.RedelayRevealID, "redelay reveal must exist")
	m.plan = refreshed
	return m, *ld.RedelayRevealID
}

// TestLiaise_Redelay_SchedulesFollowUp proves that completing the "When will I
// see you again?" reveal via the generic reveal-submit endpoint (the one the UI
// actually calls) finalizes the liaison: it marks the phase done and schedules
// a new Clandestinely Liaise plan on the result-delay row, carrying forward the
// same partner and meeting peers. Regression test for the soft-lock where the
// reveal completed but nothing downstream ran.
func TestLiaise_Redelay_SchedulesFollowUp(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m, redelayID := clDriveToRedelay(t, h)

	currentRow := h.tg.Game.CurrentRow
	preparerID := h.tg.Players[0].ID
	partnerID := h.tg.Players[1].ID

	// Sam=2, Charlie=1 → ceil(avg)=2, scheduling a meeting two rows ahead.
	clSubmitReveal(t, h, redelayID, 0, 2)
	clSubmitReveal(t, h, redelayID, 1, 1)

	// The liaison auto-resolves on the final reveal — no manual Complete step.
	refreshed, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	ld := rd.EnsureLiaise()
	assert.Equal(t, string(LiaiseDone), string(ld.Phase),
		"completing the redelay reveal must finalize the liaison to done")
	assert.Equal(t, model.PlanResolved, refreshed.Status,
		"the liaison must auto-resolve when the redelay reveal finalizes")
	require.NotNil(t, refreshed.Result)
	assert.Equal(t, "make", *refreshed.Result, "a liaison always resolves as a make")

	// A follow-up Clandestinely Liaise was scheduled two rows ahead.
	wantRow := currentRow + 2
	plans, err := h.q.ListPlansByRow(ctx, dbgen.ListPlansByRowParams{
		GameID: h.tg.Game.ID, RowNumber: new(wantRow),
	})
	require.NoError(t, err)
	var followUp *dbgen.Plan
	for i := range plans {
		if plans[i].PlanType == model.PlanClandestinelyLiaise && plans[i].ID != m.plan.ID {
			followUp = &plans[i]
		}
	}
	require.NotNilf(t, followUp, "a follow-up liaison must be scheduled on row %d", wantRow)
	assert.Equal(t, preparerID, followUp.PreparerID, "same preparer")
	require.NotNil(t, followUp.TargetPlayerID)
	assert.Equal(t, partnerID, *followUp.TargetPlayerID, "same partner")

	// The follow-up carries forward both meeting peers (the same pair reconvenes).
	frd := loadResolutionData(followUp.ResolutionData)
	fld := frd.EnsureLiaise()
	require.NotNil(t, fld.PreparerPeerID)
	require.NotNil(t, fld.PartnerPeerID)
	assert.Equal(t, m.preparerPeerID, *fld.PreparerPeerID)
	assert.Equal(t, m.partnerPeerID, *fld.PartnerPeerID)
}

// TestLiaise_Redelay_CancelSchedulesNothing proves that when either player picks
// face 0, the redelay is cancelled: the liaison finalizes to done but no
// follow-up plan is scheduled.
func TestLiaise_Redelay_CancelSchedulesNothing(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m, redelayID := clDriveToRedelay(t, h)

	before, err := h.q.ListPlansByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)

	// The partner cancels with a 0.
	clSubmitReveal(t, h, redelayID, 0, 3)
	clSubmitReveal(t, h, redelayID, 1, 0)

	refreshed, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	ld := rd.EnsureLiaise()
	assert.Equal(t, string(LiaiseDone), string(ld.Phase),
		"a cancelled redelay still finalizes the liaison to done")
	assert.Equal(t, model.PlanResolved, refreshed.Status,
		"a cancelled liaison still auto-resolves")

	after, err := h.q.ListPlansByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "a face-0 cancel must not schedule a follow-up plan")
}
