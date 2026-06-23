//go:build integration

// handler/plan_exchange_courtiers_integration_test.go — mar-path coverage for
// Exchange Courtiers. The audit found EC's mar was a no-op; the rules make it
// target-driven:
//
//   - fair_trade → the trade goes through (targeted peer → preparer)
//   - forfeit    → the target claims one of the preparer's peers
//   - riposte    → like forfeit, but the preparer may break the peer first
//
// The make path + messy-break restriction live in plan_lifecycle_examples_test.go.

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

// ecToRoll drives an EC plan to its dice roll: prepare (preparer = current
// focus), target the next player's seeded peer, jump to the row, have the target
// name a seeded preparer peer (the requested peer) as the rules require, then
// decline to force the roll. Returns the plan, the preparer/target indices, the
// targeted peer (the target's), the requested peer (the preparer's), the roll
// id, and the roll's difficulty.
func ecToRoll(t *testing.T, h *planLifecycle) (plan dbgen.Plan, preparerIdx, targetIdx int, targetPeer, requestedPeer, rollID, difficulty int64) {
	t.Helper()
	preparerIdx = h.focusPlayerIdx()
	targetIdx = (preparerIdx + 1) % len(h.tg.Players)
	targetPeer = h.seedPeer(targetIdx, "coveted peer")
	requestedPeer = h.seedPeer(preparerIdx, "requested peer")

	notes := "exchange"
	plan = h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[targetIdx].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "EC defers its roll behind the fair-trade step")

	code, body := ecOffer(t, h, targetIdx, plan.ID, requestedPeer)
	require.Equalf(t, http.StatusOK, code, "offer: %v", body)
	declinePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/fair-trade"
	code, body = h.post(preparerIdx, declinePath, map[string]any{"action": "decline"})
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap, "decline should create a roll")
	return plan, preparerIdx, targetIdx, targetPeer, requestedPeer,
		int64(rollMap["id"].(float64)), int64(rollMap["difficulty"].(float64))
}

// ecToMarWithRequest drives EC to a marred roll with the fair-trade offer in
// place. Returns the plan, indices, the targeted peer (the target's) and the
// requested peer (the preparer's).
func ecToMarWithRequest(t *testing.T, h *planLifecycle) (plan dbgen.Plan, preparerIdx, targetIdx int, targetPeer, requestedPeer int64) {
	t.Helper()
	plan, preparerIdx, targetIdx, targetPeer, requestedPeer, rollID, _ := ecToRoll(t, h)
	h.forceRoll(rollID, "mar", 0)
	return plan, preparerIdx, targetIdx, targetPeer, requestedPeer
}

// ecMakeChoice posts make-choice as a specific player (EC's mar choice is made
// by the target, who is a non-focus player).
func ecMakeChoice(t *testing.T, h *planLifecycle, playerIdx int, planID int64, choices []string) (int, map[string]any) {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/make-choice"
	return h.post(playerIdx, path, map[string]any{"result": "mar", "choices": choices})
}

func TestExchangeCourtiers_Mar_Forfeit_TakesRequestedPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, _, targetIdx, targetPeer, requestedPeer := ecToMarWithRequest(t, h)
	targetID := h.tg.Players[targetIdx].ID

	// C3 attribution: a marred EC hands the option choice to the TARGET, so the
	// bar must name the target (a non-focus, non-preparer player) — not the
	// resolving plan's focus player.
	h.assertWaitees("mar, awaiting target's choice",
		model.RowStateAwaitCourtierResponse, targetID)

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"forfeit"})
	require.Equalf(t, http.StatusOK, code, "forfeit: %v", body)

	// Forfeit takes the requested peer outright and inline — nothing further is
	// owed, so the plan auto-completes from the choice itself (no separate
	// "Complete plan" step), and the response says so.
	assert.Equal(t, true, body["resolved"], "forfeit should auto-resolve the plan")
	resolved, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status, "plan should be resolved without a Complete call")

	// The target took the peer they requested; their own targeted peer stays put
	// (no swap on a forfeit).
	claimed, err := h.q.GetAssetByID(ctx, requestedPeer)
	require.NoError(t, err)
	assert.Equal(t, targetID, claimed.OwnerID, "requested peer should belong to the target")
	tp, err := h.q.GetAssetByID(ctx, targetPeer)
	require.NoError(t, err)
	assert.Equal(t, targetID, tp.OwnerID, "targeted peer stays with the target (no swap)")
}

// ecOffer posts a fair-trade offer as the target, naming one of the preparer's
// peers to receive in exchange.
func ecOffer(t *testing.T, h *planLifecycle, targetIdx int, planID, offeredAssetID int64) (int, map[string]any) {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/fair-trade"
	return h.post(targetIdx, path, map[string]any{"action": "offer", "offered_asset_id": offeredAssetID})
}

// TestExchangeCourtiers_FairTrade_AcceptSwapsBothPeers covers the corrected
// fair-trade direction: the target names a peer in the *preparer's* retinue, and
// accepting swaps it for the targeted peer (both legs move).
func TestExchangeCourtiers_FairTrade_AcceptSwapsBothPeers(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	preparerIdx := h.focusPlayerIdx()
	targetIdx := (preparerIdx + 1) % len(h.tg.Players)
	targetPeer := h.seedPeer(targetIdx, "coveted peer")
	preparerPeer := h.seedPeer(preparerIdx, "preparer's peer")

	notes := "exchange"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[targetIdx].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &notes,
	})
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	// The target may not offer one of their *own* peers — it must name one of
	// the preparer's.
	ownPeer := h.seedPeer(targetIdx, "target's own peer")
	code, body := ecOffer(t, h, targetIdx, plan.ID, ownPeer)
	require.Equalf(t, http.StatusForbidden, code, "offering own peer should 403: %v", body)

	// Name the preparer's peer, then the preparer accepts.
	code, body = ecOffer(t, h, targetIdx, plan.ID, preparerPeer)
	require.Equalf(t, http.StatusOK, code, "offer preparer's peer: %v", body)
	acceptPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/fair-trade"
	code, body = h.post(preparerIdx, acceptPath, map[string]any{"action": "accept"})
	require.Equalf(t, http.StatusOK, code, "accept: %v", body)

	// Both legs of the swap moved.
	tp, err := h.q.GetAssetByID(ctx, targetPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[preparerIdx].ID, tp.OwnerID, "targeted peer should go to the preparer")
	pp, err := h.q.GetAssetByID(ctx, preparerPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targetIdx].ID, pp.OwnerID, "offered peer should go to the target")

	resolved, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status)
	require.NotNil(t, resolved.Result)
	assert.Equal(t, "make", *resolved.Result)
}

// TestExchangeCourtiers_Mar_FairTrade_CompletesOfferedSwap covers the second leg
// of the mar "A Fair Trade" option: when the target had named a preparer peer
// pre-roll, that peer passes back to them as the targeted peer goes to the
// preparer.
func TestExchangeCourtiers_Mar_FairTrade_CompletesOfferedSwap(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	preparerIdx := h.focusPlayerIdx()
	targetIdx := (preparerIdx + 1) % len(h.tg.Players)
	targetPeer := h.seedPeer(targetIdx, "coveted peer")
	preparerPeer := h.seedPeer(preparerIdx, "preparer's peer")

	notes := "exchange"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[targetIdx].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &notes,
	})
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	// Target names the preparer's peer pre-roll; the preparer declines and rolls.
	code, body := ecOffer(t, h, targetIdx, plan.ID, preparerPeer)
	require.Equalf(t, http.StatusOK, code, "offer: %v", body)
	declinePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/fair-trade"
	code, body = h.post(preparerIdx, declinePath, map[string]any{"action": "decline"})
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap)
	h.forceRoll(int64(rollMap["id"].(float64)), "mar", 0)

	// On the mar the target picks "A Fair Trade" — the offered swap goes through.
	// fair_trade resolves inline, so the plan auto-completes from the choice.
	code, body = ecMakeChoice(t, h, targetIdx, plan.ID, []string{"fair_trade"})
	require.Equalf(t, http.StatusOK, code, "target fair_trade: %v", body)
	assert.Equal(t, true, body["resolved"], "fair_trade should auto-resolve the plan")
	resolved, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status)

	tp, err := h.q.GetAssetByID(ctx, targetPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[preparerIdx].ID, tp.OwnerID, "targeted peer → preparer")
	pp, err := h.q.GetAssetByID(ctx, preparerPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targetIdx].ID, pp.OwnerID, "offered peer → target")
}

// TestExchangeCourtiers_LevelCap_RejectsTooHighOption covers the rules' level
// cap wired through make-choice: on a make with margin 0 the player may only
// pick Messy (level 0); Conspiracy (level 2) is rejected.
func TestExchangeCourtiers_LevelCap_RejectsTooHighOption(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _, _, _, rollID, difficulty := ecToRoll(t, h)
	// Margin 0: result == difficulty (a bare make).
	h.forceRoll(rollID, "make", int16(difficulty))

	mcPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"
	code, body := h.post(preparerIdx, mcPath, map[string]any{"result": "make", "choices": []string{"conspiracy"}})
	require.Equalf(t, http.StatusUnprocessableEntity, code,
		"conspiracy (level 2) beyond margin 0 should 422: %v", body)

	// Two options at once is rejected even when each fits.
	code, body = h.post(preparerIdx, mcPath, map[string]any{"result": "make", "choices": []string{"messy", "legal"}})
	require.Equalf(t, http.StatusUnprocessableEntity, code,
		"multi-select should 422: %v", body)

	// Messy (level 0) is allowed.
	code, body = h.post(preparerIdx, mcPath, map[string]any{"result": "make", "choices": []string{"messy"}})
	require.Equalf(t, http.StatusOK, code, "messy within margin: %v", body)
}

// TestExchangeCourtiers_Mar_Riposte_BreaksThenSurrendersRequestedPeer covers
// riposte: the preparer goes first, the break must land on the *requested* peer,
// and afterwards that same peer passes to the target (damaged).
func TestExchangeCourtiers_Mar_Riposte_BreaksThenSurrendersRequestedPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, targetIdx, targetPeer, requestedPeer := ecToMarWithRequest(t, h)
	targetID := h.tg.Players[targetIdx].ID

	// Two marginalia on the requested peer so the break doesn't destroy it; a
	// decoy peer with its own marginalia the preparer must NOT be able to break.
	firstMarg, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: requestedPeer, Position: 1, Text: "note",
	})
	require.NoError(t, err)
	_, err = h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: requestedPeer, Position: 2, Text: "second note",
	})
	require.NoError(t, err)
	decoy := h.seedPeer(preparerIdx, "decoy peer")
	decoyMarg, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: decoy, Position: 1, Text: "decoy note",
	})
	require.NoError(t, err)

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"riposte"})
	require.Equalf(t, http.StatusOK, code, "riposte: %v", body)

	// A riposte waits on the PREPARER first (to break or surrender).
	h.assertWaitees("riposte awaits preparer", model.RowStatePlanResolving, plan.PreparerID)

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/riposte-break"
	// The break must be on the requested peer, not some other peer.
	code, body = h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": decoyMarg.ID})
	require.Equalf(t, http.StatusBadRequest, code, "breaking a non-requested peer should 400: %v", body)

	// Break the requested peer, which then passes to the target. The riposte
	// break is the final mar step, so the plan auto-completes here.
	code, body = h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": firstMarg.ID})
	require.Equalf(t, http.StatusOK, code, "riposte-break: %v", body)
	assert.Equal(t, true, body["resolved"], "riposte break should auto-resolve the plan")
	torn, err := h.q.GetMarginaliaByID(ctx, firstMarg.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "the requested peer's marginalia should be torn")

	resolved, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status)

	claimed, err := h.q.GetAssetByID(ctx, requestedPeer)
	require.NoError(t, err)
	assert.Equal(t, targetID, claimed.OwnerID, "requested peer → target (damaged)")
	tp, err := h.q.GetAssetByID(ctx, targetPeer)
	require.NoError(t, err)
	assert.Equal(t, targetID, tp.OwnerID, "targeted peer stays with the target (no swap)")
	d, err := h.q.GetAssetByID(ctx, decoy)
	require.NoError(t, err)
	assert.Equal(t, plan.PreparerID, d.OwnerID, "decoy stays with the preparer")
}

// TestExchangeCourtiers_Mar_Riposte_SurrenderIntact covers the riposte skip: the
// preparer surrenders the requested peer without damaging it.
func TestExchangeCourtiers_Mar_Riposte_SurrenderIntact(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, targetIdx, _, requestedPeer := ecToMarWithRequest(t, h)

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"riposte"})
	require.Equalf(t, http.StatusOK, code, "riposte: %v", body)

	// Preparer surrenders intact; the requested peer passes to the target and the
	// plan auto-completes (no separate Complete step).
	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/riposte-break"
	code, body = h.post(preparerIdx, breakPath, map[string]any{"action": "skip"})
	require.Equalf(t, http.StatusOK, code, "riposte surrender: %v", body)
	assert.Equal(t, true, body["resolved"], "riposte surrender should auto-resolve the plan")

	resolved, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, resolved.Status)

	claimed, err := h.q.GetAssetByID(ctx, requestedPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targetIdx].ID, claimed.OwnerID)
}
