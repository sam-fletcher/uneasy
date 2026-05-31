//go:build integration

// handler/plan_propose_decree_integration_test.go — end-to-end coverage for
// Propose Decree. Drives the council → call-roll → enact → addendum → complete
// flow and asserts the rules-correct outcomes:
//
//   - make: a law row is created AND a resource asset representing it.
//   - mar:  a law row is created with NO resource asset.
//   - action-log entries land for call-roll and enactment.

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// pdCallRoll drives the signatory's call-roll and returns the created roll.
func pdCallRoll(t *testing.T, h *planLifecycle, planID int64, signatoryIdx int) dbgen.DiceRoll {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/call-roll"
	code, body := h.post(signatoryIdx, path, nil)
	require.Equalf(t, http.StatusOK, code, "call-roll: %v", body)
	rollBlob, err := json.Marshal(body["roll"])
	require.NoError(t, err)
	var roll dbgen.DiceRoll
	require.NoError(t, json.Unmarshal(rollBlob, &roll))
	return roll
}

// pdResourceAssets returns the non-destroyed resource assets in the game whose
// name marks them as an enacted law.
func pdLawAssets(t *testing.T, h *planLifecycle) []dbgen.Asset {
	t.Helper()
	all, err := h.q.ListAssetsByGame(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	var out []dbgen.Asset
	for _, a := range all {
		if a.AssetType == model.AssetResource && !a.IsDestroyed && strings.HasPrefix(a.Name, "Law: ") {
			out = append(out, a)
		}
	}
	return out
}

func pdSystemPostBodies(t *testing.T, h *planLifecycle) []string {
	t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "plan.propose_decree" {
			out = append(out, p.Body)
		}
	}
	return out
}

// pdPrepareAndResolve prepares a decree as players[focusIdx] and kicks off
// resolution (no roll yet — call-roll creates it). Returns the plan.
func pdPrepareAndResolve(t *testing.T, h *planLifecycle, focusIdx int) dbgen.Plan {
	t.Helper()
	h.setFocus(focusIdx)
	notes := "All trade taxes are halved"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanProposeDecree,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "Propose Decree creates its roll via call-roll, not resolve")
	return plan
}

// pdPlayerIdx maps a player id to its index in the harness player list.
func pdPlayerIdx(t *testing.T, h *planLifecycle, playerID int64) int {
	t.Helper()
	for i, p := range h.tg.Players {
		if p.ID == playerID {
			return i
		}
	}
	t.Fatalf("player %d not in harness", playerID)
	return -1
}

func pdData(t *testing.T, h *planLifecycle, planID int64) ProposeDecreeResolutionData {
	t.Helper()
	p, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(t, err)
	rd := loadResolutionData(p.ResolutionData)
	return *rd.EnsureProposeDecree()
}

// TestProposeDecree_Make_CreatesLawAndAsset proves a made decree (preparer is
// the Monarch, so also the signatory) writes a law row AND a resource asset,
// requires the addendum to be placed, and emits the enactment action-log.
func TestProposeDecree_Make_CreatesLawAndAsset(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Focus on player[0] (power rank 1 = Monarch): preparer == signatory, and
	// no one outranks them, so the council is just the preparer (no amenders).
	plan := pdPrepareAndResolve(t, h, 0)
	preparerIdx := h.preparerIdxFor(plan.ID)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	require.Equal(t, h.tg.Players[preparerIdx].ID, *pd.SignatoryID, "Monarch preparer is the signatory")

	lawsBefore, err := h.q.ListLaws(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	assetsBefore := len(pdLawAssets(t, h))

	roll := pdCallRoll(t, h, plan.ID, preparerIdx)
	h.forceRoll(roll.ID, "make", roll.Difficulty)
	h.makeChoice(plan.ID, "make", []string{})

	// Completion is blocked until the signatory places the addendum.
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body := h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should block pre-addendum: %v", body)

	// Place an "and" addendum, then complete.
	addPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/set-addendum"
	code, body = h.post(preparerIdx, addPath, map[string]any{"connector": "and", "addendum": "exempting grain"})
	require.Equalf(t, http.StatusOK, code, "set-addendum: %v", body)
	h.complete(plan.ID)

	lawsAfter, err := h.q.ListLaws(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, lawsAfter, len(lawsBefore)+1, "a law row should be created")
	assert.Equal(t, assetsBefore+1, len(pdLawAssets(t, h)), "a made decree creates a resource asset")

	// The addendum is composed onto the law row as "and …".
	law := lawsAfter[len(lawsAfter)-1]
	require.NotNil(t, law.Addendum)
	assert.Equal(t, "and exempting grain", *law.Addendum)

	// The enactment action-log post landed.
	posts := pdSystemPostBodies(t, h)
	assert.Conditionf(t, func() bool {
		for _, b := range posts {
			if strings.Contains(b, "enacted") && strings.Contains(b, "resource was created") {
				return true
			}
		}
		return false
	}, "expected an enactment action-log post; got %v", posts)
}

// TestProposeDecree_Mar_AmendChainThenAddendum drives the full marred flow:
// the council amends the body in turn (lowest power first), then the signatory
// places the addendum, and only then can the preparer complete. No asset.
func TestProposeDecree_Mar_AmendChainThenAddendum(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Focus on player[2] (power rank 3): players[0] (rank 1) and players[1]
	// (rank 2) outrank them and are auto-seated. Signatory = players[0].
	// Amenders, lowest power first = players[1] then players[0].
	plan := pdPrepareAndResolve(t, h, 2)
	preparerIdx := h.preparerIdxFor(plan.ID)
	assetsBefore := len(pdLawAssets(t, h))

	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	sigIdx := pdPlayerIdx(t, h, *pd.SignatoryID)

	roll := pdCallRoll(t, h, plan.ID, sigIdx) // signatory calls the roll
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)
	h.makeChoice(plan.ID, "mar", []string{})

	// Amendment order should be the two higher-power members, lowest first.
	pd = pdData(t, h, plan.ID)
	require.Len(t, pd.AmendmentOrder, 2, "both non-preparer council members amend")
	require.Equal(t, h.tg.Players[preparerIdx].ID == pd.AmendmentOrder[0], false)

	amendPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/amend-decree"

	// Wrong player can't jump the queue.
	wrongIdx := pdPlayerIdx(t, h, pd.AmendmentOrder[1])
	code, body := h.post(wrongIdx, amendPath, map[string]any{"text": "out of turn"})
	require.Equalf(t, http.StatusConflict, code, "out-of-turn amend should 409: %v", body)

	// First amender (lowest power) goes, then the second.
	for i, amenderID := range pd.AmendmentOrder {
		idx := pdPlayerIdx(t, h, amenderID)
		code, body = h.post(idx, amendPath, map[string]any{
			"text": "Amended body v" + strconv.Itoa(i+1),
		})
		require.Equalf(t, http.StatusOK, code, "amend %d: %v", i+1, body)
	}

	// Completion still blocked until the addendum is placed.
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should block pre-addendum: %v", body)

	// Signatory places a blank addendum (allowed — required step, optional text).
	addPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/set-addendum"
	code, body = h.post(sigIdx, addPath, map[string]any{"connector": "", "addendum": ""})
	require.Equalf(t, http.StatusOK, code, "blank addendum: %v", body)
	h.complete(plan.ID)

	laws, err := h.q.ListLaws(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.NotEmpty(t, laws, "a law row should still be created on a mar")
	law := laws[len(laws)-1]
	assert.Equal(t, "Amended body v2", law.Text, "final body is the last amender's text")
	assert.Nil(t, law.Addendum, "blank addendum leaves the rider empty")
	assert.Equal(t, assetsBefore, len(pdLawAssets(t, h)), "a marred decree creates NO resource asset")
}
