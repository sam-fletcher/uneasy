//go:build integration

// handler/plan_make_demands_liaise_integration_test.go — the owner's 2026-11-04
// Clandestinely Liaise rulings: whoever re-aims a liaison also names the new
// partner's meeting peer, and the Make Demands winner of the perform option
// presses "advance" in place of the preparer.

package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// liaiseUnderDemand prepares and resolves a liaison between players 0 and 1 in a
// four-player game, with player 2 holding the given option and player 3 free to
// become the new partner. It returns before any step is taken.
func liaiseUnderDemand(t *testing.T, option string, winnerIdx int) (*planLifecycle, clMeeting) {
	t.Helper()
	h := newPlanLifecycle(t, 4)
	// The made demand must exist before the plan resolves (the hold is seeded then).
	preparerPeer, preparerMarg := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, partnerMarg := h.seedPeerWithMarginalia(1, "partner's confidant")
	notes := "a meeting under the bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanClandestinelyLiaise, TargetPlayerID: &partnerID,
		PreparerPeerID: &preparerPeer, PartnerPeerID: &partnerPeer, PreparationNotes: &notes,
	})
	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	clSubmitReveal(t, h, *ld.DelayRevealID, 0, 2)
	clSubmitReveal(t, h, *ld.DelayRevealID, 1, 2)
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{option: h.tg.Players[winnerIdx].ID})
	refreshed, err := h.q.GetPlanByID(t.Context(), plan.ID)
	require.NoError(t, err)
	h.jumpToRow(*refreshed.RowNumber)
	require.Nil(t, h.resolve(plan.ID))
	return h, clMeeting{
		plan: refreshed, preparerPeerID: preparerPeer, preparerMargID: preparerMarg,
		partnerPeerID: partnerPeer, partnerMargID: partnerMarg,
	}
}

func anyPostContains(t *testing.T, h *planLifecycle, parts ...string) bool {
	t.Helper()
	posts, err := h.q.ListGamePosts(t.Context(), h.tg.Game.ID)
	require.NoError(t, err)
	for _, p := range posts {
		ok := true
		for _, part := range parts {
			ok = ok && strings.Contains(p.Body, part)
		}
		if ok {
			return true
		}
	}
	return false
}

// A re-aim names the new partner's meeting peer; the plan's stored partner and
// peer follow, every later step reads them, and the old partner is out.
func TestMakeDemandsRulings_LiaiseReaimNamesTheNewPartnersPeer(t *testing.T) {
	h, m := liaiseUnderDemand(t, game.DemandOptionKeepOrChangeTarget, 2)
	ctx := t.Context()
	newPartnerID := h.tg.Players[3].ID
	newPeer := h.seedPeer(3, "Newcomer's confidant")

	code, body := h.post(2, demandRetargetPath(m.plan.ID),
		map[string]any{"target_player_id": newPartnerID, "partner_peer_id": newPeer})
	require.Equalf(t, http.StatusOK, code, "re-aim with a peer: %v", body)

	plan, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	require.NotNil(t, plan.TargetPlayerID)
	assert.Equal(t, newPartnerID, *plan.TargetPlayerID)
	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	require.NotNil(t, ld.PartnerID)
	assert.Equal(t, newPartnerID, *ld.PartnerID, "the liaison's partner follows the re-aim")
	require.NotNil(t, ld.PartnerPeerID)
	assert.Equal(t, newPeer, *ld.PartnerPeerID, "and so does the meeting peer")
	require.NotNil(t, ld.PreparerPeerID)
	assert.Equal(t, m.preparerPeerID, *ld.PreparerPeerID, "the preparer's own peer is untouched")
	posts, err := h.q.ListGamePosts(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	var line string
	for _, p := range posts {
		if strings.Contains(p.Body, "re-aimed") {
			line = p.Body
		}
	}
	assert.Contains(t, line, h.tg.Players[3].DisplayName, "the log names the NEW partner")
	assert.NotContains(t, line, h.tg.Players[1].DisplayName, "and not the old one")
	assert.Contains(t, line, "Newcomer's confidant", "and the peer they bring")

	// The meeting runs between the preparer and the NEW partner.
	code, body = h.post(0, clPath(m.plan.ID, "advance-liaise"), nil)
	require.Equalf(t, http.StatusOK, code, "advance: %v", body)
	code, body = h.post(1, clPath(m.plan.ID, "keep-secret"), map[string]any{"asset_id": clMC(t, h, 1)})
	assert.Equalf(t, http.StatusForbidden, code, "the old partner is out of the meeting: %v", body)
	code, body = h.post(3, clPath(m.plan.ID, "keep-secret"), map[string]any{"asset_id": clMC(t, h, 3)})
	assert.Equalf(t, http.StatusOK, code, "the new partner is in: %v", body)
}

// The server validates the peer against the board as it stands now.
func TestMakeDemandsRulings_LiaiseReaimRefusesAnIneligiblePeer(t *testing.T) {
	h, m := liaiseUnderDemand(t, game.DemandOptionKeepOrChangeTarget, 2)
	ctx := t.Context()
	newPartnerID := h.tg.Players[3].ID
	reaim := func(peer *int64) (int, map[string]any) {
		return h.post(2, demandRetargetPath(m.plan.ID),
			map[string]any{"target_player_id": newPartnerID, "partner_peer_id": peer})
	}

	code, body := reaim(nil)
	assert.Equalf(t, http.StatusBadRequest, code, "no peer named: %v", body)

	oldPartnersPeer := m.partnerPeerID
	code, body = reaim(&oldPartnersPeer)
	assert.Equalf(t, http.StatusBadRequest, code, "the old partner's peer is not the new partner's: %v", body)

	trinket, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: newPartnerID, CreatorID: newPartnerID,
		AssetType: model.AssetHolding, Name: "Newcomer's manor",
	})
	require.NoError(t, err)
	notAPeer := trinket.ID
	code, body = reaim(&notAPeer)
	assert.Equalf(t, http.StatusBadRequest, code, "an asset that is not a peer: %v", body)

	gone := h.seedPeer(3, "Newcomer's lost friend")
	_, err = h.pool.Exec(ctx, `UPDATE assets SET is_destroyed = true WHERE id = $1`, gone)
	require.NoError(t, err)
	code, body = reaim(&gone)
	assert.Equalf(t, http.StatusBadRequest, code, "a destroyed peer: %v", body)

	plan, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[1].ID, *plan.TargetPlayerID, "a refused re-aim changes nothing")
	rd := loadResolutionData(plan.ResolutionData)
	assert.False(t, rd.DemandRetargetFinalized, "and does not release the hold")
}

// A pending liaison has not begun resolving, and the new peer is checked only once,
// at the re-aim: so a re-aim waits for the plan to be resolving. Keeping is unaffected.
func TestMakeDemandsRulings_LiaisePendingCanOnlyBeKept(t *testing.T) {
	h := newPlanLifecycle(t, 4)
	preparerPeer, _ := h.seedPeerWithMarginalia(0, "preparer's confidant")
	partnerPeer, _ := h.seedPeerWithMarginalia(1, "partner's confidant")
	newPeer := h.seedPeer(3, "Newcomer's confidant")
	notes := "a meeting under the bridge"
	partnerID := h.tg.Players[1].ID
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanClandestinelyLiaise, TargetPlayerID: &partnerID,
		PreparerPeerID: &preparerPeer, PartnerPeerID: &partnerPeer, PreparationNotes: &notes,
	})
	h.seedMadeDemand(2, plan.ID, game.DemandOptionWinners{game.DemandOptionKeepOrChangeTarget: h.tg.Players[2].ID})
	code, body := h.post(2, demandRetargetPath(plan.ID),
		map[string]any{"target_player_id": h.tg.Players[3].ID, "partner_peer_id": newPeer})
	assert.Equalf(t, http.StatusConflict, code, "a pending liaison cannot be re-aimed: %v", body)
	code, body = h.post(2, demandRetargetPath(plan.ID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusOK, code, "but it can be kept: %v", body)
}

// If the preparer's own meeting peer is gone, the refusal says so in terms of the
// plan, not "your own peer", and keeping still works.
func TestMakeDemandsRulings_LiaiseReaimExplainsAGonePreparerPeer(t *testing.T) {
	h, m := liaiseUnderDemand(t, game.DemandOptionKeepOrChangeTarget, 2)
	_, err := h.pool.Exec(t.Context(), `UPDATE assets SET is_destroyed = true WHERE id = $1`, m.preparerPeerID)
	require.NoError(t, err)
	newPeer := h.seedPeer(3, "Newcomer's confidant")
	code, body := h.post(2, demandRetargetPath(m.plan.ID),
		map[string]any{"target_player_id": h.tg.Players[3].ID, "partner_peer_id": newPeer})
	assert.Equalf(t, http.StatusBadRequest, code, "re-aim refused: %v", body)
	msg, _ := body["error"].(string)
	assert.Contains(t, msg, "the preparer's meeting peer")
	assert.Contains(t, msg, "only be kept")
	assert.NotContains(t, msg, "your own")
	code, body = h.post(2, demandRetargetPath(m.plan.ID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusOK, code, "keeping still works: %v", body)
}

// A preparer who won the option re-aims the same way: they name the peer too.
func TestMakeDemandsRulings_LiaisePreparerWinnerReaimsWithAPeer(t *testing.T) {
	h, m := liaiseUnderDemand(t, game.DemandOptionKeepOrChangeTarget, 0)
	newPeer := h.seedPeer(3, "Newcomer's confidant")
	code, body := h.post(0, demandRetargetPath(m.plan.ID),
		map[string]any{"target_player_id": h.tg.Players[3].ID, "partner_peer_id": newPeer})
	require.Equalf(t, http.StatusOK, code, "the preparer re-aims: %v", body)
	plan, err := h.q.GetPlanByID(t.Context(), m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(plan.ResolutionData)
	ld := rd.EnsureLiaise()
	assert.Equal(t, newPeer, *ld.PartnerPeerID)

	// Keeping needs no peer.
	h2, m2 := liaiseUnderDemand(t, game.DemandOptionKeepOrChangeTarget, 0)
	code, body = h2.post(0, demandRetargetPath(m2.plan.ID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusOK, code, "keep: %v", body)
}

// "Advance" is a step, so the Make Demands winner of the perform option presses
// it, the preparer is refused, the bar names the winner, and the log says so.
func TestMakeDemandsRulings_StandIn_LiaiseAdvanceIsTheWinners(t *testing.T) {
	h, m := liaiseUnderDemand(t, game.DemandOptionPerformSteps, 2)
	winnerID := h.tg.Players[2].ID
	h.assertWaitees("the Make Demands winner advances", model.RowStatePlanResolving, winnerID)

	code, body := h.post(0, clPath(m.plan.ID, "advance-liaise"), nil)
	assert.Equalf(t, http.StatusForbidden, code, "the preparer is locked out: %v", body)
	code, body = h.post(3, clPath(m.plan.ID, "advance-liaise"), nil)
	assert.Equalf(t, http.StatusForbidden, code, "a bystander is refused: %v", body)
	code, body = h.post(2, clPath(m.plan.ID, "advance-liaise"), nil)
	require.Equalf(t, http.StatusOK, code, "the Make Demands winner advances: %v", body)
	assert.True(t, anyPostContains(t, h, "the Make Demands winner, moved", "Secrets We Keep"))
}

// With no perform win the preparer advances, as before, and no stand-in line is logged.
func TestMakeDemandsRulings_LiaiseAdvanceStaysThePreparersWithoutAPerformWin(t *testing.T) {
	h, m := liaiseUnderDemand(t, game.DemandOptionKeepAssets, 2)
	h.assertWaitees("the preparer is waited on", model.RowStatePlanResolving, h.tg.Players[0].ID)
	code, body := h.post(2, clPath(m.plan.ID, "advance-liaise"), nil)
	assert.Equalf(t, http.StatusForbidden, code, "another option's winner has no say here: %v", body)
	code, body = h.post(0, clPath(m.plan.ID, "advance-liaise"), nil)
	require.Equalf(t, http.StatusOK, code, "the preparer advances: %v", body)
	assert.False(t, anyPostContains(t, h, "the Make Demands winner, moved"))
}
