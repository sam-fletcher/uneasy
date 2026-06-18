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
	gamepkg "uneasy/game"
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

// hfStartRoll posts a guest-roll for players[idx] and returns the (unresolved)
// roll id, leaving that guest mid roll-and-choice.
func hfStartRoll(t *testing.T, h *planLifecycle, planID int64, idx int) int64 {
	t.Helper()
	rollPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/guest-roll"
	code, body := h.post(idx, rollPath, map[string]any{"action": "roll"})
	require.Equalf(t, http.StatusCreated, code, "guest-roll: %v", body)
	return int64(body["roll"].(map[string]any)["id"].(float64))
}

// hfGuestRoll makes players[idx] roll, forces the given outcome, and submits the
// given option — concluding their roll-and-choice. extra carries option params
// (e.g. rumor_text).
func hfGuestRoll(t *testing.T, h *planLifecycle, planID int64, idx int, outcome, choice string, extra map[string]any) {
	t.Helper()
	rollID := hfStartRoll(t, h, planID, idx)
	h.forceRoll(rollID, outcome, 0)
	choicePath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/guest-choice"
	body := map[string]any{"choice": choice}
	for k, v := range extra {
		body[k] = v
	}
	code, resp := h.post(idx, choicePath, body)
	require.Equalf(t, http.StatusOK, code, "guest-choice %s: %v", choice, resp)
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

// TestHostFestivity_HostFreeMake_BenefitsHost proves an extra make benefits the
// HOST, not the triggering guest: introduce_peer adds the new peer to the host's
// own retinue.
func TestHostFestivity_HostFreeMake_BenefitsHost(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)
	hostID := h.tg.Players[hostIdx].ID

	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	choicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-choice"

	// g1 rolls a mar and locks it in (earns the host an extra make).
	hfGuestRollMar(t, h, plan.ID, g1)
	code, body := h.post(g1, choicePath, map[string]any{"choice": "accept_duels"})
	require.Equalf(t, http.StatusOK, code, "g1 mar choice: %v", body)

	// The host doesn't roll — they may not, and don't need to (their earned make
	// is recorded up front).
	code, body = h.post(hostIdx, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusForbidden, code, "host must not be allowed to roll/opt-out: %v", body)
	code, body = h.post(g2, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "g2 opt out: %v", body)

	// The host takes one of their extra makes (no target — it's their spoils).
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice":    "introduce_peer",
		"peer_name": "Gilded Guest",
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
	assert.True(t, found, "the host's extra make adds the peer to the host's own retinue")
}

// TestHostFestivity_HostEarnedMake proves the host is pre-recorded with the
// FestivityOutcomeHost outcome (so they never roll), and may take that earned
// make for themself via host-choice.
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
		"host outcome is pre-recorded as the earned extra make")

	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"

	// The host cannot roll.
	code, body := h.post(hostIdx, rollPath, map[string]any{"action": "roll"})
	require.Equalf(t, http.StatusForbidden, code, "host roll must be forbidden: %v", body)

	// Both guests opt out.
	for _, idx := range []int{g1, g2} {
		code, body = h.post(idx, rollPath, map[string]any{"action": "opt_out"})
		require.Equalf(t, http.StatusOK, code, "guest opt out: %v", body)
	}

	// The host takes one of their earned makes for themself.
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice":    "introduce_peer",
		"peer_name": "Host's Own Guest",
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

// TestHostFestivity_RollAndChoiceLocksTable proves that while a guest is mid
// roll-and-choice (rolled, not yet chosen), no other festivity action may start
// — another guest's roll/opt-out, and the host's make, are all refused — and
// that the lock lifts only once the option is chosen.
func TestHostFestivity_RollAndChoiceLocksTable(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)

	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	choicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-choice"
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"

	// g1 starts a roll and leaves it mid-flight (not yet chosen).
	rollID := hfStartRoll(t, h, plan.ID, g1)

	// g2 may not start a roll, may not opt out, and the host may not take a make.
	code, body := h.post(g2, rollPath, map[string]any{"action": "roll"})
	require.Equalf(t, http.StatusConflict, code, "g2 roll during g1's turn: %v", body)
	code, body = h.post(g2, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusConflict, code, "g2 opt-out during g1's turn: %v", body)
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{"choice": "spread_rumor", "rumor_text": "x"})
	require.Equalf(t, http.StatusConflict, code, "host make during g1's turn: %v", body)

	// g1 resolves their dice and submits a choice — concluding their sequence.
	h.forceRoll(rollID, "mar", 0)
	code, body = h.post(g1, choicePath, map[string]any{"choice": "accept_duels"})
	require.Equalf(t, http.StatusOK, code, "g1 concludes: %v", body)

	// Now the table is free again: g2 can opt out.
	code, body = h.post(g2, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "g2 opt-out after g1 concluded: %v", body)
}

// TestHostFestivity_EndEventGate proves the host may only wind the event down
// once every guest has chosen, every earned make is taken, and every
// outstanding mar (a successful guest's IOU) has been inflicted — and that
// ending resolves the plan as a make.
func TestHostFestivity_EndEventGate(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)

	endPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/end-event"
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"
	insistPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/insist-host-mar"

	// g1 succeeds (holds a mar to inflict); g2 opts out (earns the host a make).
	hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	code, body := h.post(g2, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "g2 opt out: %v", body)

	// Earned makes = host(1) + g2 opt-out(1) = 2; mars outstanding = g1's IOU.
	// Not endable yet.
	code, body = h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusConflict, code, "end before makes/mars settled: %v", body)

	// Host takes both earned makes.
	for i := 0; i < 2; i++ {
		code, body = h.post(hostIdx, hostChoicePath, map[string]any{
			"choice": "introduce_peer", "peer_name": "Guest"})
		require.Equalf(t, http.StatusOK, code, "host make %d: %v", i, body)
	}
	// A third make is refused — only two were earned.
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{"choice": "introduce_peer", "peer_name": "Extra"})
	require.Equalf(t, http.StatusConflict, code, "over-taking makes: %v", body)

	// Still not endable: g1's mar is unspent.
	code, body = h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusConflict, code, "end before g1's mar inflicted: %v", body)

	// g1 inflicts their mar on the host.
	code, body = h.post(g1, insistPath, map[string]any{"mar_option": "accept_duels"})
	require.Equalf(t, http.StatusOK, code, "g1 insist mar: %v", body)

	// Now endable. A non-host may not end it.
	code, body = h.post(g1, endPath, nil)
	require.Equalf(t, http.StatusForbidden, code, "non-host end: %v", body)

	code, body = h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusOK, code, "host ends the event: %v", body)
	assert.Equal(t, "make", body["result"], "a festivity always resolves as a make")

	resolved, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status, "ending resolves the plan")
}

// hfChallengeState reloads the plan and returns its festivity sub-state.
func hfChallengeState(t *testing.T, h *planLifecycle, planID int64) *gamepkg.FestivityResolutionData {
	t.Helper()
	reloaded, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(t, err)
	rd := loadResolutionData(reloaded.ResolutionData)
	return rd.EnsureFestivity()
}

// TestHostFestivity_Make_ChallengeDuel_Accept proves a guest who ROLLS a make
// can reach the challenge_duel option via /challenge-duel (it does NOT go
// through guest-choice), and that the target accepting spawns a nested duel.
//
// Regression: the /challenge-duel guard formerly required
// state.Outcomes[ck] == make, but that entry is only written once a guest has
// spent their make — never before this, its only legitimate caller. So the
// option 403'd on its first and only call, making challenge_duel unreachable.
func TestHostFestivity_Make_ChallengeDuel_Accept(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	challenger := (hostIdx + 1) % len(h.tg.Players)
	target := (hostIdx + 2) % len(h.tg.Players)
	targetID := h.tg.Players[target].ID
	ck := strconv.FormatInt(h.tg.Players[challenger].ID, 10)

	// The challenger rolls and resolves to a make, then opens the duel — exactly
	// the path SocializingTurn.submitMyChoice takes when pickedChoice is
	// challenge_duel (it calls challengeDuel(), not guestChoice()).
	rollID := hfStartRoll(t, h, plan.ID, challenger)
	h.forceRoll(rollID, "make", 0)

	cdPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/challenge-duel"
	code, body := h.post(challenger, cdPath, map[string]any{"target_player_id": targetID})
	require.Equalf(t, http.StatusCreated, code, "challenge-duel after rolling make: %v", body)

	// The make is now spent and recorded as challenge_duel.
	st := hfChallengeState(t, h, plan.ID)
	assert.Equal(t, "make", st.Outcomes[ck], "challenger's make is recorded")
	assert.Equal(t, "challenge_duel", st.GuestMakes[ck], "make is spent on challenge_duel")
	require.NotNil(t, st.PendingChallenge, "a challenge awaits a response")

	// The target accepts → a nested duel is spawned and tracked.
	rcPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/respond-challenge"
	code, body = h.post(target, rcPath, map[string]any{"accept": true})
	require.Equalf(t, http.StatusCreated, code, "accept challenge: %v", body)
	assert.Equal(t, true, body["accepted"], "challenge accepted")
	require.NotNil(t, body["duel_plan_id"], "a nested duel plan is created")

	st = hfChallengeState(t, h, plan.ID)
	assert.Nil(t, st.PendingChallenge, "challenge is cleared on accept")
	require.NotNil(t, st.PendingDuelPlanID, "the nested duel id is tracked")
}

// TestHostFestivity_Make_ChallengeDuel_Decline proves the same reachability and
// that a target without accept_duels may decline — clearing the challenge while
// the challenger's make stays spent (per the house rule).
func TestHostFestivity_Make_ChallengeDuel_Decline(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	challenger := (hostIdx + 1) % len(h.tg.Players)
	target := (hostIdx + 2) % len(h.tg.Players)
	targetID := h.tg.Players[target].ID
	ck := strconv.FormatInt(h.tg.Players[challenger].ID, 10)

	rollID := hfStartRoll(t, h, plan.ID, challenger)
	h.forceRoll(rollID, "make", 0)

	cdPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/challenge-duel"
	code, body := h.post(challenger, cdPath, map[string]any{"target_player_id": targetID})
	require.Equalf(t, http.StatusCreated, code, "challenge-duel after rolling make: %v", body)

	// The target declines (they have not taken the accept_duels mar).
	rcPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/respond-challenge"
	code, body = h.post(target, rcPath, map[string]any{"accept": false})
	require.Equalf(t, http.StatusOK, code, "decline challenge: %v", body)
	assert.Equal(t, false, body["accepted"], "challenge declined")

	st := hfChallengeState(t, h, plan.ID)
	assert.Nil(t, st.PendingChallenge, "challenge is cleared on decline")
	assert.Nil(t, st.PendingDuelPlanID, "no nested duel is spawned on decline")
	assert.Equal(t, "make", st.Outcomes[ck], "the challenger's make stays spent after a decline")
	assert.Equal(t, "challenge_duel", st.GuestMakes[ck], "spent on challenge_duel")
}

// TestHostFestivity_ChallengeDuel_RequiresMake proves a guest who has NOT rolled
// a make cannot open a challenge: with no roll the route 403s, and a resolved
// mar roll 403s too — the resolved roll outcome is the authority.
func TestHostFestivity_ChallengeDuel_RequiresMake(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	challenger := (hostIdx + 1) % len(h.tg.Players)
	target := (hostIdx + 2) % len(h.tg.Players)
	targetID := h.tg.Players[target].ID
	cdPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/challenge-duel"

	// No roll yet → forbidden.
	code, body := h.post(challenger, cdPath, map[string]any{"target_player_id": targetID})
	require.Equalf(t, http.StatusForbidden, code, "challenge with no roll: %v", body)

	// A resolved mar → still forbidden.
	rollID := hfStartRoll(t, h, plan.ID, challenger)
	h.forceRoll(rollID, "mar", 0)
	code, body = h.post(challenger, cdPath, map[string]any{"target_player_id": targetID})
	require.Equalf(t, http.StatusForbidden, code, "challenge after a mar: %v", body)
}
