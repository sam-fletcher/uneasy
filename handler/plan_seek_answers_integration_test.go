//go:build integration

// handler/plan_seek_answers_integration_test.go — mechanical-effect coverage
// for Seek Answers. These guard the rules-correct behaviour added after the
// audit found the handler ignored the mar self-flaw penalty and bypassed the
// canonical break helper:
//
//   - make "break_resource": tears one marginalia, auto-destroys on the last,
//     and rejects flawing the same resource twice ("overlooked until now").
//   - mar penalty: the preparer must describe a flaw in (difficulty − result)
//     of their *own* resources; completion is blocked until satisfied.

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
	"uneasy/game"
	"uneasy/model"
)

// saSeedResource creates a resource owned by players[ownerIdx] with `margs`
// intact marginalia and returns the asset id plus the marginalia ids in order.
func saSeedResource(t *testing.T, h *planLifecycle, ownerIdx int, name string, margs int) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	a, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    h.tg.Game.ID,
		OwnerID:   h.tg.Players[ownerIdx].ID,
		CreatorID: h.tg.Players[ownerIdx].ID,
		AssetType: model.AssetResource,
		Name:      name,
	})
	require.NoError(t, err)
	ids := make([]int64, margs)
	for i := range margs {
		m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: a.ID, Position: int16(i + 1), Text: "note",
		})
		require.NoError(t, err)
		ids[i] = m.ID
	}
	return a.ID, ids
}

// saCastRoll drives POST /api/plans/{planId}/seek-cast-roll as the preparer: it posts
// the pre-roll narration, closes the pre-roll step, and creates the dice roll.
// Asserts 200 and returns the roll.
func saCastRoll(t *testing.T, h *planLifecycle, planID int64) *dbgen.DiceRoll {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/seek-cast-roll"
	code, body := h.post(h.preparerIdxFor(planID), path, map[string]any{
		"narration": "I cross-referenced the ledgers, and learned the seal was forged.",
	})
	require.Equalf(t, http.StatusOK, code, "cast-roll: %v", body)
	rollBlob, _ := json.Marshal(body["roll"])
	var roll dbgen.DiceRoll
	require.NoError(t, json.Unmarshal(rollBlob, &roll))
	return &roll
}

// saPrepareToRoll drives a Seek Answers plan to a forced roll and returns the
// plan and the preparer's index. The plan is left pre make-choice.
func saPrepareToRoll(t *testing.T, h *planLifecycle, outcome string, resultDelta int16) (dbgen.Plan, int, *dbgen.DiceRoll) {
	t.Helper()
	notes := "researching the archives"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSeekAnswers,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)

	// Pin the preparer's knowledge rank to 3 so difficulty (= knowledge rank)
	// is deterministic regardless of seat-order seeding. Difficulty is computed
	// at resolve time, so swap the rank slots before resolving.
	saPinKnowledgeRank(t, h, plan.PreparerID, 3)

	h.jumpToRow(*plan.RowNumber)
	// OnResolve opens the pre-roll narration step with no roll; the preparer
	// casts the dice via cast-roll after restating their methods.
	require.Nil(t, h.resolve(plan.ID), "Seek Answers opens the pre-roll step with no roll")
	roll := saCastRoll(t, h, plan.ID)
	require.Equal(t, int16(3), roll.Difficulty, "pinned knowledge rank should set difficulty")

	var result int16
	if outcome == "make" {
		result = roll.Difficulty + resultDelta
	} else {
		result = roll.Difficulty - resultDelta
	}
	h.forceRoll(roll.ID, outcome, result)
	return plan, h.preparerIdxFor(plan.ID), roll
}

// TestSeekAnswers_Make_BreakResource_AutoDestroysOnLast proves the make-side
// break tears one marginalia and destroys the resource when it was the last,
// using the canonical break helper.
func TestSeekAnswers_Make_BreakResource_AutoDestroysOnLast(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	resourceID, margIDs := saSeedResource(t, h, otherIdx, "fragile ledger", 1)

	h.makeChoice(plan.ID, "make", []string{"break_resource"})

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": resourceID, "marginalia_id": margIDs[0],
	})
	require.Equalf(t, http.StatusOK, code, "break-resource: %v", body)
	assert.Equal(t, true, body["destroyed"], "tearing the last marginalia destroys the asset")

	destroyed, err := h.q.GetAssetByID(ctx, resourceID)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed, "resource should be destroyed")

	h.complete(plan.ID)
}

// TestSeekAnswers_Make_BreakResource_RejectsDoubleFlaw proves a resource can
// only be flawed once per resolution ("overlooked until now").
func TestSeekAnswers_Make_BreakResource_RejectsDoubleFlaw(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 3)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	resourceID, margIDs := saSeedResource(t, h, otherIdx, "two-note tome", 2)

	h.makeChoice(plan.ID, "make", []string{"break_resource", "break_resource"})

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": resourceID, "marginalia_id": margIDs[0],
	})
	require.Equalf(t, http.StatusOK, code, "first flaw: %v", body)

	code, body = h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": resourceID, "marginalia_id": margIDs[1],
	})
	assert.Equalf(t, http.StatusConflict, code, "second flaw on the same resource should 409: %v", body)
}

// TestSeekAnswers_Make_BreakResource_CapsAtPickedCount proves the make-list break
// sub-flow is server-capped at the picked count: a stale client re-prompted
// after a refresh can't flaw an extra resource beyond what was chosen.
func TestSeekAnswers_Make_BreakResource_CapsAtPickedCount(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 3)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	resA, margsA := saSeedResource(t, h, otherIdx, "tome A", 2)
	resB, margsB := saSeedResource(t, h, otherIdx, "tome B", 2)

	// Only one break picked.
	h.makeChoice(plan.ID, "make", []string{"break_resource"})

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": resA, "marginalia_id": margsA[0],
	})
	require.Equalf(t, http.StatusOK, code, "first (only) break: %v", body)

	// A second, distinct break exceeds the picked count and is rejected.
	code, body = h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": resB, "marginalia_id": margsB[0],
	})
	require.Equalf(t, http.StatusConflict, code, "break beyond the picked count must be rejected: %v", body)
	intact, err := h.q.GetMarginaliaByID(ctx, margsB[0])
	require.NoError(t, err)
	assert.False(t, intact.IsTorn, "the rejected break must not tear a marginalia")
}

// TestSeekAnswers_Make_RevealSecret_CapsAtPickedCount proves the reveal-secret
// sub-flow is server-capped at the picked count, so a re-prompted client can't
// reveal more assets' secrets than were chosen.
func TestSeekAnswers_Make_RevealSecret_CapsAtPickedCount(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 3)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	assetA, _ := saSeedResource(t, h, otherIdx, "ledger A", 1)
	assetB, _ := saSeedResource(t, h, otherIdx, "ledger B", 1)

	h.makeChoice(plan.ID, "make", []string{"reveal_secret"})

	revealPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/reveal-secret"
	code, body := h.post(preparerIdx, revealPath, map[string]any{"asset_id": assetA})
	require.Equalf(t, http.StatusOK, code, "first (only) reveal: %v", body)

	// The reveal is recorded as Tier-1 read-only state and now emits an action-log
	// entry (it previously logged nothing, unlike every other make-list step).
	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers)
	assert.Equal(t, []int64{assetA}, rd.SeekAnswers.RevealedAssetIDs, "the revealed asset is recorded")
	assert.Contains(t, saAllLogBodies(t, h), "learned the secrets of", "reveal must emit an action-log entry")

	code, body = h.post(preparerIdx, revealPath, map[string]any{"asset_id": assetB})
	require.Equalf(t, http.StatusConflict, code, "reveal beyond the picked count must be rejected: %v", body)
}

// TestSeekAnswers_CanComplete_BlockedUntilMakeListDone proves completion is
// server-gated on the committed make-list picks: a declare_truth pick must be
// performed before the plan can resolve, so the client's sub-flow gate is no
// longer the only enforcement.
func TestSeekAnswers_CanComplete_BlockedUntilMakeListDone(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	h.makeChoice(plan.ID, "make", []string{"declare_truth"})

	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"
	code, body := h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code,
		"complete must be blocked with an unconsumed declare_truth pick: %v", body)

	dtPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/declare-truth"
	code, body = h.post(preparerIdx, dtPath, map[string]any{"text": "The river runs north."})
	require.Equalf(t, http.StatusOK, code, "declare-truth: %v", body)

	h.complete(plan.ID)
}

// TestSeekAnswers_ForfeitStep_DischargesWhenNoTarget proves a make-list
// break_resource pick with no breakable resource in the game can be forfeited as
// a no-op, which discharges the remaining pick so the plan can complete. This is
// the escape hatch for an over-picked or concurrently-depleted depletable step,
// which would otherwise wedge the resolve loop (the Complete button never shows).
func TestSeekAnswers_ForfeitStep_DischargesWhenNoTarget(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	// No resource exists, so break_resource has no valid target.
	h.makeChoice(plan.ID, "make", []string{"break_resource"})

	forfeitPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/seek-forfeit-step"
	code, body := h.post(preparerIdx, forfeitPath, map[string]any{"step": "break_resource"})
	require.Equalf(t, http.StatusOK, code, "forfeit with no target should succeed: %v", body)
	assert.EqualValues(t, 1, body["forfeited"], "the single remaining pick is forfeited")

	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers)
	assert.Equal(t, int16(1), rd.SeekAnswers.BreakResourceDone, "the forfeited pick is recorded as done")
	assert.Empty(t, rd.SeekAnswers.FlawedResourceIDs, "forfeit is a no-op — nothing is flawed")
	assert.Contains(t, saAllLogBodies(t, h), "forfeited", "forfeit emits an action-log entry")

	h.complete(plan.ID)
}

// TestSeekAnswers_ForfeitStep_RejectedWhenTargetExists proves the server refuses
// to forfeit a step the preparer could still perform — a breakable resource is
// present, so the pick must be spent, not skipped.
func TestSeekAnswers_ForfeitStep_RejectedWhenTargetExists(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	saSeedResource(t, h, otherIdx, "breakable ledger", 1)

	h.makeChoice(plan.ID, "make", []string{"break_resource"})

	forfeitPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/seek-forfeit-step"
	code, body := h.post(preparerIdx, forfeitPath, map[string]any{"step": "break_resource"})
	require.Equalf(t, http.StatusConflict, code, "forfeit must be rejected while a target remains: %v", body)

	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers)
	assert.Equal(t, int16(0), rd.SeekAnswers.BreakResourceDone, "rejected forfeit changes nothing")
}

// TestSeekAnswers_Mar_ForeignBreak_CapsAtPickedCount proves that on a mar, a
// break against another player's resource is a make-list pick and is bounded by
// the result (the rules grant result-many make-list options on a mar too) — not
// an unlimited side effect.
func TestSeekAnswers_Mar_ForeignBreak_CapsAtPickedCount(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "mar", 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	resA, margsA := saSeedResource(t, h, otherIdx, "rival ledger A", 1)
	resB, margsB := saSeedResource(t, h, otherIdx, "rival ledger B", 1)

	// One make-list break pick on the mar.
	h.makeChoice(plan.ID, "mar", []string{"break_resource"})

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": resA, "marginalia_id": margsA[0],
	})
	require.Equalf(t, http.StatusOK, code, "first foreign break (within the pick): %v", body)

	// A second foreign break exceeds the single make-list pick and is rejected.
	code, body = h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": resB, "marginalia_id": margsB[0],
	})
	require.Equalf(t, http.StatusConflict, code, "foreign break beyond the picked count must be rejected: %v", body)
	intact, err := h.q.GetMarginaliaByID(ctx, margsB[0])
	require.NoError(t, err)
	assert.False(t, intact.IsTorn, "the rejected break must not tear a marginalia")
}

// TestSeekAnswers_Mar_SelfFlawPenalty_BlocksUntilApplied proves a marred plan
// requires (difficulty − result) self-flaws on the preparer's own resources
// before it can complete.
func TestSeekAnswers_Mar_SelfFlawPenalty_BlocksUntilApplied(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	// Consistent mar with penalty = 2 (difficulty − result). Difficulty must be
	// ≥ 2 for the delta to land on a non-negative result.
	plan, preparerIdx, roll := saPrepareToRoll(t, h, "mar", 2)
	require.GreaterOrEqual(t, roll.Difficulty, int16(2), "need difficulty ≥ 2 for this scenario")

	// Two of the preparer's own resources, plus one belonging to another player
	// (which must NOT count toward the penalty).
	ownA, ownAMargs := saSeedResource(t, h, preparerIdx, "preparer ledger A", 1)
	ownB, ownBMargs := saSeedResource(t, h, preparerIdx, "preparer ledger B", 1)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	foreign, foreignMargs := saSeedResource(t, h, otherIdx, "rival ledger", 1)

	// One make-list break pick (the rules grant result-many make-list options on
	// a mar) so the foreign flaw below is a legitimate, bounded make-list break;
	// plus the self-flaw penalty.
	h.makeChoice(plan.ID, "mar", []string{"break_resource"})

	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers)
	assert.Equal(t, int16(2), rd.SeekAnswers.MarSelfFlawsRequired, "penalty should be difficulty − result = 2")

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"

	// Blocked before any penalty applied.
	code, body := h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should be blocked pre-penalty: %v", body)

	// Flawing a foreign resource does not discharge the self-flaw penalty.
	code, body = h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": foreign, "marginalia_id": foreignMargs[0],
	})
	require.Equalf(t, http.StatusOK, code, "foreign flaw: %v", body)
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "foreign flaw must not count toward the penalty: %v", body)

	// First own flaw — still one short.
	code, body = h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": ownA, "marginalia_id": ownAMargs[0],
	})
	require.Equalf(t, http.StatusOK, code, "own flaw A: %v", body)
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "one self-flaw short should still block: %v", body)

	// Second own flaw — penalty satisfied.
	code, body = h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": ownB, "marginalia_id": ownBMargs[0],
	})
	require.Equalf(t, http.StatusOK, code, "own flaw B: %v", body)

	h.complete(plan.ID)

	final := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, final.SeekAnswers)
	assert.Equal(t, int16(2), final.SeekAnswers.MarSelfFlawsApplied, "both self-flaws recorded")

	// Sanity: own resources lost their marginalia and were destroyed.
	for _, id := range []int64{ownA, ownB} {
		a, err := h.q.GetAssetByID(ctx, id)
		require.NoError(t, err)
		assert.True(t, a.IsDestroyed, "self-flawed single-marginalia resource should be destroyed")
	}
}

// TestSeekAnswers_Mar_PenaltyCappedByResources proves the penalty is capped at
// the number of the preparer's eligible own resources.
func TestSeekAnswers_Mar_PenaltyCappedByResources(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Penalty would be 3, but the preparer owns only one eligible resource.
	plan, preparerIdx, roll := saPrepareToRoll(t, h, "mar", 3)
	require.GreaterOrEqual(t, roll.Difficulty, int16(3), "need difficulty ≥ 3 for this scenario")

	only, onlyMargs := saSeedResource(t, h, preparerIdx, "sole ledger", 1)

	h.makeChoice(plan.ID, "mar", []string{})

	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers)
	assert.Equal(t, int16(1), rd.SeekAnswers.MarSelfFlawsRequired, "penalty capped at the single owned resource")

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": only, "marginalia_id": onlyMargs[0],
	})
	require.Equalf(t, http.StatusOK, code, "own flaw: %v", body)

	h.complete(plan.ID)
}

// TestSeekAnswers_PreRoll_OpensWithoutRoll proves OnResolve opens the pre-roll
// narration step with no roll, and that cast-roll posts the preparer's narration
// as their own scene post, flips pre_roll_done, and creates the dice roll.
func TestSeekAnswers_PreRoll_OpensWithoutRoll(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	notes := "researching the archives"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSeekAnswers,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	preparerIdx := h.preparerIdxFor(plan.ID)

	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "Seek Answers opens the pre-roll step with no roll")

	// No roll exists yet, and pre_roll_done is false.
	_, rollErr := h.q.GetDiceRollByPlanID(context.Background(), &plan.ID)
	require.Error(t, rollErr, "no roll should exist before cast-roll")
	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers)
	assert.False(t, rd.SeekAnswers.PreRollDone, "pre-roll should still be open")

	const narration = "I retraced my sources, and learned the envoy never arrived."
	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/seek-cast-roll"
	code, body := h.post(preparerIdx, path, map[string]any{"narration": narration})
	require.Equalf(t, http.StatusOK, code, "cast-roll: %v", body)
	require.NotNil(t, body["roll"], "cast-roll should return the created roll")

	// pre_roll_done flipped, the roll now exists, and the narration is logged as
	// the preparer's own post.
	rd = loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	assert.True(t, rd.SeekAnswers.PreRollDone, "pre-roll should be closed after casting")
	_, rollErr = h.q.GetDiceRollByPlanID(context.Background(), &plan.ID)
	require.NoError(t, rollErr, "a roll should exist after cast-roll")

	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	logged := false
	for _, p := range posts {
		if p.Body == narration && p.AuthorID != nil && *p.AuthorID == plan.PreparerID {
			logged = true
		}
	}
	assert.True(t, logged, "the preparer's pre-roll narration should be logged as their post")
}

// TestSeekAnswers_PreRoll_Guards proves cast-roll requires the narration, is
// preparer-only, and rejects a second cast.
func TestSeekAnswers_PreRoll_Guards(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	notes := "researching the archives"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSeekAnswers,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	preparerIdx := h.preparerIdxFor(plan.ID)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)

	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/seek-cast-roll"

	// Empty narration is rejected.
	code, body := h.post(preparerIdx, path, map[string]any{"narration": "   "})
	require.Equalf(t, http.StatusBadRequest, code, "empty narration must 400: %v", body)

	// Only the preparer may cast.
	code, body = h.post(otherIdx, path, map[string]any{"narration": "not mine to cast"})
	require.Equalf(t, http.StatusForbidden, code, "non-preparer cast must 403: %v", body)

	// First real cast succeeds.
	code, body = h.post(preparerIdx, path, map[string]any{"narration": "methods restated; I learned X."})
	require.Equalf(t, http.StatusOK, code, "first cast: %v", body)

	// A second cast is rejected.
	code, body = h.post(preparerIdx, path, map[string]any{"narration": "again"})
	require.Equalf(t, http.StatusConflict, code, "second cast must 409: %v", body)
}

// mustGetPlan fetches a plan by id, failing the test on error.
func mustGetPlan(t *testing.T, h *planLifecycle, planID int64) dbgen.Plan {
	t.Helper()
	p, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(t, err)
	return p
}

// saPinKnowledgeRank deterministically sets a player's knowledge rank by
// swapping rank slots. The ranking table keys each (game, category, rank) slot
// to one player, so we move the player into the target slot and relocate the
// previous occupant into the player's old slot.
func saPinKnowledgeRank(t *testing.T, h *planLifecycle, playerID int64, target int16) {
	t.Helper()
	ctx := context.Background()
	ranks, err := h.q.ListRankingsByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)

	var curRank int16
	var occupant *int64
	for _, r := range ranks {
		if r.Category != model.CategoryKnowledge || r.PlayerID == nil {
			continue
		}
		if *r.PlayerID == playerID {
			curRank = r.Rank
		}
		if r.Rank == target {
			id := *r.PlayerID
			occupant = &id
		}
	}
	if curRank == target {
		return
	}
	pid := playerID
	require.NoError(t, h.q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: h.tg.Game.ID, PlayerID: &pid, Category: model.CategoryKnowledge, Rank: target,
	}))
	if occupant != nil && curRank != 0 {
		require.NoError(t, h.q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: h.tg.Game.ID, PlayerID: occupant, Category: model.CategoryKnowledge, Rank: curRank,
		}))
	}
}

// saAllLogBodies joins every action-log post body for substring assertions.
func saAllLogBodies(t *testing.T, h *planLifecycle) string {
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

// TestSeekAnswers_DeclareTruth_LogsAndCaps proves the declare_truth sub-flow logs
// the truth and is capped at the picked count.
func TestSeekAnswers_DeclareTruth_LogsAndCaps(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 1)
	h.makeChoice(plan.ID, "make", []string{"declare_truth"})

	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/declare-truth"
	code, body := h.post(preparerIdx, path, map[string]any{"text": "The crown is bankrupt."})
	require.Equalf(t, http.StatusOK, code, "declare: %v", body)
	assert.Contains(t, saAllLogBodies(t, h), "The crown is bankrupt.")

	// The truth is recorded as Tier-1 read-only state.
	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers)
	assert.Equal(t, []string{"The crown is bankrupt."}, rd.SeekAnswers.DeclaredTruths, "the declared truth is recorded")

	// Capped at the single pick.
	code, body = h.post(preparerIdx, path, map[string]any{"text": "another truth"})
	require.Equalf(t, http.StatusConflict, code, "second declare must be capped: %v", body)
}

// TestSeekAnswers_AskQuestion_AnswerFlow proves the ask→answer flow: the target
// (who doesn't outrank) can't veto, the plan blocks while pending, only the
// target may answer, and the Q&A is logged.
func TestSeekAnswers_AskQuestion_AnswerFlow(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 1)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	targetID := h.tg.Players[otherIdx].ID
	// Put the preparer at the top of the knowledge track so no target outranks
	// them — the difficulty was already locked at resolve, so this is safe. Rank
	// 2 is the top real slot in a 3-player game (a dummy sits at rank 1).
	saPinKnowledgeRank(t, h, plan.PreparerID, 2)

	h.makeChoice(plan.ID, "make", []string{"ask_question"})

	askPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/ask-question"
	answerPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/answer-question"
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"

	code, body := h.post(preparerIdx, askPath, map[string]any{"target_id": targetID, "question": "Where were you?"})
	require.Equalf(t, http.StatusOK, code, "ask: %v", body)
	assert.Equal(t, false, body["vetoable"], "a lower-ranked target cannot veto")

	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers.PendingQuestion)

	// The WaitingOn bar must name the target: the row blocks on them —
	// attribution pinned through the real ask-question flow.
	h.assertWaitees("question asked → target answers",
		model.RowStateAwaitQuestionAnswer, targetID)

	// Completion blocks while a question is pending.
	code, _ = h.post(preparerIdx, completePath, nil)
	require.Equal(t, http.StatusConflict, code)

	// Only the target may answer.
	code, _ = h.post(preparerIdx, answerPath, map[string]any{"answer": "x"})
	require.Equal(t, http.StatusForbidden, code)

	code, body = h.post(otherIdx, answerPath, map[string]any{"answer": "At the library."})
	require.Equalf(t, http.StatusOK, code, "answer: %v", body)

	final := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	assert.Nil(t, final.SeekAnswers.PendingQuestion)
	assert.Equal(t, int16(1), final.SeekAnswers.AskQuestionDone)

	// The resolved Q&A is recorded as Tier-1 read-only state.
	require.Len(t, final.SeekAnswers.AnsweredQuestions, 1)
	assert.Equal(t, targetID, final.SeekAnswers.AnsweredQuestions[0].TargetID)
	assert.Equal(t, "Where were you?", final.SeekAnswers.AnsweredQuestions[0].Question)
	assert.Equal(t, "At the library.", final.SeekAnswers.AnsweredQuestions[0].Answer)

	// The block clears once answered — the row no longer waits on the target.
	rs, err := ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
	require.NoError(t, err)
	assert.NotEqual(t, model.RowStateAwaitQuestionAnswer, rs.Kind)

	logs := saAllLogBodies(t, h)
	assert.Contains(t, logs, "Where were you?")
	assert.Contains(t, logs, "At the library.")
}

// TestSeekAnswers_AskQuestion_VetoThenReask proves a higher-knowledge target can
// veto the first question once; the re-ask can't be vetoed and is answered.
func TestSeekAnswers_AskQuestion_VetoThenReask(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 1)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	targetID := h.tg.Players[otherIdx].ID
	// Rank 2 is the top real slot in a 3-player game (dummy at rank 1), so the
	// target now outranks the preparer (who sits at rank 3 or 4) on knowledge.
	saPinKnowledgeRank(t, h, targetID, 2)

	h.makeChoice(plan.ID, "make", []string{"ask_question"})

	askPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/ask-question"
	vetoPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/veto-question"
	answerPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/answer-question"

	code, body := h.post(preparerIdx, askPath, map[string]any{"target_id": targetID, "question": "first?"})
	require.Equalf(t, http.StatusOK, code, "ask: %v", body)
	assert.Equal(t, true, body["vetoable"], "a higher-ranked target can veto")

	// Only the target may veto.
	code, _ = h.post(preparerIdx, vetoPath, nil)
	require.Equal(t, http.StatusForbidden, code)

	code, body = h.post(otherIdx, vetoPath, nil)
	require.Equalf(t, http.StatusOK, code, "veto: %v", body)
	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	assert.Nil(t, rd.SeekAnswers.PendingQuestion)
	assert.True(t, rd.SeekAnswers.CurrentAskVetoed)
	assert.Equal(t, int16(0), rd.SeekAnswers.AskQuestionDone, "veto doesn't complete the pick")

	// Re-ask — no longer vetoable.
	code, body = h.post(preparerIdx, askPath, map[string]any{"target_id": targetID, "question": "second?"})
	require.Equalf(t, http.StatusOK, code, "re-ask: %v", body)
	assert.Equal(t, false, body["vetoable"], "the re-asked question can't be vetoed")

	// The target can't veto the replacement.
	code, _ = h.post(otherIdx, vetoPath, nil)
	require.Equal(t, http.StatusConflict, code)

	// Answer completes the pick.
	code, body = h.post(otherIdx, answerPath, map[string]any{"answer": "here."})
	require.Equalf(t, http.StatusOK, code, "answer: %v", body)
	final := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	assert.Equal(t, int16(1), final.SeekAnswers.AskQuestionDone)
	assert.False(t, final.SeekAnswers.CurrentAskVetoed)
}

// TestSeekAnswers_PerformStepsWinnerDrivesMakeList proves a Make Demands
// perform_steps win transfers the preparer's post-roll make-list resolution
// (make-choice, reveal-secret, ask-question) to the winner and locks the
// preparer out — while the question's TARGET, a third party, still answers as
// themselves (perform_steps never reaches a third party's role).
func TestSeekAnswers_PerformStepsWinnerDrivesMakeList(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2) // result 5 → ample picks
	demanderIdx := (preparerIdx + 1) % 3
	targetIdx := (preparerIdx + 2) % 3

	// A secret-bearing asset owned by the reveal/question target.
	holder := h.seedPeer(targetIdx, "secret holder")
	secretID := h.seedSecret(holder, targetIdx, "the vault code is 1789")

	// A resolved, made demand hands perform_steps to the demander.
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[demanderIdx].ID,
	})

	mcPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"
	picks := map[string]any{"result": "make", "choices": []string{"reveal_secret", "ask_question"}}

	// The preparer is locked out of committing the make-list picks; the winner does.
	code, body := h.post(preparerIdx, mcPath, picks)
	require.Equalf(t, http.StatusForbidden, code, "preparer locked out of make-choice: %v", body)
	code, body = h.post(demanderIdx, mcPath, picks)
	require.Equalf(t, http.StatusOK, code, "winner drives make-choice: %v", body)

	// reveal-secret: preparer locked out; the winner drives it and, as the actor,
	// learns the secret.
	revealPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/reveal-secret"
	code, body = h.post(preparerIdx, revealPath, map[string]any{"asset_id": holder})
	require.Equalf(t, http.StatusForbidden, code, "preparer locked out of reveal-secret: %v", body)
	code, body = h.post(demanderIdx, revealPath, map[string]any{"asset_id": holder})
	require.Equalf(t, http.StatusOK, code, "winner drives reveal-secret: %v", body)
	h.assertSecretVisible("winner learns the revealed secret", holder, secretID, demanderIdx)

	// ask-question: preparer locked out; the winner asks the target.
	askPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/ask-question"
	ask := map[string]any{"target_id": h.tg.Players[targetIdx].ID, "question": "where were you?"}
	code, body = h.post(preparerIdx, askPath, ask)
	require.Equalf(t, http.StatusForbidden, code, "preparer locked out of ask-question: %v", body)
	code, body = h.post(demanderIdx, askPath, ask)
	require.Equalf(t, http.StatusOK, code, "winner drives ask-question: %v", body)

	// The TARGET (a third party) answers as themselves; the winner cannot answer
	// on their behalf.
	answerPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/answer-question"
	code, _ = h.post(demanderIdx, answerPath, map[string]any{"answer": "not mine to give"})
	require.Equal(t, http.StatusForbidden, code, "the winner cannot answer for the target")
	code, body = h.post(targetIdx, answerPath, map[string]any{"answer": "at home"})
	require.Equalf(t, http.StatusOK, code, "the question target answers as themselves: %v", body)
}
