//go:build integration

// handler/plan_scenes_integration_test.go — plan-scene lifecycle coverage
// (adr/CHAT_OVERHAUL_PLAN.md Phase 5). Uses the shared planLifecycle harness
// (plan_lifecycle_harness_test.go) plus each plan type's own drive-to-phase
// helpers (clDriveToRedelay for Liaise, hfPrepareToSocializing for Festivity)
// so these tests exercise the real HTTP flow, not a shortcut.

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

// TestPlanScene_Liaise_OpensWithPreparerAndPartnerOnly proves a Clandestinely
// Liaise plan-scene opens the moment resolution kicks off (before any phase
// advances), with exactly the preparer and partner as peers — a third player
// in the game is not a participant and is rejected speaking as their main
// character, while the partner is allowed.
func TestPlanScene_Liaise_OpensWithPreparerAndPartnerOnly(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

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
	require.NotNil(t, ld.DelayRevealID)
	clSubmitReveal(t, h, *ld.DelayRevealID, 0, 2)
	clSubmitReveal(t, h, *ld.DelayRevealID, 1, 2)

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.RowNumber)
	h.jumpToRow(*refreshed.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "CL has no dice roll")

	scene, peers := h.activeScene(2)
	require.NotNil(t, scene, "resolving a Liaise must open a plan-scene")
	assert.Equal(t, model.SceneKindPlan, scene.Kind)
	require.NotNil(t, scene.PlanID)
	assert.Equal(t, plan.ID, *scene.PlanID)
	assert.Equal(t, h.tg.Players[0].ID, scene.FocusPlayerID, "focus/owner = preparer")

	preparerMC := h.mainCharacterID(0)
	partnerMC := h.mainCharacterID(1)
	outsiderMC := h.mainCharacterID(2)
	byAsset := map[int64]scenePeerView{}
	for _, p := range peers {
		byAsset[p.PeerAssetID] = p
	}
	require.Contains(t, byAsset, preparerMC, "preparer's MC must be an explicit peer row (no implicit-MC shortcut in a plan-scene)")
	require.Contains(t, byAsset, partnerMC, "partner's MC must be a peer")
	assert.NotContains(t, byAsset, outsiderMC, "a non-participant's MC must not be a peer")

	// The preparer can speak as their main character.
	code, body := h.postMessage(0, "shall we begin?", preparerMC)
	require.Equalf(t, http.StatusCreated, code, "preparer speaking as MC: %v", body)
	// The partner can speak as their main character.
	code, body = h.postMessage(1, "I've been waiting.", partnerMC)
	require.Equalf(t, http.StatusCreated, code, "partner speaking as MC: %v", body)
	// The outsider (not a participant) cannot speak as their own main character.
	code, body = h.postMessage(2, "can I join?", outsiderMC)
	assert.Equalf(t, http.StatusBadRequest, code,
		"a non-participant must be rejected speaking as their own MC: %v", body)
}

// TestPlanScene_EndScene_RefusesPlanScene proves the turn-scene EndScene route
// refuses to end a plan-scene — it can only end when its plan resolves.
func TestPlanScene_EndScene_RefusesPlanScene(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's confidant")
	notes := "a meeting"
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
	clSubmitReveal(t, h, *ld.DelayRevealID, 0, 3)
	clSubmitReveal(t, h, *ld.DelayRevealID, 1, 3)
	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	h.jumpToRow(*refreshed.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	scene, _ := h.activeScene(0)
	require.NotNil(t, scene)

	endPath := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/end-scene"
	code, body := h.post(h.focusPlayerIdx(), endPath, nil)
	assert.Equalf(t, http.StatusConflict, code,
		"end-scene must refuse a plan-scene: %v", body)

	// The scene is still active and unchanged afterward.
	stillActive, _ := h.activeScene(0)
	require.NotNil(t, stillActive)
	assert.Equal(t, scene.ID, stillActive.ID)
	assert.False(t, stillActive.EndedAt.Valid)
}

// TestPlanScene_Liaise_ClosesOnResolutionAndAllowsFollowScene drives a
// Clandestinely Liaise all the way to completion (redelay cancelled, the
// lowest-friction path — mirrors TestLiaise_Redelay_CancelSchedulesNothing)
// and proves (a) its plan-scene closes the moment the plan resolves, and (b)
// the ordinary turn-scene follow-scene mechanism for that resolved plan is
// completely unaffected by the plan-scene that came and went earlier on the
// same row — the focus player can still set the normal follow-scene.
func TestPlanScene_Liaise_ClosesOnResolutionAndAllowsFollowScene(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m, redelayID := clDriveToRedelay(t, h)

	// Scene is open and live mid-resolution.
	scene, _ := h.activeScene(0)
	require.NotNil(t, scene, "plan-scene must be open mid-resolution")

	// Finish the liaison (either face 0 cancels the redelay; the liaison still
	// auto-resolves to done/make).
	clSubmitReveal(t, h, redelayID, 0, 0)
	clSubmitReveal(t, h, redelayID, 1, 3)

	refreshed, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	require.Equal(t, model.PlanResolved, refreshed.Status)

	closed, _ := h.activeScene(0)
	assert.Nil(t, closed, "the plan-scene must close the instant the liaison resolves")

	// The row's actual focus player (not necessarily the liaison's preparer or
	// partner — plans resolve for their preparer, turns for the focus player)
	// can still set the ordinary follow-scene on this same row, unaffected by
	// the plan-scene that opened and closed earlier on it.
	focusIdx := h.focusPlayerIdx()
	holdingAsset, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[focusIdx].ID,
		CreatorID: h.tg.Players[focusIdx].ID, AssetType: model.AssetHolding, Name: "The Old Bridge",
	})
	require.NoError(t, err)

	scenesPath := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/scenes"
	code, body := h.post(focusIdx, scenesPath, map[string]any{
		"location_holding_id": holdingAsset.ID,
		"time_elapsed":        "moments",
		"present_peer_ids":    []int64{},
	})
	require.Equalf(t, http.StatusCreated, code, "follow-scene creation must succeed: %v", body)

	sceneBlob, _ := body["scene"]
	require.NotNil(t, sceneBlob)
	newScene, _ := h.activeScene(0)
	require.NotNil(t, newScene)
	assert.Equal(t, model.SceneKindTurn, newScene.Kind, "the follow-scene is an ordinary turn-scene")
	require.NotNil(t, newScene.ResolvedPlanID)
	assert.Equal(t, m.plan.ID, *newScene.ResolvedPlanID,
		"the follow-scene must attach to the liaison that just resolved on this row")
}

// TestPlanScene_ProposeDecree_JoinCouncilAddsPeer proves an eligible
// lower-ranked player who leverages into the council (join-council) is added
// as a plan-scene peer at that moment — rejected speaking as their main
// character beforehand, allowed immediately after joining.
func TestPlanScene_ProposeDecree_JoinCouncilAddsPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Default seeding ranks players[0] highest on every track, so the preparer
	// (players[0], the initial focus player) auto-seats alone; players[1] and
	// players[2] are both eligible to leverage-join.
	notes := "a new decree"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanProposeDecree,
		PreparationNotes: &notes,
	})
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "Propose Decree has no plan-level roll at kickoff")

	scene, peers := h.activeScene(0)
	require.NotNil(t, scene)
	require.NotNil(t, scene.PlanID)
	assert.Equal(t, plan.ID, *scene.PlanID)

	joinerMC := h.mainCharacterID(1)
	preparerMC := h.mainCharacterID(0)
	byAsset := map[int64]bool{}
	for _, p := range peers {
		byAsset[p.PeerAssetID] = true
	}
	assert.True(t, byAsset[preparerMC], "auto-seated preparer must be a peer from the start")
	assert.False(t, byAsset[joinerMC], "a not-yet-joined eligible player must not be a peer yet")

	// Before joining, they cannot speak as their main character.
	code, body := h.postMessage(1, "I have thoughts on this decree", joinerMC)
	assert.Equalf(t, http.StatusBadRequest, code,
		"an eligible-but-unjoined player must be rejected: %v", body)

	// Leverage one asset to join the council.
	asset, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: h.tg.Players[1].ID,
		CreatorID: h.tg.Players[1].ID, AssetType: model.AssetArtifact, Name: "a favor owed",
	})
	require.NoError(t, err)
	joinPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/join-council"
	code, body = h.post(1, joinPath, map[string]any{"asset_ids": []int64{asset.ID}})
	require.Equalf(t, http.StatusOK, code, "join-council: %v", body)

	// The joiner's main character is now a peer, and they can speak in character.
	_, peersAfter := h.activeScene(0)
	byAssetAfter := map[int64]bool{}
	for _, p := range peersAfter {
		byAssetAfter[p.PeerAssetID] = true
	}
	assert.True(t, byAssetAfter[joinerMC], "join-council must add a plan-scene peer row")

	code, body = h.postMessage(1, "I have thoughts on this decree", joinerMC)
	assert.Equalf(t, http.StatusCreated, code,
		"a freshly-joined council member must be able to speak in character: %v", body)
}

// TestPlanScene_HostFestivity_FullRosterFromStartAndClosesOnEnd proves Host
// Festivity's plan-scene seeds every player's main character as a peer from
// the moment it opens (there is no separate "join as guest" action — hfRoster
// is the full table), and that the scene closes when the host ends the event.
func TestPlanScene_HostFestivity_FullRosterFromStartAndClosesOnEnd(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	plan, hostIdx := hfPrepareToSocializing(t, h)
	guestIdx := (hostIdx + 1) % 2

	scene, peers := h.activeScene(0)
	require.NotNil(t, scene, "resolving a festivity must open a plan-scene")
	require.NotNil(t, scene.PlanID)
	assert.Equal(t, plan.ID, *scene.PlanID)

	hostMC := h.mainCharacterID(hostIdx)
	guestMC := h.mainCharacterID(guestIdx)
	byAsset := map[int64]bool{}
	for _, p := range peers {
		byAsset[p.PeerAssetID] = true
	}
	assert.True(t, byAsset[hostMC], "the host must be a peer from the start")
	assert.True(t, byAsset[guestMC], "every player attends from the start — no join step")

	// The guest can speak in character immediately, before rolling or opting out.
	code, body := h.postMessage(guestIdx, "what a lovely party", guestMC)
	require.Equalf(t, http.StatusCreated, code, "guest speaking as MC before acting: %v", body)

	// Wind the event down: the guest opts out, the host takes both their earned
	// makes (one for hosting, one for the guest's opt-out — EarnedHostMakes
	// counts every roster member's Host/OptOut/Mar outcome), then ends the event.
	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	code, body = h.post(guestIdx, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "guest opt_out: %v", body)

	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "A New Face", "peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusOK, code, "host-choice 1: %v", body)
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "Another New Face", "peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusOK, code, "host-choice 2: %v", body)

	endPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/end-event"
	code, body = h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusOK, code, "end-event: %v", body)

	closed, _ := h.activeScene(0)
	assert.Nil(t, closed, "the plan-scene must close once the festivity ends")
}

// TestPlanScene_ChronicleHistories_AllPlayersFromStart proves Chronicle
// Histories seeds every player as a plan-scene peer immediately at kickoff —
// before the roll (and therefore before make-vs-mar is known) — since a Mar
// outcome requires every player to submit a choice during the scene, and the
// outcome isn't knowable until after the plan-scene has already opened. Also
// proves EmitPlanResolved's scene-close fires for a "cancelled" result, not
// just make/mar (closePlanSceneIfAny has no result-specific branch — the same
// code path handles all three, this just exercises the cancelled leg
// directly since none of the four plan-scene types has a real in-play
// cancellation route today).
func TestPlanScene_ChronicleHistories_AllPlayersFromStart(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "the fall of the old keep"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanChronicleHistories,
		PreparationNotes: &notes,
	})
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "Chronicle Histories has no plan-level roll at kickoff")

	scene, peers := h.activeScene(0)
	require.NotNil(t, scene)
	require.NotNil(t, scene.PlanID)
	assert.Equal(t, plan.ID, *scene.PlanID)

	byAsset := map[int64]bool{}
	for _, p := range peers {
		byAsset[p.PeerAssetID] = true
	}
	for i := range h.tg.Players {
		mc := h.mainCharacterID(i)
		assert.Truef(t, byAsset[mc], "player index %d's MC must be a peer from the start", i)
	}

	// A non-preparer can already speak in character, before any roll/outcome.
	code, body := h.postMessage(2, "I remember this place.", h.mainCharacterID(2))
	require.Equalf(t, http.StatusCreated, code, "non-preparer speaking as MC before the roll: %v", body)

	// EndScene refuses this plan-scene like any other.
	endPath := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/end-scene"
	code, body = h.post(h.focusPlayerIdx(), endPath, nil)
	assert.Equalf(t, http.StatusConflict, code, "end-scene must refuse a plan-scene: %v", body)

	// Simulate a cancellation the way every real caller does (e.g. reveals.go's
	// no-room-on-the-record path): flip the plan to PlanCancelled status (result
	// stays nil — the plans_result_check CHECK only allows 'make'/'mar'), then
	// emit the resolution, which closes any open plan-scene.
	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NoError(t, h.q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
		ID: plan.ID, Status: model.PlanCancelled,
	}))
	EmitPlanResolved(ctx, h.q, h.manager, refreshed, "cancelled")

	closed, _ := h.activeScene(0)
	assert.Nil(t, closed, "a cancelled result must close the plan-scene too")
}
