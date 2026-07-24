//go:build integration

// handler/plan_make_introductions_integration_test.go — per-peer mar coverage
// for Make Introductions. The audit found MI's mar was a flat no-op with a
// bogus "center" option; the rules resolve each introduced peer individually:
//
//   - other_retinue  → peer joins another player's retinue, who describes them
//   - broken_arrival → another player authors the peer's marginalia
//   - delayed        → arrival rescheduled d6 rows ahead
//   - broken_journey → preparer writes the peer AND the mark of the journey,
//     which arrives torn
//
// Every one of them turns on the draft-peer rule (D4): the peers named pre-roll
// have no `assets` row, and arriving is what creates them. So these tests check
// the absence of a row as carefully as its presence.
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

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// miToMar prepares an MI plan with peerCount peers, names them, forces a mar
// roll, and enters per-peer resolution. Returns the plan, the preparer's index,
// and the recorded drafts.
func miToMar(t *testing.T, h *planLifecycle, peerCount int) (planID int64, preparerIdx int, drafts []game.DraftPeer) {
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

	// Enter per-peer mar resolution (preparer records the mar result).
	h.makeChoice(plan.ID, "mar", []string{})

	return plan.ID, preparerIdx, miDraftsOf(t, h, plan.ID)
}

// miDraftsOf reads a plan's drafts straight out of resolution_data.
func miDraftsOf(t *testing.T, h *planLifecycle, planID int64) []game.DraftPeer {
	t.Helper()
	plan, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(t, err)
	rd := loadResolutionData(plan.ResolutionData)
	require.NotNil(t, rd.MakeIntroductions)
	return rd.MakeIntroductions.Drafts
}

// miAssetNamed finds a live asset by name, or nil. Used to assert that a draft
// has (or hasn't) crossed into being a real asset.
func miAssetNamed(t *testing.T, h *planLifecycle, name string) *dbgen.Asset {
	t.Helper()
	assets, err := h.q.ListAssetsByGame(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

// TestMakeIntroductions_PerformStepsWinnerNamesPeers proves a Make Demands
// perform_steps win lets the demander drive the pre-roll naming step
// (create-peer) in the preparer's stead, the preparer is locked out, and the
// keep_assets win routes the peer into the demander's retinue when they arrive.
func TestMakeIntroductions_PerformStepsWinnerNamesPeers(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	preparerIdx := h.focusPlayerIdx()
	demanderIdx := (preparerIdx + 1) % 3

	notes := "introductions"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeIntroductions,
		PeerCount:        1,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "MI defers its roll until peers are named")

	// A resolved, made demand hands perform_steps + keep_assets to the demander.
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[demanderIdx].ID,
		game.DemandOptionKeepAssets:   h.tg.Players[demanderIdx].ID,
	})

	createPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/create-peer"

	// The preparer is locked out of naming peers now that the demander holds it.
	code, body := h.post(preparerIdx, createPath, map[string]any{"name": "Preparer's pick"})
	require.Equalf(t, http.StatusForbidden, code,
		"preparer must be locked out when the demander won perform_steps: %v", body)

	// The demander names the peer instead.
	code, body = h.post(demanderIdx, createPath, map[string]any{"name": "Demander's pick"})
	require.Equalf(t, http.StatusCreated, code,
		"perform_steps winner should drive create-peer: %v", body)

	drafts := miDraftsOf(t, h, plan.ID)
	require.Len(t, drafts, 1)
	assert.Nil(t, miAssetNamed(t, h, "Demander's pick"), "naming a peer must not create an asset")

	// Roll a make and bring them in through the arrival form.
	finalizePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/finalize-peers"
	code, body = h.post(demanderIdx, finalizePath, nil)
	require.Equalf(t, http.StatusCreated, code, "finalize-peers: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap)
	h.forceRoll(int64(rollMap["id"].(float64)), "make", 0)

	code, body = h.post(demanderIdx, "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/make-choice",
		map[string]any{"result": "make", "choices": []string{}})
	require.Equalf(t, http.StatusOK, code, "make-choice: %v", body)

	arrivalPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/introductions-arrival"
	code, body = h.post(demanderIdx, arrivalPath, map[string]any{
		"draft_id": drafts[0].ID, "name": drafts[0].Name, "marginalia": "owes the demander a favour",
	})
	require.Equalf(t, http.StatusCreated, code, "arrival: %v", body)

	peer := miAssetNamed(t, h, "Demander's pick")
	require.NotNil(t, peer, "the peer should exist once they arrive")
	assert.Equal(t, h.tg.Players[demanderIdx].ID, peer.OwnerID,
		"keep_assets winner owns the introduced peer")
	margs, err := h.q.ListMarginaliaByAsset(ctx, peer.ID)
	require.NoError(t, err)
	assert.Len(t, margs, 1, "an arriving peer is never blank")
}

func TestMakeIntroductions_Mar_OtherRetinue_RecipientWritesAndReceives(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	planID, preparerIdx, drafts := miToMar(t, h, 2)
	require.Len(t, drafts, 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	other := h.tg.Players[otherIdx].ID

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"

	// Send the first peer to another player's retinue.
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "other_retinue",
		"target_player_id": other,
	})
	require.Equalf(t, http.StatusOK, code, "other_retinue: %v", body)

	// D6: the peer is still a draft — the recipient owes the marginalia, and the
	// table is waiting on them, not the preparer.
	assert.Nil(t, miAssetNamed(t, h, drafts[0].Name),
		"an other_retinue peer must not exist until the recipient describes them")
	h.assertWaitees("other_retinue awaiting the recipient",
		model.RowStateAwaitIntroductionsMarginalia, other)

	// Completion is blocked: the second peer is unresolved.
	completePath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/complete"
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked: %v", body)

	// Resolving the same peer twice is rejected.
	code, _ = h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "delayed",
	})
	assert.Equal(t, http.StatusConflict, code, "double-resolving a peer should 409")

	// Resolve the second peer (delayed).
	code, body = h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[1].ID, "outcome": "delayed",
	})
	require.Equalf(t, http.StatusOK, code, "delayed: %v", body)

	// Still blocked — the recipient hasn't written yet.
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code,
		"complete should wait on the recipient's marginalia: %v", body)

	margPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-marginalia"
	// Only the recipient may write — the preparer cannot.
	code, _ = h.post(preparerIdx, margPath, map[string]any{
		"draft_id": drafts[0].ID, "text": "not mine to write",
	})
	assert.Equal(t, http.StatusForbidden, code, "non-author write should 403")

	code, body = h.post(otherIdx, margPath, map[string]any{
		"draft_id": drafts[0].ID, "text": "a useful ally",
	})
	require.Equalf(t, http.StatusOK, code, "recipient write: %v", body)
	h.complete(planID)

	moved := miAssetNamed(t, h, drafts[0].Name)
	require.NotNil(t, moved, "the peer arrives when the recipient describes them")
	assert.Equal(t, other, moved.OwnerID, "peer should be in the other player's retinue")
	margs, err := h.q.ListMarginaliaByAsset(ctx, moved.ID)
	require.NoError(t, err)
	require.Len(t, margs, 1)
	assert.Equal(t, "a useful ally", margs[0].Text)
}

func TestMakeIntroductions_Mar_BrokenArrival_AuthorWritesMarginalia(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	planID, preparerIdx, drafts := miToMar(t, h, 1)
	authorIdx := (preparerIdx + 1) % len(h.tg.Players)

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "broken_arrival",
		"target_player_id": h.tg.Players[authorIdx].ID,
	})
	require.Equalf(t, http.StatusOK, code, "broken_arrival: %v", body)
	assert.Nil(t, miAssetNamed(t, h, drafts[0].Name),
		"a broken-arrival peer must not exist until the author describes them")
	h.assertWaitees("broken_arrival awaiting the author",
		model.RowStateAwaitIntroductionsMarginalia, h.tg.Players[authorIdx].ID)

	// Blocked until the assigned author writes the marginalia.
	completePath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/complete"
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked: %v", body)

	margPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-marginalia"
	// Only the assigned author may write — the preparer cannot.
	code, _ = h.post(preparerIdx, margPath, map[string]any{
		"draft_id": drafts[0].ID, "text": "not mine to write",
	})
	assert.Equal(t, http.StatusForbidden, code, "non-author write should 403")

	code, body = h.post(authorIdx, margPath, map[string]any{
		"draft_id": drafts[0].ID, "text": "a cruel rumor",
	})
	require.Equalf(t, http.StatusOK, code, "author write: %v", body)

	h.complete(planID)

	peer := miAssetNamed(t, h, drafts[0].Name)
	require.NotNil(t, peer, "the peer arrives when the author describes them")
	assert.Equal(t, h.tg.Players[preparerIdx].ID, peer.OwnerID,
		"a broken-arrival peer still joins the plan recipient's retinue")
	margs, err := h.q.ListMarginaliaByAsset(ctx, peer.ID)
	require.NoError(t, err)
	require.Len(t, margs, 1)
	assert.Equal(t, "a cruel rumor", margs[0].Text)
	assert.False(t, margs[0].IsTorn)
}

// TestMakeIntroductions_Mar_BrokenJourney_ArrivesBrokenNotDestroyed covers the
// audit's destroy-on-arrival bug: the peer used to be created blank, given one
// marginalia and immediately have it torn, which destroyed them. They now
// arrive carrying two — who they are, and what the road cost them — with only
// the latter torn.
func TestMakeIntroductions_Mar_BrokenJourney_ArrivesBrokenNotDestroyed(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	planID, preparerIdx, drafts := miToMar(t, h, 1)
	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"

	// Both texts are required — one alone can only destroy the peer.
	code, _ := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "broken_journey", "text": "a weary traveller",
	})
	assert.Equal(t, http.StatusBadRequest, code, "broken_journey needs both texts")

	code, body := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "broken_journey",
		"text": "a weary traveller", "journey_text": "limped in, half-starved",
	})
	require.Equalf(t, http.StatusOK, code, "broken_journey: %v", body)

	h.complete(planID)

	peer := miAssetNamed(t, h, drafts[0].Name)
	require.NotNil(t, peer, "a broken journey is survivable — the peer arrives")
	margs, err := h.q.ListMarginaliaByAsset(ctx, peer.ID)
	require.NoError(t, err)
	require.Len(t, margs, 2)
	byText := map[string]bool{}
	for _, m := range margs {
		byText[m.Text] = m.IsTorn
	}
	assert.False(t, byText["a weary traveller"], "the peer's own marginalia stays intact")
	assert.True(t, byText["limped in, half-starved"], "the journey's mark arrives torn")
}

// TestMakeIntroductions_Mar_Delayed_ArrivesOnSyntheticPlan proves a delayed peer
// stays a draft while travelling — no asset to break, take or stake — and
// materializes only when their synthetic arrival plan resolves.
func TestMakeIntroductions_Mar_Delayed_ArrivesOnSyntheticPlan(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Row 1 keeps every d6 result inside the record, so this never flakes into
	// the "lost on the journey" branch.
	h.jumpToRow(1)
	planID, preparerIdx, drafts := miToMar(t, h, 1)

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "delayed",
	})
	require.Equalf(t, http.StatusOK, code, "delayed: %v", body)
	h.complete(planID)

	assert.Nil(t, miAssetNamed(t, h, drafts[0].Name),
		"a peer still on the road owns no asset row")

	parent, err := h.q.GetPlanByID(ctx, planID)
	require.NoError(t, err)
	parentMI := loadResolutionData(parent.ResolutionData).MakeIntroductions
	require.NotNil(t, parentMI)
	require.Len(t, parentMI.DelayedPeerPlanIDs, 1, "one synthetic arrival plan")

	synthID := parentMI.DelayedPeerPlanIDs[0]
	synth, err := h.q.GetPlanByID(ctx, synthID)
	require.NoError(t, err)
	synthMI := loadResolutionData(synth.ResolutionData).MakeIntroductions
	require.NotNil(t, synthMI)
	assert.True(t, synthMI.DelayedArrival)
	require.NotNil(t, synthMI.DelayedDraft, "the draft rides on the synthetic plan")
	assert.Equal(t, drafts[0].ID, synthMI.DelayedDraft.ID)

	// Play forward to the arrival row and resolve the synthetic plan.
	require.NotNil(t, synth.RowNumber)
	h.jumpToRow(*synth.RowNumber)
	require.Nil(t, h.resolve(synthID), "a delayed arrival has no roll")

	completePath := "/api/plans/" + strconv.FormatInt(synthID, 10) + "/complete"
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code,
		"the arrival plan should not complete before the peer turns up: %v", body)

	arrivalPath := "/api/plans/" + strconv.FormatInt(synthID, 10) + "/introductions-arrival"
	code, body = h.post(preparerIdx, arrivalPath, map[string]any{
		"draft_id": drafts[0].ID, "name": drafts[0].Name, "marginalia": "road-worn but willing",
	})
	require.Equalf(t, http.StatusCreated, code, "delayed arrival: %v", body)

	// The arrival is the plan's whole content, so submitting it closes the plan.
	// A synthetic plan never rolled and so has no outcome CompletePlan could
	// read — left to the preparer it would sit resolving and hold the row.
	closed, err := h.q.GetPlanByID(ctx, synthID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, closed.Status, "the arrival closes its own plan")

	peer := miAssetNamed(t, h, drafts[0].Name)
	require.NotNil(t, peer, "the delayed peer arrives on their row")
	assert.Equal(t, h.tg.Players[preparerIdx].ID, peer.OwnerID)
	margs, err := h.q.ListMarginaliaByAsset(ctx, peer.ID)
	require.NoError(t, err)
	require.Len(t, margs, 1)
	assert.Equal(t, "road-worn but willing", margs[0].Text)
}

// TestMakeIntroductions_Mar_Delayed_LostLeavesNothing pins the other half of the
// delayed outcome: a peer whose arrival row runs past the public record simply
// never happens. Under the old model an asset had already been created and was
// destroyed retroactively, unwinding something that had been in a retinue for
// rows.
func TestMakeIntroductions_Mar_Delayed_LostLeavesNothing(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	planID, preparerIdx, drafts := miToMar(t, h, 1)
	// Row 13 is the last row of the record, so current_row + d6 always exceeds
	// it: the peer is lost whatever the die says. Jumped after preparation,
	// since preparing a delay-3 plan from row 13 trips the endgame gate instead.
	h.jumpToRow(publicRecordRowCount)

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "delayed",
	})
	require.Equalf(t, http.StatusOK, code, "delayed: %v", body)
	h.complete(planID)

	assert.Nil(t, miAssetNamed(t, h, drafts[0].Name), "a lost peer leaves no asset behind")

	parent, err := h.q.GetPlanByID(ctx, planID)
	require.NoError(t, err)
	parentMI := loadResolutionData(parent.ResolutionData).MakeIntroductions
	require.NotNil(t, parentMI)
	assert.Empty(t, parentMI.DelayedPeerPlanIDs, "no arrival plan is scheduled for a lost peer")
	assert.Empty(t, parentMI.Arrivals, "and nothing ever materialized")

	// No tombstone either: the asset never existed, so nothing was destroyed.
	all, err := h.q.ListAllAssetsByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	for _, a := range all {
		assert.NotEqual(t, drafts[0].Name, a.Name, "no destroyed row for a peer who never arrived")
	}
}

// TestMakeIntroductions_Arrival_RequiresMarginalia guards the one invariant D4
// buys: a materialized peer is never blank.
func TestMakeIntroductions_Arrival_RequiresMarginalia(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	preparerIdx := h.focusPlayerIdx()
	notes := "introductions"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeIntroductions,
		PeerCount:        1,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	createPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/create-peer"
	code, body := h.post(preparerIdx, createPath, map[string]any{"name": "Nameless Hopeful"})
	require.Equalf(t, http.StatusCreated, code, "create-peer: %v", body)

	arrivalPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/introductions-arrival"
	drafts := miDraftsOf(t, h, plan.ID)
	require.Len(t, drafts, 1)

	// Before the make step there is nothing to arrive into.
	code, _ = h.post(preparerIdx, arrivalPath, map[string]any{
		"draft_id": drafts[0].ID, "name": drafts[0].Name, "marginalia": "too early",
	})
	assert.Equal(t, http.StatusConflict, code, "arrival before the roll should 409")

	finalizePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/finalize-peers"
	code, body = h.post(preparerIdx, finalizePath, nil)
	require.Equalf(t, http.StatusCreated, code, "finalize-peers: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap)
	h.forceRoll(int64(rollMap["id"].(float64)), "make", 0)
	h.makeChoice(plan.ID, "make", []string{})

	// A blank arrival is refused outright.
	code, _ = h.post(preparerIdx, arrivalPath, map[string]any{
		"draft_id": drafts[0].ID, "name": drafts[0].Name, "marginalia": "   ",
	})
	assert.Equal(t, http.StatusBadRequest, code, "an arriving peer needs one marginalia")
	assert.Nil(t, miAssetNamed(t, h, "Nameless Hopeful"))

	// And an unknown draft is not an arrival at all.
	code, _ = h.post(preparerIdx, arrivalPath, map[string]any{
		"draft_id": "not-a-draft", "name": "Ghost", "marginalia": "never named",
	})
	assert.Equal(t, http.StatusBadRequest, code, "unknown draft should 400")

	code, body = h.post(preparerIdx, arrivalPath, map[string]any{
		"draft_id": drafts[0].ID, "name": drafts[0].Name, "marginalia": "at last, described",
	})
	require.Equalf(t, http.StatusCreated, code, "arrival: %v", body)

	// Arriving twice is refused — the draft is spent.
	code, _ = h.post(preparerIdx, arrivalPath, map[string]any{
		"draft_id": drafts[0].ID, "name": drafts[0].Name, "marginalia": "again?",
	})
	assert.Equal(t, http.StatusConflict, code, "a peer arrives once")
	h.complete(plan.ID)
}
