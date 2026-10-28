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
	"strings"
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

// hfCenteredDraft returns the centered draft peer named name, or nil. Peers
// introduced at the party are drafts until claimed (D7), so this is how a test
// sees an introduction that hasn't become an asset yet.
func hfCenteredDraft(t *testing.T, h *planLifecycle, planID int64, name string) *gamepkg.DraftPeer {
	t.Helper()
	for _, d := range hfChallengeState(t, h, planID).CenteredDrafts {
		if d.Name == name {
			return &d
		}
	}
	return nil
}

// hfPeerInRetinue reports whether players[idx] owns a live peer called name.
func hfPeerInRetinue(t *testing.T, h *planLifecycle, idx int, name string) bool {
	t.Helper()
	assets, err := h.q.ListAssetsByOwner(context.Background(), h.tg.Players[idx].ID)
	require.NoError(t, err)
	for _, a := range assets {
		if a.GameID == h.tg.Game.ID && a.AssetType == model.AssetPeer &&
			a.Name == name && !a.IsDestroyed {
			return true
		}
	}
	return false
}

// hfPeerAssetExists reports whether ANY player owns a peer called name — the
// check that a draft really has no `assets` row behind it.
func hfPeerAssetExists(t *testing.T, h *planLifecycle, name string) bool {
	t.Helper()
	for idx := range h.tg.Players {
		if hfPeerInRetinue(t, h, idx, name) {
			return true
		}
	}
	return false
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

// TestHostFestivity_BlankInputsRejected proves the rumor/peer-name options
// require real input — no silent "(untold rumor)" / "New peer" fallback.
func TestHostFestivity_BlankInputsRejected(t *testing.T) {
	h := newPlanLifecycle(t, 2)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)

	choicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-choice"
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"

	// The host's earned (hosting) make can't introduce a nameless peer. Done
	// first, before any guest roll locks the table.
	code, body := h.post(hostIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "  ", "peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusBadRequest, code, "blank peer name rejected: %v", body)

	// introduce_peer requires exactly one marginalia too (host route is enough
	// to prove it; guest-choice shares the same applier/validator).
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "New Peer", "peer_marginalia": []string{},
	})
	require.Equalf(t, http.StatusBadRequest, code, "missing marginalia rejected: %v", body)
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "New Peer",
		"peer_marginalia": []string{"first", "second"},
	})
	require.Equalf(t, http.StatusBadRequest, code, "two marginalia rejected: %v", body)

	// Whitespace-only rumor text is rejected too.
	rollID := hfStartRoll(t, h, plan.ID, g1)
	h.forceRoll(rollID, "make", 0)
	code, body = h.post(g1, choicePath, map[string]any{"choice": "spread_rumor", "rumor_text": "   "})
	require.Equalf(t, http.StatusBadRequest, code, "blank rumor text rejected: %v", body)
}

// TestHostFestivity_InsistBreakSelf_HostResolves proves the book's reading of
// "insist that the host choose one option from the mar list" (p.61): the guest's
// insist names no option — it queues an unchosen mar — and the host then CHOOSES
// break_self and picks which marginalia to tear. Nothing tears until they do.
func TestHostFestivity_InsistBreakSelf_HostResolves(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)

	// Two marginalia on the HOST's main character so tearing one won't destroy it.
	hostMCID, hostMargIDs := hfSeedMCMarginalia(t, h, hostIdx, 2)

	insistPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/insist-host-mar"
	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/resolve-host-mar"
	endPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/end-event"

	// g1 rolls a make (earning an IOU) and insists — naming no option.
	hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	code, body := h.post(g1, insistPath, nil)
	require.Equalf(t, http.StatusOK, code, "g1 insists: %v", body)

	// The mar is unchosen — the host has yet to pick, and nothing was torn.
	state := hfChallengeState(t, h, plan.ID)
	assert.Equal(t, []int64{h.tg.Players[g1].ID}, state.UnchosenHostMars, "insist queues an unchosen mar")
	assert.Empty(t, state.PendingHostMars, "no option is recorded: the host hasn't chosen")
	assert.Empty(t, state.HostMarInsists, "nothing counts as inflicted until the host chooses")
	for _, id := range hostMargIDs {
		m, err := h.q.GetMarginaliaByID(ctx, id)
		require.NoError(t, err)
		assert.False(t, m.IsTorn, "no marginalia is torn before the host resolves")
	}

	// The event can't be wound down while a mar is owed.
	code, body = h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusConflict, code, "end with a mar unchosen: %v", body)

	// The insisting guest doesn't get to choose — only the host does.
	g1ID := h.tg.Players[g1].ID
	code, body = h.post(g1, breakPath, map[string]any{
		"insister_id": g1ID, "mar_option": "break_self", "marginalia_id": hostMargIDs[0],
	})
	require.Equalf(t, http.StatusForbidden, code, "insisting guest chooses the mar: %v", body)

	// The host chooses break_self and which marginalia to tear.
	code, body = h.post(hostIdx, breakPath, map[string]any{
		"insister_id": g1ID, "mar_option": "break_self", "marginalia_id": hostMargIDs[1],
	})
	require.Equalf(t, http.StatusOK, code, "host chooses break_self: %v", body)

	state = hfChallengeState(t, h, plan.ID)
	assert.Empty(t, state.UnchosenHostMars, "choosing clears the unchosen mar")
	assert.Equal(t, []string{"break_self"}, state.HostMarInsists, "the chosen mar is recorded as inflicted")
	chosen, err := h.q.GetMarginaliaByID(ctx, hostMargIDs[1])
	require.NoError(t, err)
	assert.True(t, chosen.IsTorn, "the host's chosen marginalia is torn")
	other, err := h.q.GetMarginaliaByID(ctx, hostMargIDs[0])
	require.NoError(t, err)
	assert.False(t, other.IsTorn, "the marginalia the host did not pick stays intact")

	intact, err := h.q.GetAssetByID(ctx, hostMCID)
	require.NoError(t, err)
	assert.False(t, intact.IsDestroyed, "tearing one of two marginalia does not destroy the MC")
}

// TestHostFestivity_InsistBreakSelf_BlankMainCharacterIsDestroyed proves an
// insisted break_self still lands when the host's main character carries no
// marginalia at all: the soft-lock guard must NOT settle it as a no-op, because
// the break has somewhere to go — it destroys the MC outright (D3). Before the
// backstop, a host with a blank MC discharged every insisted break for free.
func TestHostFestivity_InsistBreakSelf_BlankMainCharacterIsDestroyed(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)

	// The host's seeded main character is blank — no hfSeedMCMarginalia call.
	hostMCID := h.mainCharacterID(hostIdx)
	blank, err := h.q.CountMarginalia(ctx, hostMCID)
	require.NoError(t, err)
	require.EqualValues(t, 0, blank, "this scenario needs a blank host main character")

	insistPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/insist-host-mar"
	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/resolve-host-mar"

	hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	code, body := h.post(g1, insistPath, nil)
	require.Equalf(t, http.StatusOK, code, "g1 insists: %v", body)

	// The host chooses break_self with no marginalia named — there is none to name.
	code, body = h.post(hostIdx, breakPath, map[string]any{
		"insister_id": h.tg.Players[g1].ID, "mar_option": "break_self",
	})
	require.Equalf(t, http.StatusOK, code, "host resolves break on a blank MC: %v", body)

	state := hfChallengeState(t, h, plan.ID)
	assert.Empty(t, state.UnchosenHostMars, "choosing clears the unchosen mar")

	mc, err := h.q.GetAssetByID(ctx, hostMCID)
	require.NoError(t, err)
	assert.True(t, mc.IsDestroyed, "a blank main character is destroyed by the break")

	logs := hfAllLogBodies(t, h)
	assert.Contains(t, logs, "destroyed themselves", "the break_self log uses breakVerb(true)")
	assert.NotContains(t, logs, "The host had nothing left to tear",
		"the no-op escape must NOT fire — the break had a target")
}

// hfAllLogBodies concatenates every action-log body in the harness's game.
func hfAllLogBodies(t *testing.T, h *planLifecycle) string {
	t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	var sb strings.Builder
	for _, p := range posts {
		sb.WriteString(p.Body)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TestHostFestivity_InsistDisagreement_HostResolves proves the host can choose
// disagreement in answer to an insistence, and picks which of their own peers
// falls out.
func TestHostFestivity_InsistDisagreement_HostResolves(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)

	// The host owns a peer (with a marginalia) they can fall out with.
	peerID := h.seedPeer(hostIdx, "Loyal Aide")
	_, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: peerID, Position: 1, Text: "devoted"})
	require.NoError(t, err)

	insistPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/insist-host-mar"
	resolvePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/resolve-host-mar"

	// g1 rolls a make (earning an IOU) and insists.
	hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "psst"})
	code, body := h.post(g1, insistPath, nil)
	require.Equalf(t, http.StatusOK, code, "g1 insists: %v", body)

	// Nothing centered until the host chooses.
	state := hfChallengeState(t, h, plan.ID)
	assert.NotContains(t, state.CenteredAssetIDs, peerID, "no peer is centered before the host chooses")

	// A non-host can't choose for the host.
	g1ID := h.tg.Players[g1].ID
	code, body = h.post(g1, resolvePath, map[string]any{
		"insister_id": g1ID, "mar_option": "disagreement", "asset_id": peerID,
	})
	require.Equalf(t, http.StatusForbidden, code, "non-host chooses disagreement: %v", body)

	// The host chooses disagreement and which of their peers falls out.
	code, body = h.post(hostIdx, resolvePath, map[string]any{
		"insister_id": g1ID, "mar_option": "disagreement", "asset_id": peerID,
	})
	require.Equalf(t, http.StatusOK, code, "host chooses disagreement: %v", body)

	state = hfChallengeState(t, h, plan.ID)
	assert.Empty(t, state.UnchosenHostMars, "choosing clears the unchosen mar")
	assert.Contains(t, state.CenteredAssetIDs, peerID, "the host's chosen peer is now in the center")
	assert.Contains(t, state.DisagreementAssetIDs, peerID, "it's tracked for the rejoin-broken settlement")
}

// TestHostFestivity_Disagreement_AbandonedPeerRejoinsBroken proves the tail of a
// disagreement: a peer no one took from the center rejoins its owner broken when
// the event ends (one marginalia torn).
func TestHostFestivity_Disagreement_AbandonedPeerRejoinsBroken(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)

	// g1 owns a peer with two marginalia (so breaking one won't destroy it).
	peerID := h.seedPeer(g1, "Old Friend")
	var margIDs []int64
	for i := range 2 {
		m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: peerID, Position: int16(i + 1), Text: "note"})
		require.NoError(t, err)
		margIDs = append(margIDs, m.ID)
	}

	// g1 rolls a mar and falls out with their own peer — it goes to the center.
	hfGuestRoll(t, h, plan.ID, g1, "mar", "disagreement", map[string]any{"asset_id": peerID})
	state := hfChallengeState(t, h, plan.ID)
	require.Contains(t, state.DisagreementAssetIDs, peerID)

	// Host takes every earned make (hosting + g1's mar = 2), then ends the event.
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"
	endPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/end-event"
	for i := range 2 {
		code, body := h.post(hostIdx, hostChoicePath, map[string]any{
			"choice": "introduce_peer", "peer_name": "Guest", "peer_marginalia": []string{"a trait"},
		})
		require.Equalf(t, http.StatusOK, code, "host make %d: %v", i, body)
	}
	code, body := h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusOK, code, "host ends event: %v", body)

	// The un-taken peer rejoined g1 broken: exactly one marginalia torn, peer alive.
	torn := 0
	for _, id := range margIDs {
		m, err := h.q.GetMarginaliaByID(ctx, id)
		require.NoError(t, err)
		if m.IsTorn {
			torn++
		}
	}
	assert.Equal(t, 1, torn, "exactly one marginalia is torn on the abandoned peer")
	peer, err := h.q.GetAssetByID(ctx, peerID)
	require.NoError(t, err)
	assert.False(t, peer.IsDestroyed, "breaking one of two marginalia doesn't destroy the peer")

	state = hfChallengeState(t, h, plan.ID)
	assert.Empty(t, state.DisagreementAssetIDs, "the watch-list is cleared once the event ends")
}

// TestHostFestivity_HostFreeMake_BenefitsHost proves an extra make benefits the
// HOST, not the triggering guest: the peer the host introduces is drafted in
// their name and, unclaimed, ends up in the host's own retinue.
func TestHostFestivity_HostFreeMake_BenefitsHost(t *testing.T) {
	h := newPlanLifecycle(t, 3)

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
		"choice":          "introduce_peer",
		"peer_name":       "Gilded Guest",
		"peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusOK, code, "host-choice introduce_peer: %v", body)

	// The peer is drafted in the HOST's name, not g1's — and is nobody's asset
	// yet: they're working the room, free for any guest to claim (D7).
	draft := hfCenteredDraft(t, h, plan.ID, "Gilded Guest")
	require.NotNil(t, draft, "the host's introduction is recorded as a centered draft")
	assert.Equal(t, hostID, draft.CreatorID, "the host introduced them, not the triggering guest")
	assert.False(t, hfPeerAssetExists(t, h, "Gilded Guest"),
		"a centered draft has no assets row in anyone's retinue")

	// Nobody claims them, so they leave with the host when the evening ends.
	for range 2 {
		code, body = h.post(hostIdx, hostChoicePath, map[string]any{
			"choice": "spread_rumor", "rumor_text": "a whisper",
		})
		require.Equalf(t, http.StatusOK, code, "host takes a remaining make: %v", body)
	}
	code, body = h.post(hostIdx, "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/end-event", nil)
	require.Equalf(t, http.StatusOK, code, "host ends the event: %v", body)

	assert.True(t, hfPeerInRetinue(t, h, hostIdx, "Gilded Guest"),
		"the host's extra make adds the peer to the host's own retinue")
}

// TestHostFestivity_IntroducedPeer_UnclaimedJoinsIntroducer proves D7's default
// ending: a peer introduced at the party belongs to nobody while the event runs
// — no `assets` row exists at all — and joins the *introducer's* retinue when
// nobody has claimed them by the time the host winds the evening down.
func TestHostFestivity_IntroducedPeer_UnclaimedJoinsIntroducer(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)

	// g1 rolls a make and introduces a peer; g2 opts out.
	hfGuestRoll(t, h, plan.ID, g1, "make", "introduce_peer", map[string]any{
		"peer_name": "Ysolde of the Marches", "peer_marginalia": []string{"keeps a hawk"},
	})
	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	code, body := h.post(g2, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "g2 opt out: %v", body)

	draft := hfCenteredDraft(t, h, plan.ID, "Ysolde of the Marches")
	require.NotNil(t, draft, "the introduction is recorded as a centered draft")
	assert.Equal(t, h.tg.Players[g1].ID, draft.CreatorID, "g1 introduced them")
	assert.Equal(t, "keeps a hawk", draft.Marginalia,
		"the marginalia written at introduce time rides on the draft")
	assert.False(t, hfPeerAssetExists(t, h, "Ysolde of the Marches"),
		"no assets row exists while the draft sits in the centre")

	// Wind the event down: the host takes both earned makes (hosting + g2's
	// opt-out) and g1 spends their IOU, then the host ends it.
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"
	for range 2 {
		code, body = h.post(hostIdx, hostChoicePath, map[string]any{
			"choice": "spread_rumor", "rumor_text": "a whisper",
		})
		require.Equalf(t, http.StatusOK, code, "host make: %v", body)
	}
	hfInsistAndChoose(t, h, plan.ID, hostIdx, g1, gamepkg.FestivityMarAcceptDuels)
	code, body = h.post(hostIdx, "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/end-event", nil)
	require.Equalf(t, http.StatusOK, code, "host ends the event: %v", body)

	assert.True(t, hfPeerInRetinue(t, h, g1, "Ysolde of the Marches"),
		"an unclaimed peer leaves with the player who introduced them")
	assert.Empty(t, hfChallengeState(t, h, plan.ID).CenteredDrafts,
		"the centre is emptied once the event ends")
	assert.Contains(t, hfAllLogBodies(t, h), "Nobody else claimed",
		"the settlement is narrated in the action log")
}

// TestHostFestivity_IntroducedPeer_ClaimedByGuest proves the other half of D7:
// take_center_peer on a draft is a FIRST CLAIM, not a transfer — the peer
// materializes straight into the claimant's retinue carrying the marginalia
// their introducer wrote, and leaves the centre.
func TestHostFestivity_IntroducedPeer_ClaimedByGuest(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)

	hfGuestRoll(t, h, plan.ID, g1, "make", "introduce_peer", map[string]any{
		"peer_name": "Brannoc the Quiet", "peer_marginalia": []string{"never blinks"},
	})
	draft := hfCenteredDraft(t, h, plan.ID, "Brannoc the Quiet")
	require.NotNil(t, draft)

	choicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-choice"

	// A draft that isn't in the centre can't be claimed.
	hfGuestRollMar(t, h, plan.ID, g2)
	code, body := h.post(g2, choicePath, map[string]any{
		"choice": "take_center_peer", "draft_id": "not-a-real-draft",
	})
	require.Equalf(t, http.StatusBadRequest, code, "unknown draft_id must be refused: %v", body)

	// g2's roll is still open, so they can still choose. Force it to a make and
	// claim the drafted peer.
	rollID := hfChallengeState(t, h, plan.ID).GuestRollIDs[strconv.FormatInt(h.tg.Players[g2].ID, 10)]
	h.forceRoll(rollID, "make", 0)
	code, body = h.post(g2, choicePath, map[string]any{
		"choice": "take_center_peer", "draft_id": draft.ID,
	})
	require.Equalf(t, http.StatusOK, code, "g2 claims the drafted peer: %v", body)

	assert.True(t, hfPeerInRetinue(t, h, g2, "Brannoc the Quiet"),
		"the claimed peer materializes into the claimant's retinue, not the introducer's")
	assert.False(t, hfPeerInRetinue(t, h, g1, "Brannoc the Quiet"),
		"the introducer keeps nothing — arrival is creation, not a transfer")
	assert.Empty(t, hfChallengeState(t, h, plan.ID).CenteredDrafts,
		"a claimed draft leaves the centre")

	// The peer carries the marginalia their introducer wrote — materialization
	// goes through createAssetWithFirstMarginalia, so no blank peer can arrive.
	ctx := context.Background()
	claimed, err := h.q.ListAssetsByOwner(ctx, h.tg.Players[g2].ID)
	require.NoError(t, err)
	var peerID int64
	for _, a := range claimed {
		if a.Name == "Brannoc the Quiet" {
			peerID = a.ID
		}
	}
	require.NotZero(t, peerID)
	marg, err := h.q.ListMarginaliaByAsset(ctx, peerID)
	require.NoError(t, err)
	require.Len(t, marg, 1, "the claimed peer arrives with exactly the introducer's note")
	assert.Equal(t, "never blinks", marg[0].Text)
}

// TestHostFestivity_TakeCenterPeer_DisagreementPeerStillTransfers proves the
// other kind of centered peer is untouched by D7: a real peer shoved to the
// centre by a disagreement still changes hands through the canonical transfer,
// carrying its secrets to the new owner.
func TestHostFestivity_TakeCenterPeer_DisagreementPeerStillTransfers(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)

	peerID, _ := h.seedPeerWithMarginalia(g1, "Estranged Cousin")
	secretID := h.seedSecret(peerID, g1, "owes money to the crown")

	// g1 mars and falls out with their own peer, shoving them to the centre.
	hfGuestRoll(t, h, plan.ID, g1, "mar", "disagreement", map[string]any{"asset_id": peerID})
	require.Contains(t, hfChallengeState(t, h, plan.ID).CenteredAssetIDs, peerID)

	// g2 rolls a make and takes them — an ordinary transfer, by asset_id.
	hfGuestRoll(t, h, plan.ID, g2, "make", "take_center_peer", map[string]any{"asset_id": peerID})

	moved, err := h.q.GetAssetByID(ctx, peerID)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[g2].ID, moved.OwnerID, "the disagreement peer changes hands")
	h.assertSecretVisible("taking a disagreement peer grants its secrets", peerID, secretID, g2)

	state := hfChallengeState(t, h, plan.ID)
	assert.NotContains(t, state.CenteredAssetIDs, peerID, "a taken peer leaves the centre")
	assert.NotContains(t, state.DisagreementAssetIDs, peerID,
		"and drops off the rejoin-broken watch-list")
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
		"choice":          "introduce_peer",
		"peer_name":       "Host's Own Guest",
		"peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusOK, code, "host self make: %v", body)

	// The new peer is drafted in the host's name (they belong to nobody until
	// claimed, or until the event ends — see the D7 tests below).
	draft := hfCenteredDraft(t, h, plan.ID, "Host's Own Guest")
	require.NotNil(t, draft, "the host's earned make records a centered draft")
	assert.Equal(t, hostID, draft.CreatorID, "the earned make is the host's own")
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
	resolvePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/resolve-host-mar"

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
			"choice": "introduce_peer", "peer_name": "Guest", "peer_marginalia": []string{"a trait"},
		})
		require.Equalf(t, http.StatusOK, code, "host make %d: %v", i, body)
	}
	// A third make is refused — only two were earned.
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "Extra", "peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusConflict, code, "over-taking makes: %v", body)

	// Still not endable: g1's mar is unspent.
	code, body = h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusConflict, code, "end before g1's mar inflicted: %v", body)

	// g1 insists. That alone isn't enough: the mar is the host's to choose.
	code, body = h.post(g1, insistPath, nil)
	require.Equalf(t, http.StatusOK, code, "g1 insist mar: %v", body)
	code, body = h.post(hostIdx, endPath, nil)
	require.Equalf(t, http.StatusConflict, code, "end with the insisted mar unchosen: %v", body)
	code, body = h.post(hostIdx, resolvePath, map[string]any{
		"insister_id": h.tg.Players[g1].ID, "mar_option": "accept_duels",
	})
	require.Equalf(t, http.StatusOK, code, "host chooses a mar: %v", body)

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

// TestHostFestivity_PerformStepsWinnerDrivesHostSteps proves a Make Demands
// perform_steps win transfers the HOST's resolution steps — taking an earned
// make (host-choice) and settling an insisted mar (resolve-host-mar) — to the
// winner, locking the host out. The make/mar still lands on the HOST (the winner
// drives, the host reaps), and the guest routes (rolling, insisting, opting out)
// stay untouched third-party roles.
func TestHostFestivity_PerformStepsWinnerDrivesHostSteps(t *testing.T) {
	h := newPlanLifecycle(t, 4)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	demanderIdx := (hostIdx + 1) % 4
	insisterIdx := (hostIdx + 2) % 4
	spareIdx := (hostIdx + 3) % 4
	hostID := h.tg.Players[hostIdx].ID

	// Two marginalia on the host's MC so an insisted break_self has something to
	// tear without destroying the character.
	_, hostMargIDs := hfSeedMCMarginalia(t, h, hostIdx, 2)

	// A guest (third party) rolls a make and insists — an untouched guest role
	// that queues a mar for the host to choose.
	hfGuestRoll(t, h, plan.ID, insisterIdx, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	insistPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/insist-host-mar"
	code, body := h.post(insisterIdx, insistPath, nil)
	require.Equalf(t, http.StatusOK, code, "guest insists on the host: %v", body)
	insisterID := h.tg.Players[insisterIdx].ID

	// A resolved, made demand hands perform_steps to the demander.
	h.seedMadeDemand(demanderIdx, plan.ID, gamepkg.DemandOptionWinners{
		gamepkg.DemandOptionPerformSteps: h.tg.Players[demanderIdx].ID,
	})

	// host-choice (make): the host is locked out; the winner drives it; the peer
	// still lands in the HOST's retinue.
	hostChoicePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/host-choice"
	code, body = h.post(hostIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "Winner's Gift", "peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusForbidden, code, "host locked out of host-choice: %v", body)
	code, body = h.post(demanderIdx, hostChoicePath, map[string]any{
		"choice": "introduce_peer", "peer_name": "Winner's Gift", "peer_marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusOK, code, "winner drives host-choice: %v", body)

	// The peer is drafted in the HOST's name, not the winner's — perform_steps
	// hands over who acts, never who gains (keep_assets is the option that
	// redirects gains, and it is not in play here).
	gift := hfCenteredDraft(t, h, plan.ID, "Winner's Gift")
	require.NotNil(t, gift, "the winner-driven make records a centered draft")
	assert.Equal(t, hostID, gift.CreatorID, "the host make is credited to the HOST, not the winner")

	// resolve-host-mar (mar): the host is locked out; the winner makes the host's
	// choice and picks which of the HOST's marginalia to tear.
	resolvePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/resolve-host-mar"
	code, body = h.post(hostIdx, resolvePath, map[string]any{
		"insister_id": insisterID, "mar_option": "break_self", "marginalia_id": hostMargIDs[0],
	})
	require.Equalf(t, http.StatusForbidden, code, "host locked out of resolve-host-mar: %v", body)
	code, body = h.post(demanderIdx, resolvePath, map[string]any{
		"insister_id": insisterID, "mar_option": "break_self", "marginalia_id": hostMargIDs[0],
	})
	require.Equalf(t, http.StatusOK, code, "winner drives resolve-host-mar: %v", body)

	torn, err := h.q.GetMarginaliaByID(ctx, hostMargIDs[0])
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "the winner's break lands on the HOST's own marginalia")

	// A guest role stays untouched: a spare guest can still opt out as themselves.
	rollPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/guest-roll"
	code, body = h.post(spareIdx, rollPath, map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "a guest still opts out as themselves: %v", body)
}

// ── Q13: the host chooses the insisted mar ───────────────────────────────────

func hfPath(planID int64, route string) string {
	return "/api/plans/" + strconv.FormatInt(planID, 10) + "/" + route
}

// hfInsistAndChoose has players[guestIdx] insist, then the host choose marOption
// (one that needs no further input, e.g. accept_duels) in answer.
func hfInsistAndChoose(t *testing.T, h *planLifecycle, planID int64, hostIdx, guestIdx int, marOption string) {
	t.Helper()
	code, body := h.post(guestIdx, hfPath(planID, "insist-host-mar"), nil)
	require.Equalf(t, http.StatusOK, code, "guest insists: %v", body)
	code, body = h.post(hostIdx, hfPath(planID, "resolve-host-mar"), map[string]any{
		"insister_id": h.tg.Players[guestIdx].ID, "mar_option": marOption,
	})
	require.Equalf(t, http.StatusOK, code, "host chooses %s: %v", marOption, body)
}

// TestHostFestivity_Insist_CarriesNoOption proves the guest's insist names no mar:
// even a request that still sends the old `mar_option` queues an UNCHOSEN mar and
// applies nothing. (Before the Q13 ruling the insister picked, and rumor /
// accept_duels landed on the host immediately.)
func TestHostFestivity_Insist_CarriesNoOption(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	rumorsBefore, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)

	code, body := h.post(g1, hfPath(plan.ID, "insist-host-mar"), map[string]any{
		"mar_option": "rumor_about_you", "rumor_text": "the guest wrote this",
	})
	require.Equalf(t, http.StatusOK, code, "insist: %v", body)

	state := hfChallengeState(t, h, plan.ID)
	assert.Equal(t, []int64{h.tg.Players[g1].ID}, state.UnchosenHostMars, "an unchosen mar is queued")
	assert.Empty(t, state.HostMarInsists, "no mar has been inflicted yet")
	assert.Empty(t, state.GuestIOUs, "the IOU is spent by insisting")
	rumorsAfter, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Len(t, rumorsAfter, len(rumorsBefore), "the insister cannot make a rumor land on the host")

	// An IOU is spent once: a second insist is refused.
	code, _ = h.post(g1, hfPath(plan.ID, "insist-host-mar"), nil)
	assert.Equal(t, http.StatusForbidden, code, "no second insist on one IOU")

	logs := hfAllLogBodies(t, h)
	assert.Contains(t, logs, "insisted the host take a mar", "the insisting is logged")
	assert.NotContains(t, logs, "A rumor spread about", "the insister's pick must not have applied")
}

// TestHostFestivity_HostChoosesInsistedMar_AppliesToHost proves the two options
// that need no asset pick take effect only when the HOST chooses them, land on
// the host, and that the host's choice is logged apart from the insisting.
func TestHostFestivity_HostChoosesInsistedMar_AppliesToHost(t *testing.T) {
	t.Run("rumor_about_you needs the host's rumor text", func(t *testing.T) {
		h := newPlanLifecycle(t, 3)
		ctx := context.Background()
		plan, hostIdx := hfPrepareToSocializing(t, h)
		g1 := (hostIdx + 1) % len(h.tg.Players)
		hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
		code, body := h.post(g1, hfPath(plan.ID, "insist-host-mar"), nil)
		require.Equalf(t, http.StatusOK, code, "insist: %v", body)
		g1ID := h.tg.Players[g1].ID

		// A blank rumor is rejected and the insistence stays queued.
		code, body = h.post(hostIdx, hfPath(plan.ID, "resolve-host-mar"), map[string]any{
			"insister_id": g1ID, "mar_option": "rumor_about_you",
		})
		require.Equalf(t, http.StatusBadRequest, code, "blank rumor: %v", body)
		assert.Equal(t, []int64{g1ID}, hfChallengeState(t, h, plan.ID).UnchosenHostMars)

		code, body = h.post(hostIdx, hfPath(plan.ID, "resolve-host-mar"), map[string]any{
			"insister_id": g1ID, "mar_option": "rumor_about_you", "rumor_text": "the host was seen at the docks",
		})
		require.Equalf(t, http.StatusOK, code, "host chooses a rumor: %v", body)

		state := hfChallengeState(t, h, plan.ID)
		assert.Empty(t, state.UnchosenHostMars)
		assert.Equal(t, []string{"rumor_about_you"}, state.HostMarInsists)
		rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
		require.NoError(t, err)
		found := false
		for _, r := range rumors {
			if r.Text == "the host was seen at the docks" {
				found = true
				require.NotNil(t, r.TargetAssetID, "the rumor targets the host's main character")
				assert.Equal(t, h.mainCharacterID(hostIdx), *r.TargetAssetID)
			}
		}
		assert.True(t, found, "the host's rumor was created")

		logs := hfAllLogBodies(t, h)
		assert.Contains(t, logs, "chose to have a rumor spread about them in answer to",
			"the host's choice is logged, naming whose insistence it answers")
	})

	t.Run("accept_duels binds the host", func(t *testing.T) {
		h := newPlanLifecycle(t, 3)
		plan, hostIdx := hfPrepareToSocializing(t, h)
		g1 := (hostIdx + 1) % len(h.tg.Players)
		hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
		hfInsistAndChoose(t, h, plan.ID, hostIdx, g1, "accept_duels")

		state := hfChallengeState(t, h, plan.ID)
		assert.Equal(t, []int64{h.tg.Players[hostIdx].ID}, state.AcceptDuels,
			"the HOST must now accept duels, not the insister")
		assert.Contains(t, hfAllLogBodies(t, h), "to accept any duel challenge for the rest of the event")
	})
}

// TestHostFestivity_ResolveHostMar_Guards pins the route's refusals and the
// legacy path kept for games already in flight.
func TestHostFestivity_ResolveHostMar_Guards(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g1ID := h.tg.Players[g1].ID
	resolve := hfPath(plan.ID, "resolve-host-mar")

	// Nothing insisted yet → nothing to choose.
	code, body := h.post(hostIdx, resolve, map[string]any{"insister_id": g1ID, "mar_option": "accept_duels"})
	require.Equalf(t, http.StatusConflict, code, "choosing with no insistence: %v", body)

	hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	code, _ = h.post(g1, hfPath(plan.ID, "insist-host-mar"), nil)
	require.Equal(t, http.StatusOK, code)

	code, body = h.post(hostIdx, resolve, map[string]any{"insister_id": g1ID, "mar_option": "nonsense"})
	require.Equalf(t, http.StatusBadRequest, code, "invalid option: %v", body)
	// An insistence is answered by the guest who made it, not by a bystander's id.
	other := (hostIdx + 2) % len(h.tg.Players)
	code, body = h.post(hostIdx, resolve, map[string]any{
		"insister_id": h.tg.Players[other].ID, "mar_option": "accept_duels",
	})
	require.Equalf(t, http.StatusConflict, code, "answering an insistence nobody made: %v", body)

	// LEGACY: a game in flight holds a mar the old insister picked, as a bare option
	// in pending_host_mars. It still settles with no insister_id, needs a
	// host-resolved option, and is not double-counted as inflicted.
	_, margIDs := hfSeedMCMarginalia(t, h, hostIdx, 2)
	reloaded, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(reloaded.ResolutionData)
	st := rd.EnsureFestivity()
	st.UnchosenHostMars = nil
	st.PendingHostMars = []string{gamepkg.FestivityMarBreakSelf}
	require.NoError(t, saveResolutionData(ctx, h.q, plan.ID, rd))

	code, body = h.post(hostIdx, resolve, map[string]any{"mar_option": "accept_duels"})
	require.Equalf(t, http.StatusBadRequest, code, "legacy path only settles asset-pick mars: %v", body)
	code, body = h.post(hostIdx, resolve, map[string]any{"mar_option": "break_self", "marginalia_id": margIDs[0]})
	require.Equalf(t, http.StatusOK, code, "legacy settle: %v", body)
	state := hfChallengeState(t, h, plan.ID)
	assert.Empty(t, state.PendingHostMars)
	assert.Empty(t, state.HostMarInsists, "a legacy mar was already recorded when it was insisted")
	torn, err := h.q.GetMarginaliaByID(ctx, margIDs[0])
	require.NoError(t, err)
	assert.True(t, torn.IsTorn)
}

// TestHostFestivity_InsistedMarKeepsHostInWaitingBar proves the bar names the host
// from the moment a guest insists until they choose — ADDED to whoever else still
// owes something, never replacing them.
func TestHostFestivity_InsistedMarKeepsHostInWaitingBar(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, hostIdx := hfPrepareToSocializing(t, h)
	g1 := (hostIdx + 1) % len(h.tg.Players)
	g2 := (hostIdx + 2) % len(h.tg.Players)
	hostID, g1ID, g2ID := h.tg.Players[hostIdx].ID, h.tg.Players[g1].ID, h.tg.Players[g2].ID

	hfGuestRoll(t, h, plan.ID, g1, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	code, body := h.post(hostIdx, hfPath(plan.ID, "host-choice"), map[string]any{
		"choice": "spread_rumor", "rumor_text": "the host's own",
	})
	require.Equalf(t, http.StatusOK, code, "host takes the hosting make: %v", body)

	// g2 hasn't acted and g1 still holds an IOU; the host owes nothing.
	h.assertWaitees("before the insist", model.RowStateAwaitFestivityGuestTurn, g1ID, g2ID)

	code, body = h.post(g1, hfPath(plan.ID, "insist-host-mar"), nil)
	require.Equalf(t, http.StatusOK, code, "insist: %v", body)
	h.assertWaitees("insisted, host has not chosen", model.RowStateAwaitFestivityGuestTurn, g2ID, hostID)

	code, body = h.post(hostIdx, hfPath(plan.ID, "resolve-host-mar"), map[string]any{
		"insister_id": g1ID, "mar_option": "accept_duels",
	})
	require.Equalf(t, http.StatusOK, code, "host chooses: %v", body)
	h.assertWaitees("host has chosen", model.RowStateAwaitFestivityGuestTurn, g2ID)
}

// TestHostFestivity_InsistedMarNamesPerformStepsWinner: when a Make Demands
// perform_steps winner drives the host's steps, the bar names THEM for the
// insisted mar (the server refuses the host), not the locked-out host.
func TestHostFestivity_InsistedMarNamesPerformStepsWinner(t *testing.T) {
	h := newPlanLifecycle(t, 4)
	plan, hostIdx := hfPrepareToSocializing(t, h)
	winnerIdx := (hostIdx + 1) % 4
	g2 := (hostIdx + 2) % 4
	g3 := (hostIdx + 3) % 4
	hostID, winnerID := h.tg.Players[hostIdx].ID, h.tg.Players[winnerIdx].ID

	// Everyone else has acted, the hosting make is taken, so the only thing
	// outstanding is g2's insisted mar.
	hfGuestRoll(t, h, plan.ID, g2, "make", "spread_rumor", map[string]any{"rumor_text": "a whisper"})
	for _, idx := range []int{winnerIdx, g3} {
		code, body := h.post(idx, hfPath(plan.ID, "guest-roll"), map[string]any{"action": "opt_out"})
		require.Equalf(t, http.StatusOK, code, "opt out: %v", body)
	}
	h.seedMadeDemand(winnerIdx, plan.ID, gamepkg.DemandOptionWinners{
		gamepkg.DemandOptionPerformSteps: winnerID,
	})
	// The host has earned 3 makes (hosting + two opt-outs); the winner takes them.
	for range 3 {
		code, body := h.post(winnerIdx, hfPath(plan.ID, "host-choice"), map[string]any{
			"choice": "spread_rumor", "rumor_text": "winner-driven",
		})
		require.Equalf(t, http.StatusOK, code, "winner takes a make: %v", body)
	}
	code, body := h.post(g2, hfPath(plan.ID, "insist-host-mar"), nil)
	require.Equalf(t, http.StatusOK, code, "insist: %v", body)

	h.assertWaitees("winner owes the host's choice", model.RowStateAwaitFestivityGuestTurn, winnerID)
	assert.NotContains(t, h.rowState().ActingPlayerIDs, hostID, "the locked-out host is not the one to wait on")
}
