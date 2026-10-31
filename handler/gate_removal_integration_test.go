//go:build integration

// handler/gate_removal_integration_test.go — the cross-cutting gate-removal pass
// (adr/audit/FINDINGS.md, Tier 4; DESIGN_PRINCIPLES.md principle 3).
//
// The rule under test: a step that asks nothing the server could not work out
// for itself must not be a "Waiting On" gate. Most removals are pinned where the
// plan's own tests already walk the flow (Make Introductions, Make Demands,
// Exchange Courtiers, Spread Propaganda, Propose Decree and Host Festivity tests
// all assert the plan ends on its last real step, with no click). What lives here
// is what has no other home: the Chronicle hole that let a plan complete with its
// steps unspent, the Make Introductions outcome that is forced AND input-free,
// and the festivity that a finishing duel was holding open.

package handler

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/model"
)

// A made Chronicle used to be completable with zero steps: CanComplete has no roll
// access, so it could only enforce a budget copied into resolution_data, and the
// copy was made by the FIRST make-step. Until then it saw no budget and passed.
func TestChronicleHistories_Make_CannotCompleteBeforeTheFirstStep(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := chPrepareToRoll(t, h, "make", 1)

	rd := loadResolutionData(mustPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.ChronicleHistories)
	assert.Equal(t, int16(4), rd.ChronicleHistories.MakeBudget,
		"the budget is the dice result, captured when the dice landed")

	code, body := h.post(preparerIdx, "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/complete", nil)
	require.Equalf(t, http.StatusConflict, code, "a made Chronicle with no steps must not complete: %v", body)
	assert.Equal(t, model.PlanResolving, mustPlan(t, h, plan.ID).Status)
}

// The marred half of the same hole: MarActive / MarRequiredChoices were captured by
// the first mar-choice, so a marred Chronicle nobody had answered completed too.
func TestChronicleHistories_Mar_CannotCompleteBeforeAnyoneChooses(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := chPrepareToRoll(t, h, "mar", 1)

	rd := loadResolutionData(mustPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.ChronicleHistories)
	assert.True(t, rd.ChronicleHistories.MarActive, "the mar gate is armed when the dice land")
	assert.Equal(t, int16(3), rd.ChronicleHistories.MarRequiredChoices, "every player must choose")

	code, body := h.post(preparerIdx, "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/complete", nil)
	require.Equalf(t, http.StatusConflict, code, "a marred Chronicle nobody answered must not complete: %v", body)
}

// "Choose a different one for each new peer": with four peers and three outcomes
// handed out, the last peer's fate is the one remaining — but it is still shown
// to the preparer to take, never dealt behind their back (a player must not feel
// their agency was taken, and seeing the menu teaches what exists). The plan
// ends once the last peer is resolved and the authors have written.
func TestMakeIntroductions_Mar_LastPeerStillChoosesTheOnlyOutcomeLeft(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	planID, preparerIdx, drafts := miToMar(t, h, 4)
	require.Len(t, drafts, 4)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	other := h.tg.Players[otherIdx].ID
	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"

	for i, req := range []map[string]any{
		{"draft_id": drafts[0].ID, "outcome": "other_retinue", "target_player_id": other},
		{"draft_id": drafts[1].ID, "outcome": "broken_arrival", "target_player_id": other},
		{"draft_id": drafts[2].ID, "outcome": "broken_journey", "text": "who arrived", "journey_text": "the road"},
	} {
		code, body := h.post(preparerIdx, marPath, req)
		require.Equalf(t, http.StatusOK, code, "outcome %d: %v", i, body)
	}

	mi := loadResolutionData(mustPlan(t, h, planID).ResolutionData).MakeIntroductions
	require.NotNil(t, mi)
	assert.Nil(t, mi.MarOutcomeFor(drafts[3].ID), "the last peer is left for the preparer to resolve")

	code, body := h.post(preparerIdx, marPath, map[string]any{"draft_id": drafts[3].ID, "outcome": "delayed"})
	require.Equalf(t, http.StatusOK, code, "the last outcome: %v", body)

	// Two peers still owe a marginalia (their authors), so the plan stays open
	// until they write.
	assert.Equal(t, model.PlanResolving, mustPlan(t, h, planID).Status)
	margPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-marginalia"
	for _, d := range drafts[:2] {
		code, body := h.post(otherIdx, margPath, map[string]any{"draft_id": d.ID, "text": "a note on " + d.Name})
		require.Equalf(t, http.StatusOK, code, "author writes for %s: %v", d.Name, body)
	}
	h.requireResolved(planID, "mar")
}

// A make with fewer outcomes spent is NOT forced: three peers can still choose
// among the remaining chips, so the preparer keeps the decision.
func TestMakeIntroductions_Mar_NotForcedWhileAChoiceRemains(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	planID, preparerIdx, drafts := miToMar(t, h, 3)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "other_retinue", "target_player_id": h.tg.Players[otherIdx].ID,
	})
	require.Equalf(t, http.StatusOK, code, "first outcome: %v", body)
	code, body = h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[1].ID, "outcome": "broken_journey", "text": "who arrived", "journey_text": "the road",
	})
	require.Equalf(t, http.StatusOK, code, "second outcome: %v", body)

	mi := loadResolutionData(mustPlan(t, h, planID).ResolutionData).MakeIntroductions
	assert.Len(t, mi.MarOutcomes, 2, "the third peer still has two outcomes to choose between")
	assert.Nil(t, mi.MarOutcomeFor(drafts[2].ID), "…so it is left for the preparer")
}

// ── Plans with an embedded scene stay open until the players are done ────────
//
// The owner's ruling (2026-10-30): Host Festivity, Propose Decree, Chronicle
// Histories and Clandestinely Liaise each open a plan-scene that closes the moment
// the plan resolves. "Wrap the scene" in the book means the players steer toward a
// satisfying close — it does not mean the app may end the scene for them. So the
// last owed action must NOT end these plans; a closing action the players take does.

// A festivity with nothing left owed is still running until the host ends it.
func TestHostFestivity_EndableEventStaysOpenUntilTheHostEndsIt(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)
	for _, g := range []int{g1, g2} {
		code, body := h.post(g, hfPath(plan.ID, "guest-roll"), map[string]any{"action": "opt_out"})
		require.Equalf(t, http.StatusOK, code, "opt out: %v", body)
	}
	for i := range 3 {
		code, body := h.post(hostIdx, hfPath(plan.ID, "host-choice"), map[string]any{
			"choice": "spread_rumor", "rumor_text": "whispers " + strconv.Itoa(i),
		})
		require.Equalf(t, http.StatusOK, code, "host make %d: %v", i, body)
	}

	assert.Equal(t, model.PlanResolving, mustPlan(t, h, plan.ID).Status,
		"everything is settled, but the party goes on until the host calls it")
	h.assertWaitees("only the host's closing remains", model.RowStateAwaitFestivityGuestTurn,
		h.tg.Players[hostIdx].ID)

	code, body := h.post(hostIdx, hfPath(plan.ID, "end-event"), nil)
	require.Equalf(t, http.StatusOK, code, "the host ends the event: %v", body)
	h.requireResolved(plan.ID, "make")
}

// A marred decree's addendum is not the end: the preparer's enactment is the
// beat that closes the scene.
func TestProposeDecree_MarStaysOpenUntilTheLawIsEnacted(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan := pdPrepareAndResolve(t, h, 0)
	preparerIdx := h.preparerIdxFor(plan.ID)
	pdDeclineRemaining(t, h, plan.ID)
	roll := pdCallRoll(t, h, plan.ID, preparerIdx)
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)
	for {
		pd := pdData(t, h, plan.ID)
		next := pd.NextAmender()
		if next == 0 {
			break
		}
		code, body := h.post(pdPlayerIdx(t, h, next), "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/amend-decree",
			map[string]any{"text": "amended"})
		require.Equalf(t, http.StatusOK, code, "amend: %v", body)
	}
	code, body := h.post(preparerIdx, "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/set-addendum",
		map[string]any{"connector": "", "addendum": ""})
	require.Equalf(t, http.StatusOK, code, "addendum: %v", body)

	assert.Equal(t, model.PlanResolving, mustPlan(t, h, plan.ID).Status,
		"the scene is still open: the preparer enacts when it has found its ending")
	assert.Nil(t, pdData(t, h, plan.ID).LawID, "no law until it is enacted")
	pdEnactMar(t, h, plan.ID)
	h.requireResolved(plan.ID, "mar")
}

// pdEnactMar is the preparer's enact-law on a marred decree (no resource).
func pdEnactMar(t *testing.T, h *planLifecycle, planID int64) {
	t.Helper()
	code, body := h.post(h.preparerIdxFor(planID), "/api/plans/"+strconv.FormatInt(planID, 10)+"/enact-law",
		map[string]any{})
	require.Equalf(t, http.StatusOK, code, "enact-law: %v", body)
}
