//go:build integration

// handler/plan_spread_propaganda_integration_test.go — mar-path coverage for
// Spread Propaganda. These guard the rules-correct mechanical effects added
// after the audit found the handler ignored most make/mar options:
//
//   - (b) lay_low      → esteem lockout flag
//   - (a) give_peer    → completion blocked until a peer is handed over
//   - (c) break_self   → completion blocked until the preparer breaks an asset
//
// and the soft-lock class: a mar can commit more give_peer / break_self picks than
// the preparer has targets for, after which /complete refuses forever. These
// tests drive that over-pick all the way to /complete through sp-forfeit-step.
//
// The make path (artifact creation) is covered in plan_lifecycle_examples_test.go.

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

// spPrepareToMar drives an SP plan to a marred roll and returns the plan plus
// the preparer's index. The plan is left in resolving status, pre make-choice.
func spPrepareToMar(t *testing.T, h *planLifecycle) (dbgen.Plan, int) {
	t.Helper()
	notes := "rabble-rousing"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadPropaganda,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll, "SP creates its roll on resolve")
	h.forceRoll(roll.ID, "mar", 0)

	preparerIdx := -1
	for i, p := range h.tg.Players {
		if p.ID == plan.PreparerID {
			preparerIdx = i
		}
	}
	require.GreaterOrEqual(t, preparerIdx, 0, "preparer must be one of the seeded players")
	return plan, preparerIdx
}

// TestMakeChoice_EnforcesOptionBudget proves the server-side count cap is wired
// into make-choice: with a consistent mar roll (budget = difficulty − result =
// 1), two options are rejected and one is accepted.
func TestMakeChoice_EnforcesOptionBudget(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	notes := "propaganda"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadPropaganda,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll)
	// Consistent mar: result = difficulty − 1, so the mar budget is exactly 1.
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)

	preparerIdx := h.preparerIdxFor(plan.ID)
	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"
	code, body := h.post(preparerIdx, path, map[string]any{
		"result": "mar", "choices": []string{"lay_low", "give_peer"},
	})
	assert.Equalf(t, http.StatusUnprocessableEntity, code, "over-budget should 422: %v", body)

	code, body = h.post(preparerIdx, path, map[string]any{
		"result": "mar", "choices": []string{"lay_low"},
	})
	require.Equalf(t, http.StatusOK, code, "within budget should succeed: %v", body)
}

func TestSpreadPropaganda_Mar_LayLow_SetsEsteemLockout(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, _ := spPrepareToMar(t, h)
	h.makeChoice(plan.ID, "mar", []string{"lay_low"})
	h.complete(plan.ID)

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	require.NotNil(t, rd.SpreadPropaganda)
	assert.True(t, rd.SpreadPropaganda.EsteemLockout, "lay_low must set the esteem lockout")
}

func TestSpreadPropaganda_Mar_GivePeer_BlocksUntilTransferred(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx := spPrepareToMar(t, h)
	recipientIdx := (preparerIdx + 1) % len(h.tg.Players)
	gift := h.seedPeer(preparerIdx, "doomed peer")

	h.makeChoice(plan.ID, "mar", []string{"give_peer"})

	// Completion is blocked until the peer is actually handed over. Completion
	// is preparer-gated, so drive it as the preparer.
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body := h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked: %v", body)

	// Hand the peer to the recipient.
	givePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/give-peer"
	code, body = h.post(preparerIdx, givePath, map[string]any{
		"peer_asset_id": gift,
		"to_player_id":  h.tg.Players[recipientIdx].ID,
	})
	require.Equalf(t, http.StatusOK, code, "give-peer: %v", body)

	// Now completion succeeds and the peer changed hands.
	h.complete(plan.ID)
	moved, err := h.q.GetAssetByID(ctx, gift)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[recipientIdx].ID, moved.OwnerID, "peer should belong to recipient")
}

func TestSpreadPropaganda_Mar_GivePeer_RejectsForeignPeer(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx := spPrepareToMar(t, h)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	notMine := h.seedPeer(otherIdx, "someone else's peer")

	h.makeChoice(plan.ID, "mar", []string{"give_peer"})

	givePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/give-peer"
	code, body := h.post(preparerIdx, givePath, map[string]any{
		"peer_asset_id": notMine,
		"to_player_id":  h.tg.Players[otherIdx].ID,
	})
	assert.Equalf(t, http.StatusForbidden, code,
		"giving away a peer you don't own should 403, got %d: %v", code, body)
}

func TestSpreadPropaganda_Mar_BreakSelf_BlocksUntilBroken(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx := spPrepareToMar(t, h)

	// Give the preparer an asset with a marginalia to tear.
	ownPeer := h.seedPeer(preparerIdx, "thin-skinned peer")
	m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: ownPeer, Position: 1, Text: "a cherished note",
	})
	require.NoError(t, err)

	h.makeChoice(plan.ID, "mar", []string{"break_self"})

	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body := h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked: %v", body)

	// Breaking someone else's asset is rejected.
	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-self"
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	foreignPeer := h.seedPeer(otherIdx, "not the preparer's")
	mForeign, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: foreignPeer, Position: 1, Text: "off-limits",
	})
	require.NoError(t, err)
	code, body = h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": mForeign.ID})
	assert.Equalf(t, http.StatusForbidden, code, "breaking a foreign asset should 403: %v", body)

	// Break the preparer's own marginalia.
	code, body = h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": m.ID})
	require.Equalf(t, http.StatusOK, code, "break-self: %v", body)

	h.complete(plan.ID)

	torn, err := h.q.GetMarginaliaByID(ctx, m.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "the preparer's marginalia should be torn")
}

// TestSpreadPropaganda_Mar_GivePeer_Repeatable proves the mar is genuinely
// repeatable per the rules ("Choose options equal to (difficulty − result)
// (repeatable)"): picking give_peer twice must require two separate
// give-peer transfers before completion, and a third call beyond what was
// picked must be rejected.
func TestSpreadPropaganda_Mar_GivePeer_Repeatable(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// spPrepareToMar forces result=0, so the budget equals the preparer's
	// esteem difficulty — at least 2 in a 3-player game (dummy occupies
	// rank 1), enough room to pick give_peer twice.
	plan, preparerIdx := spPrepareToMar(t, h)
	recipientIdx := (preparerIdx + 1) % len(h.tg.Players)
	giftA := h.seedPeer(preparerIdx, "first doomed peer")
	giftB := h.seedPeer(preparerIdx, "second doomed peer")

	h.makeChoice(plan.ID, "mar", []string{"give_peer", "give_peer"})

	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body := h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked before either transfer: %v", body)

	givePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/give-peer"
	code, body = h.post(preparerIdx, givePath, map[string]any{
		"peer_asset_id": giftA,
		"to_player_id":  h.tg.Players[recipientIdx].ID,
	})
	require.Equalf(t, http.StatusOK, code, "first give-peer: %v", body)

	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should still be blocked after one of two: %v", body)

	code, body = h.post(preparerIdx, givePath, map[string]any{
		"peer_asset_id": giftB,
		"to_player_id":  h.tg.Players[recipientIdx].ID,
	})
	require.Equalf(t, http.StatusOK, code, "second give-peer: %v", body)

	// A third call beyond the two picked is rejected.
	giftC := h.seedPeer(preparerIdx, "not part of the deal")
	code, body = h.post(preparerIdx, givePath, map[string]any{
		"peer_asset_id": giftC,
		"to_player_id":  h.tg.Players[recipientIdx].ID,
	})
	assert.Equalf(t, http.StatusConflict, code, "a third give-peer beyond the picked count should be rejected: %v", body)

	h.complete(plan.ID)

	movedA, err := h.q.GetAssetByID(ctx, giftA)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[recipientIdx].ID, movedA.OwnerID, "first peer should belong to recipient")
	movedB, err := h.q.GetAssetByID(ctx, giftB)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[recipientIdx].ID, movedB.OwnerID, "second peer should belong to recipient")
}

// TestSpreadPropaganda_Mar_CounterProp_DoesNotDoubleFire proves that picking
// "counter_prop" twice in one submission — legal under the repeatable budget
// cap, since the server only enforces an upper bound on total picks — still
// spawns exactly one recursive propaganda plan. Unlike give_peer/break_self,
// co-opt fires its side effect immediately inside ApplyChoice rather than
// deferring to a gated sub-flow, so it needs its own guard against a
// duplicate pick in the same choices list.
func TestSpreadPropaganda_Mar_CounterProp_DoesNotDoubleFire(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "propaganda"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadPropaganda,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolve(plan.ID)
	require.NotNil(t, roll, "SP creates its roll on resolve")

	preparerIdx := h.preparerIdxFor(plan.ID)
	interfererIdx := (preparerIdx + 1) % len(h.tg.Players)
	_, err := h.q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
		RollID: roll.ID, PlayerID: h.tg.Players[interfererIdx].ID, IsInterference: true,
	})
	require.NoError(t, err)

	h.forceRoll(roll.ID, "mar", 0)
	h.makeChoice(plan.ID, "mar", []string{"counter_prop", "counter_prop"})

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	require.NotNil(t, rd.SpreadPropaganda)
	require.NotNil(t, rd.SpreadPropaganda.RecursivePlanID, "co-opt should still fire once")

	plans, err := h.q.ListPlansByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	spCount := 0
	for _, p := range plans {
		if p.PlanType == model.PlanSpreadPropaganda {
			spCount++
		}
	}
	assert.Equal(t, 2, spCount, "counter_prop picked twice should still spawn exactly one recursive plan")
}

// ── Soft-lock regression: over-picked mar steps ──────────────────────────────

// spMarBudget is how many mar picks the forced roll allows: (adjusted) difficulty
// minus the forced result.
func spMarBudget(t *testing.T, h *planLifecycle, planID int64) int {
	t.Helper()
	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	diff := roll.Difficulty
	if roll.AdjustedDifficulty != nil {
		diff = *roll.AdjustedDifficulty
	}
	require.NotNil(t, roll.Result)
	return int(diff - *roll.Result)
}

// spStripRetinue destroys everything players[idx] holds, so a test can then seed
// exactly the targets a give_peer / break_self pick can be spent on.
func spStripRetinue(t *testing.T, h *planLifecycle, idx int) {
	t.Helper()
	ctx := context.Background()
	assets, err := h.q.ListAssetsByOwner(ctx, h.tg.Players[idx].ID)
	require.NoError(t, err)
	for _, a := range assets {
		require.NoError(t, h.q.DestroyAsset(ctx, a.ID))
	}
}

func spPath(planID int64, route string) string {
	return "/api/plans/" + strconv.FormatInt(planID, 10) + "/" + route
}

// TestSpreadPropaganda_Mar_GivePeer_OverPickCanStillComplete is THE soft-lock
// regression. A mar is repeatable, so the preparer can commit give_peer once per
// point of budget — including more times than they have peers. Before the
// sp-forfeit-step hatch the surplus picks could never be satisfied: after the last
// peer left, /complete answered "N of M give-peer pick(s) still to resolve"
// forever and nobody at the table could finish the row.
//
// The picker now caps give_peer at the live peer count, so this needs a stale
// client or a peer lost between commit and resolution; the server accepts the
// over-pick (it has no target count at commit time), and the hatch must take it
// from there to a completed plan.
func TestSpreadPropaganda_Mar_GivePeer_OverPickCanStillComplete(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx := spPrepareToMar(t, h)
	budget := spMarBudget(t, h, plan.ID)
	require.GreaterOrEqual(t, budget, 2, "the over-pick needs room for at least one surplus pick")
	recipientID := h.tg.Players[(preparerIdx+1)%len(h.tg.Players)].ID

	// Exactly budget−1 peers: the last pick has nothing to be spent on.
	spStripRetinue(t, h, preparerIdx)
	peers := make([]int64, 0, budget-1)
	for i := range budget - 1 {
		peers = append(peers, h.seedPeer(preparerIdx, "doomed peer "+strconv.Itoa(i)))
	}

	choices := make([]string, budget)
	for i := range choices {
		choices[i] = spOptGivePeer
	}
	h.makeChoice(plan.ID, "mar", choices)

	// The hatch refuses while a peer is still there to give: the client must not be
	// able to forfeit its way out of an obligation it could still meet.
	code, body := h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptGivePeer})
	assert.Equalf(t, http.StatusConflict, code, "forfeit must be refused while a peer remains: %v", body)
	assert.Contains(t, body["error"], "valid targets remain")

	for _, peerID := range peers {
		code, body = h.post(preparerIdx, spPath(plan.ID, "give-peer"), map[string]any{
			"peer_asset_id": peerID, "to_player_id": recipientID,
		})
		require.Equalf(t, http.StatusOK, code, "give-peer: %v", body)
	}

	// The wedge from the report: every peer is gone, one pick is still owed.
	code, body = h.post(preparerIdx, spPath(plan.ID, "complete"), nil)
	require.Equalf(t, http.StatusConflict, code, "complete must be blocked on the undischargeable pick: %v", body)
	assert.Contains(t, body["error"], "1 of "+strconv.Itoa(budget)+" give-peer pick(s) still to resolve")

	// The escape hatch discharges exactly the surplus.
	code, body = h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptGivePeer})
	require.Equalf(t, http.StatusOK, code, "forfeit with no peer left should succeed: %v", body)
	assert.EqualValues(t, 1, body["forfeited"])

	// Nothing left to forfeit — a second call is refused, not a silent no-op.
	code, body = h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptGivePeer})
	assert.Equalf(t, http.StatusConflict, code, "nothing remains to forfeit: %v", body)

	h.complete(plan.ID)

	done, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, done.Status)
	rd := loadResolutionData(done.ResolutionData)
	assert.Equal(t, budget, rd.SpreadPropaganda.GivePeerDone, "every pick is accounted for")
	assert.Contains(t, saAllLogBodies(t, h), "forfeited", "the forfeit is logged for transparency")
	for _, peerID := range peers {
		moved, aErr := h.q.GetAssetByID(ctx, peerID)
		require.NoError(t, aErr)
		assert.Equal(t, recipientID, moved.OwnerID, "the peers that existed really changed hands")
	}
}

// TestSpreadPropaganda_Mar_BreakSelf_BlankAssetsAreBreakable proves the three
// things the blank-asset sweep (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D3) owed
// break_self, and that the forfeit hatch agrees with them:
//
//   - a blank asset is a legal target: break-self takes it with no marginalia_id
//     and the break destroys it outright;
//   - it counts as a live target for the hatch (assetIsBreakable), so the hatch
//     refuses while only a blank asset remains;
//   - once nothing breakable is left, the surplus pick can be forfeited and the
//     row completes.
func TestSpreadPropaganda_Mar_BreakSelf_BlankAssetsAreBreakable(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx := spPrepareToMar(t, h)
	budget := spMarBudget(t, h, plan.ID)
	require.GreaterOrEqual(t, budget, 2)

	// budget−1 breaks in total: one blank asset (a single outright destroy) and a
	// peer carrying budget−2 tearable marginalia.
	spStripRetinue(t, h, preparerIdx)
	blank := h.seedPeer(preparerIdx, "blank confidant")
	var margIDs []int64
	var noted int64
	if budget > 2 {
		noted = h.seedPeer(preparerIdx, "annotated confidant")
		for i := range budget - 2 {
			m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
				AssetID: noted, Position: int16(i + 1), Text: "note " + strconv.Itoa(i),
			})
			require.NoError(t, err)
			margIDs = append(margIDs, m.ID)
		}
	}

	choices := make([]string, budget)
	for i := range choices {
		choices[i] = spOptBreakSelf
	}
	h.makeChoice(plan.ID, "mar", choices)

	// Tear every marginalia first, leaving ONLY the blank asset standing.
	for _, mID := range margIDs {
		code, body := h.post(preparerIdx, spPath(plan.ID, "break-self"), map[string]any{"marginalia_id": mID})
		require.Equalf(t, http.StatusOK, code, "break-self (marginalia only, asset derived): %v", body)
	}

	// A blank asset is still a target, so the hatch must refuse.
	code, body := h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptBreakSelf})
	assert.Equalf(t, http.StatusConflict, code,
		"a blank asset is breakable, so the forfeit must be refused: %v", body)

	// Naming an asset with notes but no marginalia is rejected; the blank one needs none.
	if noted != 0 {
		code, body = h.post(preparerIdx, spPath(plan.ID, "break-self"), map[string]any{"asset_id": noted})
		assert.Truef(t, code == http.StatusBadRequest || code == http.StatusConflict,
			"breaking a destroyed/annotated asset without a marginalia must fail, got %d: %v", code, body)
	}
	code, body = h.post(preparerIdx, spPath(plan.ID, "break-self"), map[string]any{"asset_id": blank})
	require.Equalf(t, http.StatusOK, code, "a blank asset breaks with no marginalia_id: %v", body)
	assert.Equal(t, true, body["destroyed"], "breaking a blank asset destroys it outright")
	gone, err := h.q.GetAssetByID(ctx, blank)
	require.NoError(t, err)
	assert.True(t, gone.IsDestroyed)

	// Nothing breakable remains and one pick is still owed: the hatch takes it.
	code, body = h.post(preparerIdx, spPath(plan.ID, "complete"), nil)
	require.Equalf(t, http.StatusConflict, code, "the surplus pick still blocks completion: %v", body)
	code, body = h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptBreakSelf})
	require.Equalf(t, http.StatusOK, code, "forfeit with nothing left to break: %v", body)
	assert.EqualValues(t, 1, body["forfeited"])

	h.complete(plan.ID)
}

// TestSpreadPropaganda_ForfeitStep_RejectsBadCallers keeps the hatch honest:
// only the resolution actor may use it, only for the two depletable steps, and
// not before anything was picked.
func TestSpreadPropaganda_ForfeitStep_RejectsBadCallers(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx := spPrepareToMar(t, h)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	spStripRetinue(t, h, preparerIdx)
	h.makeChoice(plan.ID, "mar", []string{spOptGivePeer})

	code, body := h.post(otherIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptGivePeer})
	assert.Equalf(t, http.StatusForbidden, code, "a non-actor cannot forfeit: %v", body)

	code, body = h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptLayLow})
	assert.Equalf(t, http.StatusBadRequest, code, "lay_low is not a depletable step: %v", body)

	// break_self was never picked, so it has nothing to forfeit even with no assets.
	code, body = h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptBreakSelf})
	assert.Equalf(t, http.StatusConflict, code, "no break_self pick was committed: %v", body)

	code, body = h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptGivePeer})
	require.Equalf(t, http.StatusOK, code, "the preparer's own forfeit is fine: %v", body)
}

// TestSpreadPropaganda_Mar_CounterProp_RefusedCleanlyWhenNobodyInterfered proves
// option (d) is checked BEFORE anything is applied. With no interference dice it
// used to fail on apply with a leaked backend string ("co-opt is not available:
// no interference dice were committed to this roll") — and, because earlier picks
// in the same submission had already run, after posting their log lines. The
// picker greys the option out; this guards the stale-client path behind it.
func TestSpreadPropaganda_Mar_CounterProp_RefusedCleanlyWhenNobodyInterfered(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx := spPrepareToMar(t, h)
	require.GreaterOrEqual(t, spMarBudget(t, h, plan.ID), 2)

	code, body := h.post(preparerIdx, spPath(plan.ID, "make-choice"), map[string]any{
		"result": "mar", "choices": []string{spOptLayLow, spOptCounterProp},
	})
	require.NotEqualf(t, http.StatusOK, code, "co-opt with no interferer cannot succeed: %v", body)
	msg, _ := body["error"].(string)
	assert.Contains(t, msg, "Nobody interfered", "the reason is written for a player")
	assert.NotContains(t, msg, "interference dice", "no backend jargon leaks")

	// Nothing was applied: no committed choices, no lockout, no stray log line.
	after, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(after.ResolutionData)
	assert.Empty(t, rd.MakeMarChoices, "a refused submission commits nothing")
	if rd.SpreadPropaganda != nil {
		assert.False(t, rd.SpreadPropaganda.EsteemLockout, "lay_low must not have run ahead of the failure")
	}
	assert.NotContains(t, saAllLogBodies(t, h), "keep their head down")

	// And the plan is not wedged: a valid submission still goes through.
	h.makeChoice(plan.ID, "mar", []string{spOptLayLow, spOptLayLow})
}

// TestSpreadPropaganda_CoOptBlocker_RecursivePlan covers the other reason the
// option is unavailable. It needs no database: the depth cap is checked first.
func TestSpreadPropaganda_CoOptBlocker_RecursivePlan(t *testing.T) {
	parent := int64(7)
	rd := ResolutionData{SpreadPropaganda: &SpreadPropagandaResolutionData{OriginalPlanID: &parent}}
	_, reason, err := spCoOptInterferers(context.Background(), nil, &dbgen.Plan{ID: 8}, &rd)
	require.NoError(t, err)
	assert.Contains(t, reason, "counter-propaganda")
}

// TestSpreadPropaganda_PerformStepsWinner_FullMarFlow proves a Make Demands
// perform_steps winner holds every post-commit mar step, not just the pick: they
// give the peer, they may forfeit the surplus, the preparer is refused both, and
// the Waiting-On bar names the winner while the steps are owed (then hands back
// to the preparer, who completes).
func TestSpreadPropaganda_PerformStepsWinner_FullMarFlow(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx := spPrepareToMar(t, h)
	winnerIdx := (preparerIdx + 1) % len(h.tg.Players)
	budget := spMarBudget(t, h, plan.ID)
	require.GreaterOrEqual(t, budget, 2)
	preparerID := h.tg.Players[preparerIdx].ID
	winnerID := h.tg.Players[winnerIdx].ID

	spStripRetinue(t, h, preparerIdx)
	peer := h.seedPeer(preparerIdx, "the one peer")

	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: winnerID,
	})

	choices := make([]string, budget)
	for i := range choices {
		choices[i] = spOptGivePeer
	}
	code, body := h.post(winnerIdx, spPath(plan.ID, "make-choice"), map[string]any{
		"result": "mar", "choices": choices,
	})
	require.Equalf(t, http.StatusOK, code, "the winner drives make-choice: %v", body)

	// While picks are owed, the table is waiting on the winner — not the preparer,
	// who is locked out of every one of them.
	h.assertWaitees("steps owed", model.RowStatePlanResolving, winnerID)

	code, body = h.post(preparerIdx, spPath(plan.ID, "give-peer"), map[string]any{
		"peer_asset_id": peer, "to_player_id": winnerID,
	})
	assert.Equalf(t, http.StatusForbidden, code, "the preparer is locked out of give-peer: %v", body)

	code, body = h.post(winnerIdx, spPath(plan.ID, "give-peer"), map[string]any{
		"peer_asset_id": peer, "to_player_id": winnerID,
	})
	require.Equalf(t, http.StatusOK, code, "the winner performs give-peer: %v", body)

	code, body = h.post(preparerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptGivePeer})
	assert.Equalf(t, http.StatusForbidden, code, "the preparer is locked out of the forfeit too: %v", body)
	code, body = h.post(winnerIdx, spPath(plan.ID, "sp-forfeit-step"), map[string]any{"step": spOptGivePeer})
	require.Equalf(t, http.StatusOK, code, "the winner may forfeit the surplus: %v", body)

	// Nothing is owed any more; completion is the preparer's (the hand-back).
	h.assertWaitees("hand-back", model.RowStatePlanResolving, preparerID)
	h.complete(plan.ID)
}
