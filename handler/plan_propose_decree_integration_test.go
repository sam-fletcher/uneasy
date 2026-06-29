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
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
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

// pdDeclineRemaining has every eligible non-council player decline to join, so
// the signatory may close the council and call the roll. The council seated at
// OnResolve already holds the preparer and every higher-power member, so the
// players left out are exactly the lower-power eligible joiners. A no-op when the
// preparer is lowest power (everyone is auto-seated). Tests that don't exercise
// the join flow call this to clear the pre-roll decision gate.
func pdDeclineRemaining(t *testing.T, h *planLifecycle, planID int64) {
	t.Helper()
	pd := pdData(t, h, planID)
	for i, p := range h.tg.Players {
		if slices.Contains(pd.SignatoryPlayerIDs, p.ID) {
			continue
		}
		path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/decline-council"
		code, body := h.post(i, path, nil)
		require.Equalf(t, http.StatusOK, code, "decline-council: %v", body)
	}
}

// pdResourceAssets returns the non-destroyed resource assets created by an
// enacted decree. A made decree creates the resource with a placeholder name
// (the preparer names it afterwards via name-asset); these tests don't rename
// it, so the placeholder identifies them.
func pdLawAssets(t *testing.T, h *planLifecycle) []dbgen.Asset {
	t.Helper()
	all, err := h.q.ListAssetsByGame(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	var out []dbgen.Asset
	for _, a := range all {
		if a.AssetType == model.AssetResource && !a.IsDestroyed && a.Name == lawResourceNameDefault {
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

// pdPrepareResolveNoDebate prepares a decree as players[focusIdx] and kicks off
// resolution, stopping BEFORE the preparer opens the debate (so a test can
// exercise the drafting / debate-gate state). Returns the plan.
func pdPrepareResolveNoDebate(t *testing.T, h *planLifecycle, focusIdx int) dbgen.Plan {
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

// pdStartDebate has the preparer finalize the text and open the debate — the
// required step before the signatory can call the roll.
func pdStartDebate(t *testing.T, h *planLifecycle, planID int64) {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/start-debate"
	code, body := h.post(h.preparerIdxFor(planID), path, map[string]any{"text": "All trade taxes are halved"})
	require.Equalf(t, http.StatusOK, code, "start-debate: %v", body)
}

// pdPrepareAndResolve prepares a decree, kicks off resolution, and opens the
// debate — the common path to a rollable decree (no roll yet; call-roll creates
// it). Use pdPrepareResolveNoDebate when a test needs the pre-debate state.
func pdPrepareAndResolve(t *testing.T, h *planLifecycle, focusIdx int) dbgen.Plan {
	t.Helper()
	plan := pdPrepareResolveNoDebate(t, h, focusIdx)
	pdStartDebate(t, h, plan.ID)
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

	// Lower-power players must decide before the roll can be called.
	pdDeclineRemaining(t, h, plan.ID)
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

// TestProposeDecree_NameAsset_RenamesResource proves the preparer names the
// resource a made decree created (it starts with a placeholder) via name-asset,
// the resolution data records it as named, and non-preparers are rejected.
func TestProposeDecree_NameAsset_RenamesResource(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan := pdPrepareAndResolve(t, h, 0)
	preparerIdx := h.preparerIdxFor(plan.ID)
	pdDeclineRemaining(t, h, plan.ID)
	roll := pdCallRoll(t, h, plan.ID, preparerIdx)
	h.forceRoll(roll.ID, "make", roll.Difficulty)
	h.makeChoice(plan.ID, "make", []string{})

	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.ResourceAssetID, "make creates the resource and records its id")
	require.False(t, pd.ResourceNamed)

	created, err := h.q.GetAssetByID(ctx, *pd.ResourceAssetID)
	require.NoError(t, err)
	require.Equal(t, lawResourceNameDefault, created.Name, "resource starts with a placeholder")

	namePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/name-resource"

	// A non-preparer cannot name it.
	otherIdx := (preparerIdx + 1) % 3
	code, body := h.post(otherIdx, namePath, map[string]any{"name": "Hijack"})
	require.Equalf(t, http.StatusForbidden, code, "only the preparer may name: %v", body)

	// The preparer names it; the asset is renamed and the flag flips.
	code, body = h.post(preparerIdx, namePath, map[string]any{"name": "The Royal Granary"})
	require.Equalf(t, http.StatusOK, code, "name-asset: %v", body)

	renamed, err := h.q.GetAssetByID(ctx, *pd.ResourceAssetID)
	require.NoError(t, err)
	assert.Equal(t, "The Royal Granary", renamed.Name)
	assert.True(t, pdData(t, h, plan.ID).ResourceNamed, "naming flips ResourceNamed")
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

// TestProposeDecree_JoinCouncil_MintsAndCleansUpDice proves the leverage-to-join
// path produces exactly one die (pre-roll rule 2: exactly one asset is leveraged
// to take a seat) and that an unspent 'decree' die is discarded when the law is
// enacted so it can't leak onto a later roll.
func TestProposeDecree_JoinCouncil_MintsAndCleansUpDice(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Preparer = players[1] (power rank 3). players[0] (rank 2) auto-seats and
	// signs; players[2] (rank 4) is the lower-power "other player" who may join.
	plan := pdPrepareAndResolve(t, h, 1)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	require.Equal(t, h.tg.Players[0].ID, *pd.SignatoryID, "highest-power member signs")

	joinerIdx := 2
	joinerID := h.tg.Players[joinerIdx].ID
	a1 := h.seedPeer(joinerIdx, "Guild ledger")
	a2 := h.seedPeer(joinerIdx, "Harbor writ")

	joinPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/join-council"

	// Only one asset may be leveraged at the council stage — two is rejected and
	// mints nothing.
	code, body := h.post(joinerIdx, joinPath, map[string]any{"asset_ids": []int64{a1, a2}})
	require.Equalf(t, http.StatusBadRequest, code, "two-asset join is rejected: %v", body)
	count, err := h.q.CountUnspentBankedDiceByPlayer(ctx, dbgen.CountUnspentBankedDiceByPlayerParams{
		GameID: h.tg.Game.ID, PlayerID: joinerID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), count, "a rejected join mints no die")

	// Joining with one asset mints exactly one 'decree' banked die.
	code, body = h.post(joinerIdx, joinPath, map[string]any{"asset_ids": []int64{a1}})
	require.Equalf(t, http.StatusOK, code, "join-council: %v", body)

	dice, err := h.q.ListBankedDiceByPlayer(ctx, dbgen.ListBankedDiceByPlayerParams{
		GameID: h.tg.Game.ID, PlayerID: joinerID,
	})
	require.NoError(t, err)
	require.Len(t, dice, 1, "one leveraged asset mints one council die")
	assert.Equal(t, "decree", dice[0].Source, "council dice are tagged 'decree'")

	// The joiner is now a council member; a lower-power join does not change the
	// signatory (players[0] still outranks everyone present).
	pd = pdData(t, h, plan.ID)
	assert.Contains(t, pd.SignatoryPlayerIDs, joinerID, "joiner is seated")
	require.NotNil(t, pd.SignatoryID)
	assert.Equal(t, h.tg.Players[0].ID, *pd.SignatoryID, "signatory unchanged by lower-power join")

	// Every eligible player has now decided (the only joiner joined), so the
	// signatory can close the council. The minted die is left unspent.
	roll := pdCallRoll(t, h, plan.ID, 0)
	h.forceRoll(roll.ID, "make", roll.Difficulty)
	h.makeChoice(plan.ID, "make", []string{})

	// The unspent council die is discarded at enactment.
	unspentLeft, err := h.q.CountUnspentBankedDiceByPlayer(ctx, dbgen.CountUnspentBankedDiceByPlayerParams{
		GameID: h.tg.Game.ID, PlayerID: joinerID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), unspentLeft, "unspent council dice are discarded at enactment")
}

// TestProposeDecree_JoinCouncil_RejectsHigherPower proves the eligibility is the
// right way round: the leverage-to-join path is for players ranked BELOW the
// preparer. A higher-power player (already auto-seated) is refused and mints no
// die.
func TestProposeDecree_JoinCouncil_RejectsHigherPower(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Preparer = players[1] (rank 2); players[0] (rank 1) outranks them.
	plan := pdPrepareAndResolve(t, h, 1)

	higherIdx := 0
	asset := h.seedPeer(higherIdx, "Crown seal")
	joinPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/join-council"
	code, body := h.post(higherIdx, joinPath, map[string]any{"asset_ids": []int64{asset}})
	require.Equalf(t, http.StatusForbidden, code, "higher-power player may not leverage to join: %v", body)

	count, err := h.q.CountUnspentBankedDiceByPlayer(ctx, dbgen.CountUnspentBankedDiceByPlayerParams{
		GameID: h.tg.Game.ID, PlayerID: h.tg.Players[higherIdx].ID,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "a rejected join mints no die")
}

// TestProposeDecree_Make_ResourceOwnedByPreparer proves the made decree's
// resource is owned by the preparer ("what YOU gain"), not the higher-power
// signatory who signs it.
func TestProposeDecree_Make_ResourceOwnedByPreparer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Preparer = players[2] (rank 3); signatory = players[0] (rank 1) ≠ preparer.
	plan := pdPrepareAndResolve(t, h, 2)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	require.NotEqual(t, plan.PreparerID, *pd.SignatoryID, "signatory differs from preparer")

	roll := pdCallRoll(t, h, plan.ID, pdPlayerIdx(t, h, *pd.SignatoryID))
	h.forceRoll(roll.ID, "make", roll.Difficulty)
	h.makeChoice(plan.ID, "make", []string{})

	pd = pdData(t, h, plan.ID)
	require.NotNil(t, pd.ResourceAssetID)
	asset, err := h.q.GetAssetByID(ctx, *pd.ResourceAssetID)
	require.NoError(t, err)
	assert.Equal(t, plan.PreparerID, asset.OwnerID, "the resource belongs to the preparer, not the signatory")
}

// TestProposeDecree_CallRoll_SignatoryGated proves only the signatory (not the
// preparer, when they differ) may call the roll.
func TestProposeDecree_CallRoll_SignatoryGated(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Preparer = players[2]; signatory = players[0].
	plan := pdPrepareAndResolve(t, h, 2)
	preparerIdx := h.preparerIdxFor(plan.ID)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	require.NotEqual(t, plan.PreparerID, *pd.SignatoryID)

	callPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/call-roll"
	code, body := h.post(preparerIdx, callPath, nil)
	require.Equalf(t, http.StatusForbidden, code, "preparer (non-signatory) may not call the roll: %v", body)

	// The signatory can.
	code, body = h.post(pdPlayerIdx(t, h, *pd.SignatoryID), callPath, nil)
	require.Equalf(t, http.StatusOK, code, "signatory calls the roll: %v", body)
}

// TestProposeDecree_SkipAmend_AdvancesChain proves an amender may decline to
// change the law ("amend at will") via skip-amend, advancing the chain without
// editing the text, while the next member still amends.
func TestProposeDecree_SkipAmend_AdvancesChain(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Preparer = players[2]; amenders lowest power first = players[1], players[0].
	plan := pdPrepareAndResolve(t, h, 2)
	pd := pdData(t, h, plan.ID)
	roll := pdCallRoll(t, h, plan.ID, pdPlayerIdx(t, h, *pd.SignatoryID))
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)
	h.makeChoice(plan.ID, "mar", []string{})

	pd = pdData(t, h, plan.ID)
	require.Len(t, pd.AmendmentOrder, 2)
	firstIdx := pdPlayerIdx(t, h, pd.AmendmentOrder[0])
	secondIdx := pdPlayerIdx(t, h, pd.AmendmentOrder[1])

	// First amender skips (leaves the body unchanged).
	skipPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/skip-amend"
	code, body := h.post(firstIdx, skipPath, nil)
	require.Equalf(t, http.StatusOK, code, "skip-amend: %v", body)

	// The skip advanced the queue; the second amender now amends for real.
	amendPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/amend-decree"
	code, body = h.post(secondIdx, amendPath, map[string]any{"text": "Only the second hand wrote"})
	require.Equalf(t, http.StatusOK, code, "second amend: %v", body)

	// Chain complete: signatory addenda, preparer completes.
	addPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/set-addendum"
	code, body = h.post(pdPlayerIdx(t, h, *pd.SignatoryID), addPath, map[string]any{"connector": "", "addendum": ""})
	require.Equalf(t, http.StatusOK, code, "addendum: %v", body)
	h.complete(plan.ID)

	laws, err := h.q.ListLaws(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, "Only the second hand wrote", laws[len(laws)-1].Text,
		"a skip leaves the prior text; only the real amendment shows")

	// The skip is recorded in the action log.
	posts := pdSystemPostBodies(t, h)
	assert.Conditionf(t, func() bool {
		for _, b := range posts {
			if strings.Contains(b, "left the decree's text unchanged") {
				return true
			}
		}
		return false
	}, "expected a skip action-log post; got %v", posts)
}

// TestProposeDecree_Waitees_NamesSignatoryAndAmenders proves the WaitingOn bar
// names the actual actor at each sub-phase — the signatory pre-roll and for the
// addendum, each amender in turn during a mar, and the preparer to complete —
// rather than mis-attributing every wait to the preparer.
func TestProposeDecree_Waitees_NamesSignatoryAndAmenders(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Preparer = players[2] (rank 3); signatory = players[0] (rank 1).
	plan := pdPrepareAndResolve(t, h, 2)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	sigID := *pd.SignatoryID
	preparerID := plan.PreparerID

	// Pre-roll: the council convenes; only the signatory can call the roll.
	h.assertWaitees("pre-roll", model.RowStatePlanResolving, sigID)

	roll := pdCallRoll(t, h, plan.ID, pdPlayerIdx(t, h, sigID))
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)
	h.makeChoice(plan.ID, "mar", []string{})

	// Mar amendment chain: each amender in turn (lowest power first).
	pd = pdData(t, h, plan.ID)
	require.Len(t, pd.AmendmentOrder, 2)
	first, second := pd.AmendmentOrder[0], pd.AmendmentOrder[1]
	h.assertWaitees("first amend", model.RowStatePlanResolving, first)

	amendPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/amend-decree"
	code, body := h.post(pdPlayerIdx(t, h, first), amendPath, map[string]any{"text": "v1"})
	require.Equalf(t, http.StatusOK, code, "first amend: %v", body)
	h.assertWaitees("second amend", model.RowStatePlanResolving, second)

	code, body = h.post(pdPlayerIdx(t, h, second), amendPath, map[string]any{"text": "v2"})
	require.Equalf(t, http.StatusOK, code, "second amend: %v", body)

	// Amendments done → the signatory must place the addendum.
	h.assertWaitees("addendum", model.RowStatePlanResolving, sigID)

	addPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/set-addendum"
	code, body = h.post(pdPlayerIdx(t, h, sigID), addPath, map[string]any{"connector": "", "addendum": ""})
	require.Equalf(t, http.StatusOK, code, "addendum: %v", body)

	// Addendum placed → the preparer completes.
	h.assertWaitees("complete", model.RowStatePlanResolving, preparerID)
}

// TestProposeDecree_DebateGate proves the pre-roll drafting step: until the
// preparer finalizes the decree's text and opens the debate, the table waits on
// the preparer and the roll cannot be called; opening the debate records the
// finalized text, posts the proposed law to the chat, and hands the wait to the
// signatory.
func TestProposeDecree_DebateGate(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Preparer = players[2] (lowest power): everyone else is auto-seated, so the
	// only pre-roll gate is the preparer opening the debate.
	plan := pdPrepareResolveNoDebate(t, h, 2)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	sigID := *pd.SignatoryID
	require.False(t, pd.DebateStarted)

	// Pre-debate: the table waits on the preparer to finalize, not the signatory.
	h.assertWaitees("drafting", model.RowStatePlanResolving, plan.PreparerID)

	// The signatory cannot call the roll before the debate is opened.
	callPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/call-roll"
	code, body := h.post(pdPlayerIdx(t, h, sigID), callPath, nil)
	require.Equalf(t, http.StatusConflict, code, "roll blocked before the debate opens: %v", body)

	// Only the preparer may open the debate.
	startPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/start-debate"
	code, _ = h.post(pdPlayerIdx(t, h, sigID), startPath, map[string]any{"text": "x"})
	require.Equal(t, http.StatusForbidden, code, "only the preparer opens the debate")

	// The preparer finalizes an edited text and opens the debate.
	final := "Finalized: all trade taxes are abolished"
	code, body = h.post(h.preparerIdxFor(plan.ID), startPath, map[string]any{"text": final})
	require.Equalf(t, http.StatusOK, code, "start-debate: %v", body)

	pd = pdData(t, h, plan.ID)
	assert.True(t, pd.DebateStarted)
	assert.Equal(t, final, pd.LawText, "the finalized text is recorded")

	// The proposed law is posted to the chat to seed discussion.
	posts := pdSystemPostBodies(t, h)
	assert.Conditionf(t, func() bool {
		for _, b := range posts {
			if strings.Contains(b, "opened the council debate") && strings.Contains(b, final) {
				return true
			}
		}
		return false
	}, "expected a debate-opened post containing the proposed law; got %v", posts)

	// Debate open, no eligible joiners → the wait passes to the signatory, who can
	// now call the roll. The enacted law carries the finalized (edited) text.
	h.assertWaitees("debate open", model.RowStatePlanResolving, sigID)
	roll := pdCallRoll(t, h, plan.ID, pdPlayerIdx(t, h, sigID))
	h.forceRoll(roll.ID, "make", roll.Difficulty)
	h.makeChoice(plan.ID, "make", []string{})
	laws, err := h.q.ListLaws(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, final, laws[len(laws)-1].Text, "the enacted law uses the finalized text")
}

// TestProposeDecree_DraftingWaitsOnPreparerAndJoiners proves the Waiting-On bar
// names every player who can act on a pre-roll gate in parallel: while the
// preparer is still drafting, the eligible joiners can already join/decline, so
// all of them appear together (not just the preparer).
func TestProposeDecree_DraftingWaitsOnPreparerAndJoiners(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Preparer = players[0] (rank 2); players[1] (rank 3) and players[2] (rank 4)
	// are the eligible joiners. Debate not yet opened.
	plan := pdPrepareResolveNoDebate(t, h, 0)
	preparerID := plan.PreparerID
	p1, p2 := h.tg.Players[1].ID, h.tg.Players[2].ID

	// Drafting: the preparer AND both undecided joiners are all actionable.
	h.assertWaitees("drafting", model.RowStatePlanResolving, preparerID, p1, p2)

	// A joiner decides while the preparer is still drafting → they drop off, the
	// preparer and the remaining joiner stay.
	declinePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/decline-council"
	code, body := h.post(1, declinePath, nil)
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	h.assertWaitees("one decided", model.RowStatePlanResolving, preparerID, p2)

	// The preparer opens the debate → only the still-undecided joiner remains.
	pdStartDebate(t, h, plan.ID)
	h.assertWaitees("debate open", model.RowStatePlanResolving, p2)
}

// TestProposeDecree_CouncilDecisionGate proves the pre-roll council gate: while
// any eligible (lower-power) player still owes a join/decline decision the bar
// names those players and the signatory cannot call the roll; once everyone has
// joined or declined the wait passes to the signatory and the roll can be called.
func TestProposeDecree_CouncilDecisionGate(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Preparer = players[0] (rank 2); no higher-power real player (rank 1 is a
	// dummy), so the preparer is also the signatory. players[1] (rank 3) and
	// players[2] (rank 4) are the eligible joiners who must decide.
	plan := pdPrepareAndResolve(t, h, 0)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	sigID := *pd.SignatoryID
	require.Equal(t, plan.PreparerID, sigID, "preparer signs when no one outranks them")

	p1, p2 := h.tg.Players[1].ID, h.tg.Players[2].ID
	callPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/call-roll"
	declinePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/decline-council"
	joinPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/join-council"

	// Pre-roll: the bar waits on both undecided eligible players, not the signatory.
	h.assertWaitees("undecided", model.RowStatePlanResolving, p1, p2)

	// The signatory cannot call the roll until everyone has decided.
	code, body := h.post(pdPlayerIdx(t, h, sigID), callPath, nil)
	require.Equalf(t, http.StatusConflict, code, "roll blocked while players are undecided: %v", body)

	// players[1] declines → only players[2] remains.
	code, body = h.post(1, declinePath, nil)
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	h.assertWaitees("one left", model.RowStatePlanResolving, p2)

	// A decided player cannot decide again, by either route.
	code, _ = h.post(1, declinePath, nil)
	require.Equal(t, http.StatusConflict, code, "cannot decline twice")
	asset := h.seedPeer(1, "Second thoughts")
	code, _ = h.post(1, joinPath, map[string]any{"asset_ids": []int64{asset}})
	require.Equal(t, http.StatusConflict, code, "a declined player cannot then join")

	// players[2] joins (one asset) → everyone has decided; the wait passes to the
	// signatory and the roll can be called.
	a := h.seedPeer(2, "Harbor writ")
	code, body = h.post(2, joinPath, map[string]any{"asset_ids": []int64{a}})
	require.Equalf(t, http.StatusOK, code, "join: %v", body)
	h.assertWaitees("all decided", model.RowStatePlanResolving, sigID)

	code, body = h.post(pdPlayerIdx(t, h, sigID), callPath, nil)
	require.Equalf(t, http.StatusOK, code, "roll callable once all decided: %v", body)
}

// TestProposeDecree_LowPowerMonarch_SignsAndSeated proves ADR-007 Phase C: a
// monarch ranked BELOW the preparer (and below everyone else) is still
// auto-seated and still signs — power rank no longer fakes the monarch. The
// throne is established by stamping a monarch title + tripping the gate.
func TestProposeDecree_LowPowerMonarch_SignsAndSeated(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Establish the throne with the monarch on players[2] (power rank 3 — the
	// lowest of the three). Preparer will be players[0] (rank 1).
	establishThrone(t, h.pool, h.tg.Game.ID)
	seedTitledAsset(t, h.q, h.pool, h.tg.Game.ID, h.tg.Players[2].ID, "Fallen Crown", game.TitleMonarch)

	plan := pdPrepareAndResolve(t, h, 0)
	pd := pdData(t, h, plan.ID)

	require.NotNil(t, pd.SignatoryID)
	assert.Equal(t, h.tg.Players[2].ID, *pd.SignatoryID,
		"the monarch signs even at the worst power rank")
	assert.Contains(t, pd.SignatoryPlayerIDs, h.tg.Players[2].ID,
		"the monarch is auto-seated despite ranking below the preparer")
	assert.Contains(t, pd.SignatoryPlayerIDs, plan.PreparerID, "the preparer is seated")
}

// TestProposeDecree_Throneless_FallsBackToPower proves the throneless table is
// unchanged from the post-audit behavior: a monarch title marginalia exists but
// the gate is NOT tripped, so currentMonarch returns "no monarch" and signatory
// selection falls back to pure power rank.
func TestProposeDecree_Throneless_FallsBackToPower(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Stamp a monarch title on players[1] but DO NOT establish the throne — a
	// lone claim with no gate is a powerless pretender.
	seedTitledAsset(t, h.q, h.pool, h.tg.Game.ID, h.tg.Players[1].ID, "Pretender", game.TitleMonarch)

	// Preparer = players[2] (rank 3); the highest-power member is players[0].
	plan := pdPrepareAndResolve(t, h, 2)
	pd := pdData(t, h, plan.ID)

	require.NotNil(t, pd.SignatoryID)
	assert.Equal(t, h.tg.Players[0].ID, *pd.SignatoryID,
		"with no throne the highest-power member signs, ignoring the pretender")
}

// TestProposeDecree_MonarchKeepsSignatureWhenHigherPowerJoins proves the
// signatory is fixed at seating: a reigning low-power monarch keeps the pen
// even when a member ranked ABOVE them (but still below the preparer) leverages
// in. join-council adds the member and their dice but never moves the signature.
func TestProposeDecree_MonarchKeepsSignatureWhenHigherPowerJoins(t *testing.T) {
	h := newPlanLifecycle(t, 4)

	// Monarch = players[3] (rank 4, the lowest). Preparer = players[1] (rank 2).
	// Joiner = players[2] (rank 3): below the preparer (eligible to leverage in)
	// but higher power than the monarch.
	establishThrone(t, h.pool, h.tg.Game.ID)
	seedTitledAsset(t, h.q, h.pool, h.tg.Game.ID, h.tg.Players[3].ID, "Diminished Crown", game.TitleMonarch)

	plan := pdPrepareAndResolve(t, h, 1)
	pd := pdData(t, h, plan.ID)
	require.NotNil(t, pd.SignatoryID)
	require.Equal(t, h.tg.Players[3].ID, *pd.SignatoryID, "the monarch signs pre-join")

	joinerIdx := 2
	asset := h.seedPeer(joinerIdx, "Guild charter")
	joinPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/join-council"
	code, body := h.post(joinerIdx, joinPath, map[string]any{"asset_ids": []int64{asset}})
	require.Equalf(t, http.StatusOK, code, "join-council: %v", body)

	pd = pdData(t, h, plan.ID)
	assert.Contains(t, pd.SignatoryPlayerIDs, h.tg.Players[joinerIdx].ID, "joiner is seated")
	require.NotNil(t, pd.SignatoryID)
	assert.Equal(t, h.tg.Players[3].ID, *pd.SignatoryID,
		"the monarch keeps the signature despite a higher-power joiner")
}
