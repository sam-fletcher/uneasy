//go:build integration

// handler/plan_spread_propaganda_integration_test.go — mar-path coverage for
// Spread Propaganda. These guard the rules-correct mechanical effects added
// after the audit found the handler ignored most make/mar options:
//
//   - (b) lay_low      → esteem lockout flag
//   - (a) give_peer    → completion blocked until a peer is handed over
//   - (c) break_self   → completion blocked until the preparer breaks an asset
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
