//go:build integration

// handler/plan_reaim_integration_test.go — a plan whose prepare-time target is
// gone by the time it resolves re-aims instead of acting on the corpse.
//
// Audit Tier 1 #5: Exchange Courtiers' OnResolve was `return nil, nil`, so a
// peer destroyed between prepare and resolve was still "transferred" — an audit
// agent watched a main character change hands for a destroyed asset. The fix is
// a class (plan_reaim.go): Exchange Courtiers, Spread Rumors and Make Demands
// each hold a prepare-time reference across the delay, and each pauses at
// resolve for the preparer to re-aim (book p.12) when it has stopped standing.
//
// Every test here pins one of the three outcomes of that check: the aim still
// stands (nothing changes), it is stale and something valid remains (pause,
// name the preparer, refuse every other route, take a choice, carry on), or it is
// stale and nothing remains (close with no effect — and say so in the log).

package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

func reaimPath(planID int64) string { return "/api/plans/" + itoa(planID) + "/reaim" }

// destroyAssets marks assets destroyed the way a break that removes the last
// marginalia does (the row survives; is_destroyed flips).
func (h *planLifecycle) destroyAssets(ids ...int64) {
	h.t.Helper()
	for _, id := range ids {
		require.NoError(h.t, h.q.DestroyAsset(context.Background(), id))
	}
}

// reaimOptions fetches GET /plans/{id}/reaim as a player and returns the
// response body.
func (h *planLifecycle) reaimOptions(playerIdx int, planID int64) map[string]any {
	h.t.Helper()
	code, body := h.get(playerIdx, reaimPath(planID))
	require.Equalf(h.t, http.StatusOK, code, "GET reaim: %v", body)
	return body
}

// optionAssetIDs returns the target_asset_id of every option in a reaim body.
func optionAssetIDs(body map[string]any) []int64 {
	var out []int64
	opts, _ := body["options"].([]any)
	for _, o := range opts {
		m := o.(map[string]any)
		if id, ok := m["target_asset_id"].(float64); ok {
			out = append(out, int64(id))
		}
	}
	return out
}

func optionPlanIDs(body map[string]any) []int64 {
	var out []int64
	opts, _ := body["options"].([]any)
	for _, o := range opts {
		m := o.(map[string]any)
		if id, ok := m["target_plan_id"].(float64); ok {
			out = append(out, int64(id))
		}
	}
	return out
}

func (h *planLifecycle) reaimOwed(planID int64) (owed bool, reason string) {
	h.t.Helper()
	plan, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(h.t, err)
	ra := loadResolutionData(plan.ResolutionData).ReAim
	if ra == nil {
		return false, ""
	}
	return ra.Owed, ra.Reason
}

// logBodies returns every system-post body for the given code in the game.
func (h *planLifecycle) logBodies(code string) []string {
	h.t.Helper()
	return prologuePostsByCode(h.t, h.q, h.tg.Game.ID, code)
}

// ── Exchange Courtiers ───────────────────────────────────────────────────────

// ecPrepared prepares an EC against a freshly seeded peer of the next player
// and returns the pieces every EC re-aim test needs.
func ecPrepared(t *testing.T, h *planLifecycle) (plan dbgen.Plan, prepIdx, targetIdx, thirdIdx int, targetPeer int64) {
	t.Helper()
	prepIdx = h.focusPlayerIdx()
	targetIdx = (prepIdx + 1) % len(h.tg.Players)
	thirdIdx = (prepIdx + 2) % len(h.tg.Players)
	targetPeer = h.seedPeer(targetIdx, "doomed courtier")
	notes := "poach the courtier"
	plan = h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[targetIdx].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	return plan, prepIdx, targetIdx, thirdIdx, targetPeer
}

// The reported case: the targeted peer is destroyed during the five-row delay.
// The plan must not open a fair trade over it; it pauses, names the preparer,
// refuses every other route, accepts a valid new peer, and then runs the ordinary
// fair-trade → transfer flow against THAT peer. The destroyed one never moves.
func TestReAim_ExchangeCourtiers_DestroyedPeer_ReAimsThenTradeRunsOnTheNewPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, targetIdx, thirdIdx, doomed := ecPrepared(t, h)
	prepID := h.tg.Players[prepIdx].ID
	thirdID := h.tg.Players[thirdIdx].ID
	fresh := h.seedPeer(thirdIdx, "fresh courtier")
	requested := h.seedPeer(prepIdx, "requested peer")

	h.destroyAssets(doomed) // five rows pass; the courtier is broken
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolveAndCast(plan.ID))

	// Paused, and the bar names the preparer — not the target, who has nothing to do.
	owed, reason := h.reaimOwed(plan.ID)
	require.True(t, owed, "a destroyed target must pause the plan")
	assert.Contains(t, reason, "doomed courtier")
	assert.Contains(t, reason, "destroyed")
	h.assertWaitees("re-aim owed", model.RowStateAwaitPlanReAim, prepID)
	assert.True(t, anyContains(h.logBodies("plan.exchange_courtiers"), "lost its mark"),
		"the pause itself is logged")

	// Backend guard: the plan's own routes are refused while the re-aim is owed,
	// so a stale tab can't open the fair-trade step over the dead peer.
	code, body := ecOffer(t, h, targetIdx, plan.ID, requested)
	assert.Equalf(t, http.StatusConflict, code, "fair-trade must be refused while paused: %v", body)

	// The options are the living peers of players other than the preparer — the
	// destroyed one is not among them, nor is any of the preparer's own.
	opts := optionAssetIDs(h.reaimOptions(thirdIdx, plan.ID)) // a bystander can read them
	assert.Contains(t, opts, fresh)
	assert.NotContains(t, opts, doomed)
	assert.NotContains(t, opts, requested)

	// Only the preparer chooses, and only a listed option.
	code, _ = h.post(targetIdx, reaimPath(plan.ID),
		map[string]any{"target_player_id": thirdID, "target_asset_id": fresh})
	assert.Equal(t, http.StatusForbidden, code, "a bystander may not re-aim")
	code, _ = h.post(prepIdx, reaimPath(plan.ID),
		map[string]any{"target_player_id": h.tg.Players[targetIdx].ID, "target_asset_id": doomed})
	assert.Equal(t, http.StatusBadRequest, code, "the destroyed peer is not a valid new aim")
	code, _ = h.post(prepIdx, reaimPath(plan.ID),
		map[string]any{"target_player_id": thirdID, "target_asset_id": requested})
	assert.Equal(t, http.StatusBadRequest, code, "a peer the preparer holds is not a valid aim")

	code, body = h.post(prepIdx, reaimPath(plan.ID),
		map[string]any{"target_player_id": thirdID, "target_asset_id": fresh})
	require.Equalf(t, http.StatusOK, code, "re-aim: %v", body)

	// Retargeted, un-paused, and the usual EC flow opens — now owed by the NEW target.
	reloaded, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.TargetAssetID)
	assert.Equal(t, fresh, *reloaded.TargetAssetID)
	assert.Equal(t, thirdID, *reloaded.TargetPlayerID)
	owed, _ = h.reaimOwed(plan.ID)
	assert.False(t, owed)
	assert.True(t, anyContains(h.logBodies("plan.exchange_courtiers"), "re-aimed their Exchange Courtiers at"))
	h.assertWaitees("fair trade opens on the new target", model.RowStateAwaitCourtierResponse, thirdID)

	code, body = ecOffer(t, h, thirdIdx, plan.ID, requested)
	require.Equalf(t, http.StatusOK, code, "offer: %v", body)
	code, body = h.post(prepIdx, "/api/plans/"+itoa(plan.ID)+"/fair-trade", map[string]any{"action": "accept"})
	require.Equalf(t, http.StatusOK, code, "accept: %v", body)

	got, err := h.q.GetAssetByID(ctx, fresh)
	require.NoError(t, err)
	assert.Equal(t, prepID, got.OwnerID, "the re-aimed peer is the one that changes hands")
	dead, err := h.q.GetAssetByID(ctx, doomed)
	require.NoError(t, err)
	assert.True(t, dead.IsDestroyed)
	assert.Equal(t, h.tg.Players[targetIdx].ID, dead.OwnerID, "the destroyed peer never transfers")
}

// A peer that moved to a third player is no longer "the target player's
// courtier": the difficulty is that player's power status and the fair-trade
// premise names them. The plan re-aims — and the peer under its NEW owner is a
// legal choice (the intent, taking that courtier, may still make sense).
func TestReAim_ExchangeCourtiers_PeerChangedHands_IsStaleAndReAimable(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, _, thirdIdx, peer := ecPrepared(t, h)
	thirdID := h.tg.Players[thirdIdx].ID

	require.NoError(t, h.q.TransferAsset(ctx, dbgen.TransferAssetParams{ID: peer, OwnerID: thirdID}))
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolveAndCast(plan.ID))

	owed, reason := h.reaimOwed(plan.ID)
	require.True(t, owed, "a target peer that changed hands must pause the plan")
	assert.Contains(t, reason, "no longer belongs")

	opts := h.reaimOptions(prepIdx, plan.ID)
	assert.Contains(t, optionAssetIDs(opts), peer, "the same peer under its new owner is a valid re-aim")
	code, body := h.post(prepIdx, reaimPath(plan.ID),
		map[string]any{"target_player_id": thirdID, "target_asset_id": peer})
	require.Equalf(t, http.StatusOK, code, "re-aim: %v", body)
	reloaded, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, thirdID, *reloaded.TargetPlayerID, "difficulty now reads the new owner's power status")
}

// A target that still stands is left completely alone — the check must not add a
// gate to every Exchange Courtiers (principle 3).
func TestReAim_ExchangeCourtiers_ValidTarget_NoPause(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, targetIdx, _, _ := ecPrepared(t, h)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolveAndCast(plan.ID))

	owed, _ := h.reaimOwed(plan.ID)
	assert.False(t, owed)
	h.assertWaitees("straight to the fair trade", model.RowStateAwaitCourtierResponse, h.tg.Players[targetIdx].ID)
	code, _ := h.post(prepIdx, reaimPath(plan.ID), map[string]any{})
	assert.Equal(t, http.StatusConflict, code, "nothing to re-aim while the target stands")
}

// Nobody else has a living peer: there is nothing to re-aim at. The plan closes
// with no effect — and the log says why, which is the difference between a plan
// that fizzled and a plan that looks broken.
func TestReAim_ExchangeCourtiers_NoValidTarget_ClosesWithNoEffectAndLogs(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, _, _, doomed := ecPrepared(t, h)

	for i, p := range h.tg.Players {
		if i == prepIdx {
			continue
		}
		owned, err := h.q.ListAssetsByOwner(ctx, p.ID)
		require.NoError(t, err)
		for _, a := range owned {
			h.destroyAssets(a.ID)
		}
	}
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	closed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, closed.Status, "no valid aim: the plan closes rather than stalling the row")
	assert.Nil(t, closed.Result, "closed with no effect — neither a make nor a mar")
	owed, _ := h.reaimOwed(plan.ID)
	assert.False(t, owed, "nothing is owed when there is nothing to choose")
	lines := h.logBodies("plan.exchange_courtiers")
	assert.True(t, anyContains(lines, "has nothing left to act on"), "the close must say why: %v", lines)
	assert.True(t, anyContains(lines, "doomed courtier"), "and name the peer that was lost: %v", lines)

	dead, err := h.q.GetAssetByID(ctx, doomed)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[(prepIdx+1)%3].ID, dead.OwnerID, "the destroyed peer stays where it was")
}

// The last line of defence: whatever route reaches it, a destroyed asset does
// not change hands.
func TestTakeAssetEffect_RefusesADestroyedAsset(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()
	peer := h.seedPeer(0, "gone")
	h.destroyAssets(peer)

	_, err := takeAssetEffect(ctx, h.q, h.manager, h.tg.Game.ID, peer, h.tg.Players[0].ID, h.tg.Players[1].ID)
	require.Error(t, err)
	got, err := h.q.GetAssetByID(ctx, peer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[0].ID, got.OwnerID)
}

// ── Spread Rumors ────────────────────────────────────────────────────────────

func srPrepared(t *testing.T, h *planLifecycle, secretAssetID *int64) (plan dbgen.Plan, prepIdx, otherIdx int, subject int64) {
	t.Helper()
	prepIdx = h.focusPlayerIdx()
	otherIdx = (prepIdx + 1) % len(h.tg.Players)
	subject = h.seedPeer(otherIdx, "the Duchess")
	notes := "she poisons her own wine"
	plan = h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &subject,
		PreparationNotes: &notes,
		SecretAssetID:    secretAssetID,
	})
	require.NotNil(t, plan.RowNumber)
	return plan, prepIdx, otherIdx, subject
}

// The rumor's subject is destroyed in the four-row delay. No roll is made over a
// destroyed asset (its difficulty is read off the subject); the preparer picks a
// new subject, the roll is made against THAT, and the rumor row is about it.
func TestReAim_SpreadRumors_DestroyedSubject_ReAimsThenRollsAndSpreadsAboutTheNewSubject(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, otherIdx, subject := srPrepared(t, h, nil)
	prepID := h.tg.Players[prepIdx].ID
	newSubject := h.seedPeer(otherIdx, "the Duke")

	h.destroyAssets(subject)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "no roll over a destroyed subject")

	owed, reason := h.reaimOwed(plan.ID)
	require.True(t, owed)
	assert.Contains(t, reason, "the Duchess")
	h.assertWaitees("re-aim owed", model.RowStateAwaitPlanReAim, prepID)
	_, err := h.q.GetDiceRollByPlanID(ctx, &plan.ID)
	assert.Error(t, err, "the roll must not exist until the plan is re-aimed")

	opts := optionAssetIDs(h.reaimOptions(prepIdx, plan.ID))
	assert.Contains(t, opts, newSubject)
	assert.NotContains(t, opts, subject)

	code, body := h.post(prepIdx, reaimPath(plan.ID), map[string]any{"target_asset_id": subject})
	assert.Equalf(t, http.StatusBadRequest, code, "the destroyed subject is not a valid aim: %v", body)

	code, body = h.post(prepIdx, reaimPath(plan.ID), map[string]any{"target_asset_id": newSubject})
	require.Equalf(t, http.StatusOK, code, "re-aim: %v", body)
	assert.Nil(t, body["roll"], "re-aiming resumes the plan at its pre-roll: the roll waits for the narration")
	roll := h.castPreRoll(plan.ID)
	require.NotNil(t, roll)

	h.forceRoll(roll.ID, "make", 1)
	h.makeChoice(plan.ID, "make", []string{"reveal_source"})

	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1)
	require.NotNil(t, rumors[0].TargetAssetID)
	assert.Equal(t, newSubject, *rumors[0].TargetAssetID, "the rumor is about the re-aimed subject")
	assert.Equal(t, "The rumor, as stated.", rumors[0].Text, "the rumor is the one stated in the pre-roll, after the re-aim")
}

// A rumor is about an asset, not about whoever holds it. A change of owner leaves
// it valid: no pause, a roll straight away, and the difficulty is computed off
// the live owner.
func TestReAim_SpreadRumors_SubjectChangedOwner_StillStands(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, _, _, subject := srPrepared(t, h, nil)
	thirdIdx := (h.focusPlayerIdx() + 2) % 3
	require.NoError(t, h.q.TransferAsset(context.Background(),
		dbgen.TransferAssetParams{ID: subject, OwnerID: h.tg.Players[thirdIdx].ID}))

	h.jumpToRow(*plan.RowNumber)
	roll := h.resolveAndCast(plan.ID)
	require.NotNil(t, roll, "an owner change is not a reason to pause: the roll is made")
	owed, _ := h.reaimOwed(plan.ID)
	assert.False(t, owed)
}

// Nothing in the game is left to be the subject of a rumor: close with no effect,
// log why.
func TestReAim_SpreadRumors_NoValidSubject_ClosesWithNoEffectAndLogs(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, _, _, _ := srPrepared(t, h, nil)

	all, err := h.q.ListAssetsByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	for _, a := range all {
		h.destroyAssets(a.ID)
	}
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	closed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, closed.Status)
	assert.Nil(t, closed.Result)
	assert.True(t, anyContains(h.logBodies("plan.spread_rumors"), "has nothing left to act on"),
		"the close must say why: %v", h.logBodies("plan.spread_rumors"))
}

// A kept-secret rumor stores its draft as a Secret under one of the preparer's
// assets. If THAT asset is destroyed during the delay the plan must carry on as
// normal: the shelter is not the rumor's subject, so there is nothing to re-aim,
// and the rumor that is spread is the one the preparer states in the pre-roll —
// the Secret's text is never read back by the server. (The helper deliberately
// does not treat SecretAssetID as a target.)
func TestReAim_SpreadRumors_DestroyedSecretShelter_DoesNotLoseTheRumorText(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	prepIdx := h.focusPlayerIdx()
	shelter := h.seedPeer(prepIdx, "the confidant")
	plan, _, _, _ := srPrepared(t, h, &shelter)

	h.destroyAssets(shelter)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolveAndCast(plan.ID)
	require.NotNil(t, roll, "the subject stands, so the roll is made as normal")
	h.forceRoll(roll.ID, "make", 1)
	h.makeChoice(plan.ID, "make", []string{"reveal_source"})

	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1)
	assert.Equal(t, "The rumor, as stated.", rumors[0].Text,
		"the stated rumor is published whatever became of the shelter; no placeholder reaches the board")
}

// ── Make Demands ─────────────────────────────────────────────────────────────

// mdPrepared sets up: one player's Exchange Courtiers on a future row, and
// another player's Make Demands against it. Returns both plans and the indices.
func mdPrepared(t *testing.T, h *planLifecycle) (target, demand dbgen.Plan, demanderIdx, targetPrepIdx, thirdIdx int) {
	t.Helper()
	demanderIdx = h.focusPlayerIdx()
	targetPrepIdx = (demanderIdx + 1) % len(h.tg.Players)
	thirdIdx = (demanderIdx + 2) % len(h.tg.Players)

	// The target plan is prepared by someone else (their turn), then the focus
	// comes back to the demander.
	h.setFocus(targetPrepIdx)
	peer := h.seedPeer(thirdIdx, "a courtier")
	notes := "poach"
	target = h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[thirdIdx].ID,
		TargetAssetID:    &peer,
		PreparationNotes: &notes,
	})
	h.setFocus(demanderIdx)
	dnotes := "I insist"
	demand = h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeDemands,
		TargetPlanID:     &target.ID,
		PreparationNotes: &dnotes,
	})
	require.NotNil(t, demand.RowNumber)
	require.Equal(t, *target.RowNumber, *demand.RowNumber, "a demand lands on its target's row")
	return target, demand, demanderIdx, targetPrepIdx, thirdIdx
}

// The plan demanded against is no longer pending when the demand comes up to
// resolve. The demand pauses, the preparer (the demander) names a different plan
// still pending on the row, the roll is made against it, and the demand now hangs
// off the new target.
func TestReAim_MakeDemands_TargetNoLongerPending_ReAimsToAnotherPendingPlan(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	target, demand, demanderIdx, _, thirdIdx := mdPrepared(t, h)

	// Another player's plan, pending on the same row: the only thing left to demand against.
	other := createPlanOnRow(t, h.q, &h.tg.Game, &h.tg.Players[thirdIdx],
		model.PlanSeekAnswers, model.CategoryEsteem, *demand.RowNumber)

	require.NoError(t, h.q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{ID: target.ID, Status: model.PlanResolved}))
	h.jumpToRow(*demand.RowNumber)
	require.Nil(t, h.resolveAndCast(demand.ID), "no roll while the demand has nothing to demand against")

	owed, reason := h.reaimOwed(demand.ID)
	require.True(t, owed, "a demand whose target is no longer pending must pause")
	assert.Contains(t, reason, "no longer waiting to resolve")
	h.assertWaitees("re-aim owed", model.RowStateAwaitPlanReAim, h.tg.Players[demanderIdx].ID)
	code, _ := h.post(demanderIdx, "/api/plans/"+itoa(demand.ID)+"/draft-choice",
		map[string]any{"option": "keep_assets"})
	assert.Equal(t, http.StatusConflict, code, "the draft cannot start while the demand is paused")

	opts := optionPlanIDs(h.reaimOptions(demanderIdx, demand.ID))
	assert.Equal(t, []int64{other.ID}, opts, "only a pending, non-own, demandable plan on this row qualifies")

	code, body := h.post(demanderIdx, reaimPath(demand.ID), map[string]any{"target_plan_id": target.ID})
	assert.Equalf(t, http.StatusBadRequest, code, "the finished plan is not a valid aim: %v", body)
	code, body = h.post(demanderIdx, reaimPath(demand.ID), map[string]any{"target_plan_id": other.ID})
	require.Equalf(t, http.StatusOK, code, "re-aim: %v", body)
	require.NotNil(t, body["roll"], "re-aiming resumes the demand: its roll is made now")

	reloaded, err := h.q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.TargetedPlanID)
	assert.Equal(t, other.ID, *reloaded.TargetedPlanID)
	assert.True(t, anyContains(h.logBodies("plan.make_demands"), "re-aimed their Make Demands at"))
}

// A target that is still pending is left alone.
func TestReAim_MakeDemands_PendingTarget_NoPause(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	_, demand, _, _, _ := mdPrepared(t, h)
	h.jumpToRow(*demand.RowNumber)
	require.NotNil(t, h.resolveAndCast(demand.ID), "a live target: the demand rolls as normal")
	owed, _ := h.reaimOwed(demand.ID)
	assert.False(t, owed)
}

// Nothing else is pending on the row to demand against: close with no effect, log why.
func TestReAim_MakeDemands_NoValidTarget_ClosesWithNoEffectAndLogs(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	target, demand, _, _, _ := mdPrepared(t, h)

	require.NoError(t, h.q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{ID: target.ID, Status: model.PlanResolved}))
	h.jumpToRow(*demand.RowNumber)
	require.Nil(t, h.resolveAndCast(demand.ID))

	closed, err := h.q.GetPlanByID(ctx, demand.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, closed.Status)
	assert.Nil(t, closed.Result)
	assert.True(t, anyContains(h.logBodies("plan.make_demands"), "has nothing left to act on"),
		"the close must say why: %v", h.logBodies("plan.make_demands"))
	// A fizzled demand is not a made one: it must not govern anything.
	governing, winners, err := DemandWinnersForTargetPlan(ctx, h.q, &target)
	require.NoError(t, err)
	assert.Nil(t, governing)
	assert.Empty(t, winners)
}
