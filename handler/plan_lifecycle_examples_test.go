//go:build integration

// handler/plan_lifecycle_examples_test.go — example tests that exercise the
// planLifecycle harness end-to-end. They double as regression guards for
// the two simplest plan paths: Spread Propaganda's full happy path, and
// Exchange Courtiers' messy-break preparer-asset restriction.
//
// Tests for the other 10 plans should follow the same pattern: instantiate
// newPlanLifecycle, call run() for the common shape, then layer plan-specific
// route calls via post() for any sub-flows (drafts, bouts, multi-guest rolls).

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

// TestPlanLifecycle_SpreadPropaganda_Make exercises the full common-shape
// lifecycle (prepare → resolve → roll → make-choice → complete) and asserts
// the rules-mandated make effect: an artifact representing the societal shift
// is created for the preparer.
func TestPlanLifecycle_SpreadPropaganda_Make(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "test propaganda"
	plan := h.run(PreparePlanRequest{
		PlanType:         model.PlanSpreadPropaganda,
		PreparationNotes: &notes,
	}, "make", []string{"create_artifact"})

	require.NotNil(t, plan.Result)
	assert.Equal(t, "make", *plan.Result)
	assert.Equal(t, model.PlanResolved, plan.Status)

	// The make step must have created an artifact owned by the preparer.
	rd := loadResolutionData(plan.ResolutionData)
	require.NotNil(t, rd.SpreadPropaganda)
	require.NotNil(t, rd.SpreadPropaganda.ArtifactID, "make should create an artifact")

	artifact, err := h.q.GetAssetByID(ctx, *rd.SpreadPropaganda.ArtifactID)
	require.NoError(t, err)
	assert.Equal(t, model.AssetArtifact, model.AssetType(artifact.AssetType))
	assert.Equal(t, plan.PreparerID, artifact.OwnerID)
	// The artifact is created with a placeholder name; the preparer names it
	// afterwards via name-asset (not derived from the propaganda message).
	assert.Equal(t, propagandaArtifactNameDefault, artifact.Name)
	assert.False(t, rd.SpreadPropaganda.ArtifactNamed)
}

// TestPlanLifecycle_ExchangeCourtiers_MessyBreakRestricted regression-guards
// the fix that scopes EC's messy-break to the preparer's own assets. Drives
// the lifecycle step-by-step because EC pauses for fair-trade before the
// dice roll, and the messy-break post-condition runs between make-choice
// and complete.
func TestPlanLifecycle_ExchangeCourtiers_MessyBreakRestricted(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Seed: P1 (preparer/focus) has a peer; P2 (target) has a peer EC will
	// steal; P3 (uninvolved) has a peer with marginalia — the test will try
	// to break that one and expect a 403, then break a P1 peer and succeed.
	preparerPeer := h.seedPeer(0, "P1 peer")
	targetPeer := h.seedPeer(1, "P2 target peer")
	uninvolvedPeer := h.seedPeer(2, "P3 peer")

	// Add marginalia to both candidate-break peers so messy-break has
	// something to tear.
	mPreparer, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: preparerPeer, Position: 1, Text: "on P1 peer",
	})
	require.NoError(t, err)
	mUninvolved, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID: uninvolvedPeer, Position: 1, Text: "on P3 peer",
	})
	require.NoError(t, err)

	ecNotes := "test exchange"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[1].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &ecNotes,
	})
	h.jumpToRow(*plan.RowNumber)
	// EC starts with fair-trade phase; the harness's resolve() returns
	// roll=nil for EC. Decline immediately to fall through to the dice roll.
	roll := h.resolve(plan.ID)
	assert.Nil(t, roll, "EC defers its roll behind the fair-trade step")

	// The target (P2) must name one of the preparer's peers before the preparer
	// can decline and roll (the rules' pre-roll step).
	ftPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/fair-trade"
	code, body := h.post(1, ftPath, map[string]any{"action": "offer", "offered_asset_id": preparerPeer})
	require.Equalf(t, http.StatusOK, code, "offer: %v", body)
	code, body = h.post(0, ftPath, map[string]any{"action": "decline"})
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	// fair-trade decline returns the freshly-created dice roll.
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap, "decline should create a roll")
	rollID := int64(rollMap["id"].(float64))

	h.forceRoll(rollID, "make", 0)
	h.makeChoice(plan.ID, "make", []string{"messy"})

	// Attempt messy-break against P3's (uninvolved) peer — must 403 now
	// that the handler restricts the target to the preparer's assets.
	mbPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/messy-break"
	code, body = h.post(1, mbPath, map[string]any{"marginalia_id": mUninvolved.ID})
	assert.Equalf(t, http.StatusForbidden, code,
		"messy-break on non-preparer asset should 403, got %d: %v", code, body)

	// Now break a preparer peer's marginalia — should succeed.
	code, body = h.post(1, mbPath, map[string]any{"marginalia_id": mPreparer.ID})
	require.Equalf(t, http.StatusOK, code, "messy-break on preparer asset: %v", body)

	h.complete(plan.ID)

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, refreshed.Status)
	require.NotNil(t, refreshed.Result)
	assert.Equal(t, "make", *refreshed.Result)

	// And the target peer should now belong to the preparer.
	stolen, err := h.q.GetAssetByID(ctx, targetPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[0].ID, stolen.OwnerID)
}

// TestPlanLifecycle_MakeIntroductions_NamingThenRoll exercises MI's pre-roll
// peer-naming step: prepare → resolve (no roll yet) → /create-peer ×N →
// /finalize-peers → roll → make-choice → complete. Regression-guards the
// deferred-roll behavior added when the missing peer-naming UI was filled in.
func TestPlanLifecycle_MakeIntroductions_NamingThenRoll(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "test introductions"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeIntroductions,
		PeerCount:        2,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	// MI now defers its roll: resolve must return roll=nil so the focus
	// player can name peers first.
	roll := h.resolve(plan.ID)
	assert.Nil(t, roll, "MI should not create the roll on kickoff")

	// Name the two peers.
	createPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/create-peer"
	for i, name := range []string{"Alice", "Bob"} {
		code, body := h.post(0, createPath, map[string]any{"name": name})
		require.Equalf(t, http.StatusCreated, code, "create-peer[%d]: %v", i, body)
	}

	// Trying a third peer should 409 (peer_count exceeded).
	code, _ := h.post(0, createPath, map[string]any{"name": "Carol"})
	assert.Equal(t, http.StatusConflict, code, "third create-peer should 409")

	// Finalize → roll is created.
	finalizePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/finalize-peers"
	code, body := h.post(0, finalizePath, nil)
	require.Equalf(t, http.StatusCreated, code, "finalize-peers: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap, "finalize-peers should return a roll")
	rollID := int64(rollMap["id"].(float64))

	// Finish the lifecycle normally.
	h.forceRoll(rollID, "make", 0)
	h.makeChoice(plan.ID, "make", []string{"peers_arrive"})
	h.complete(plan.ID)

	refreshed, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PlanResolved, refreshed.Status)
	require.NotNil(t, refreshed.Result)
	assert.Equal(t, "make", *refreshed.Result)

	// Both peers should exist, owned by the preparer.
	allAssets, err := h.q.ListAssetsByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	mi := 0
	for _, a := range allAssets {
		if a.Name == "Alice" || a.Name == "Bob" {
			mi++
			assert.Equal(t, h.tg.Players[0].ID, a.OwnerID)
		}
	}
	assert.Equal(t, 2, mi, "both named peers should exist in the game")
}
