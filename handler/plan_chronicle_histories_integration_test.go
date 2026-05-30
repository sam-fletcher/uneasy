//go:build integration

// handler/plan_chronicle_histories_integration_test.go — mechanical-effect
// coverage for Chronicle Histories. Guards the rules-correct behaviour added
// after the audit:
//
//   - make budget cap: "choose options equal to your result" (ChoiceLimiter).
//   - break-artifact uses breakMarginalia (auto-destroy on the last marginalium).
//   - mar: every player present must submit one choice before completion;
//     a mar break_artifact tears its marginalium atomically in the mar-choice.

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// chSeedArtifact creates an artifact owned by players[ownerIdx] with `margs`
// intact marginalia and returns the asset id plus the marginalia ids in order.
func chSeedArtifact(t *testing.T, h *planLifecycle, ownerIdx int, name string, margs int) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	a, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    h.tg.Game.ID,
		OwnerID:   h.tg.Players[ownerIdx].ID,
		CreatorID: h.tg.Players[ownerIdx].ID,
		AssetType: model.AssetArtifact,
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

// chInvoke drives POST /api/plans/{planId}/invoke-artifact as the preparer
// during the pre-roll invoke phase. Asserts 200.
func chInvoke(t *testing.T, h *planLifecycle, planID, assetID int64) {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/invoke-artifact"
	code, body := h.post(h.preparerIdxFor(planID), path, map[string]any{"asset_id": assetID})
	require.Equalf(t, http.StatusOK, code, "invoke-artifact: %v", body)
}

// chCastRoll drives POST /api/plans/{planId}/cast-roll as the preparer to close
// the invoke phase and create the dice roll. Asserts 200 and returns the roll.
func chCastRoll(t *testing.T, h *planLifecycle, planID int64) *dbgen.DiceRoll {
	t.Helper()
	path := "/api/plans/" + strconv.FormatInt(planID, 10) + "/cast-roll"
	code, body := h.post(h.preparerIdxFor(planID), path, nil)
	require.Equalf(t, http.StatusOK, code, "cast-roll: %v", body)
	rollBlob, _ := json.Marshal(body["roll"])
	var roll dbgen.DiceRoll
	require.NoError(t, json.Unmarshal(rollBlob, &roll))
	return &roll
}

// chPrepareToRoll drives a Chronicle Histories plan to a forced roll and
// returns the plan and the preparer's index. Difficulty is pinned via the
// preparer's knowledge rank so make/mar deltas are deterministic. The plan is
// left pre make-choice.
func chPrepareToRoll(t *testing.T, h *planLifecycle, outcome string, resultDelta int16) (dbgen.Plan, int, *dbgen.DiceRoll) {
	t.Helper()
	notes := "the lost charter"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanChronicleHistories,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)

	// Pin knowledge rank to 3 so difficulty = max(rank, #invoked) = 3 (no
	// pre-roll invocations here). Difficulty is computed at cast-roll time.
	saPinKnowledgeRank(t, h, plan.PreparerID, 3)

	h.jumpToRow(*plan.RowNumber)
	// OnResolve opens the invoke phase without a roll; the preparer casts it.
	require.Nil(t, h.resolve(plan.ID), "Chronicle Histories opens the invoke phase with no roll")
	roll := chCastRoll(t, h, plan.ID)
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

// TestChronicleHistories_Make_EnforcesOptionBudget proves make-choice caps the
// number of options at the dice result.
func TestChronicleHistories_Make_EnforcesOptionBudget(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	// Consistent make: result = difficulty + 0 = 3, so the budget is 3.
	plan, preparerIdx, _ := chPrepareToRoll(t, h, "make", 0)
	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"

	// Four options exceeds the budget of 3.
	code, body := h.post(preparerIdx, path, map[string]any{
		"result": "make",
		"choices": []string{
			"echo_present", "echo_present", "echo_present", "echo_present",
		},
	})
	assert.Equalf(t, http.StatusUnprocessableEntity, code, "over-budget should 422: %v", body)

	// Exactly 3 is accepted.
	code, body = h.post(preparerIdx, path, map[string]any{
		"result":  "make",
		"choices": []string{"echo_present", "echo_present", "echo_present"},
	})
	require.Equalf(t, http.StatusOK, code, "within budget should succeed: %v", body)
}

// TestChronicleHistories_PreRollInvoke_RaisesDifficulty proves the pre-roll
// invoke phase is reachable through the real routes and that invoking more
// artifacts than the preparer's knowledge rank raises the difficulty to the
// number invoked (difficulty = max(rank, #invoked)).
func TestChronicleHistories_PreRollInvoke_RaisesDifficulty(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	notes := "the lost charter"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanChronicleHistories,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	preparerIdx := h.preparerIdxFor(plan.ID)

	// Pin knowledge rank low so the #invoked artifacts dominates.
	saPinKnowledgeRank(t, h, plan.PreparerID, 2)

	// Three artifacts owned by another player, to be invoked in the pre-roll.
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	a1, _ := chSeedArtifact(t, h, otherIdx, "first folio", 1)
	a2, _ := chSeedArtifact(t, h, otherIdx, "second folio", 1)
	a3, _ := chSeedArtifact(t, h, otherIdx, "third folio", 1)

	h.jumpToRow(*plan.RowNumber)
	// Resolve opens the invoke phase; no roll yet.
	require.Nil(t, h.resolve(plan.ID), "pre-roll invoke phase opens with no roll")

	// Invoke three artifacts via the real route, then cast.
	chInvoke(t, h, plan.ID, a1)
	chInvoke(t, h, plan.ID, a2)
	chInvoke(t, h, plan.ID, a3)
	roll := chCastRoll(t, h, plan.ID)
	require.NotNil(t, roll)
	assert.EqualValues(t, 3, roll.Difficulty,
		"difficulty = max(knowledge rank 2, 3 invoked) = 3")

	// The invoke phase is now closed; further invokes are rejected.
	invokePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/invoke-artifact"
	code, body := h.post(preparerIdx, invokePath, map[string]any{"asset_id": a1})
	assert.Equalf(t, http.StatusConflict, code, "invoke after cast should 409: %v", body)

	// And cast-roll is idempotent-safe — a second cast is rejected.
	castPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/cast-roll"
	code, body = h.post(preparerIdx, castPath, nil)
	assert.Equalf(t, http.StatusConflict, code, "second cast should 409: %v", body)
}

// TestChronicleHistories_Make_BreakInvokedArtifact_AutoDestroys invokes an
// artifact pre-roll, then breaks its single marginalium on the make path and
// asserts auto-destruction.
func TestChronicleHistories_Make_BreakInvokedArtifact_AutoDestroys(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "the lost charter"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanChronicleHistories,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	preparerIdx := h.preparerIdxFor(plan.ID)
	saPinKnowledgeRank(t, h, plan.PreparerID, 3)

	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	artifactID, margIDs := chSeedArtifact(t, h, otherIdx, "brittle scroll", 1)

	// Resolve opens the invoke phase; invoke pre-roll via the real route, cast.
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))
	chInvoke(t, h, plan.ID, artifactID)
	roll := chCastRoll(t, h, plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, "make", roll.Difficulty) // consistent make

	h.makeChoice(plan.ID, "make", []string{"break_artifact"})

	breakPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-artifact"
	code, body := h.post(preparerIdx, breakPath, map[string]any{
		"asset_id": artifactID, "marginalia_id": margIDs[0],
	})
	require.Equalf(t, http.StatusOK, code, "break-artifact: %v", body)
	assert.Equal(t, true, body["destroyed"], "tearing the last marginalium destroys the artifact")

	destroyed, err := h.q.GetAssetByID(ctx, artifactID)
	require.NoError(t, err)
	assert.True(t, destroyed.IsDestroyed, "invoked artifact should be destroyed")

	h.complete(plan.ID)
}

// TestChronicleHistories_Mar_AllPlayersMustChoose proves a marred plan blocks
// completion until every player present submits one choice, and that a mar
// break_artifact tears its marginalium atomically.
func TestChronicleHistories_Mar_AllPlayersMustChoose(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "the lost charter"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanChronicleHistories,
		PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	preparerIdx := h.preparerIdxFor(plan.ID)
	saPinKnowledgeRank(t, h, plan.PreparerID, 3)

	// An invoked artifact with two marginalia so one mar break won't destroy it.
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	artifactID, margIDs := chSeedArtifact(t, h, otherIdx, "ancient codex", 2)

	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))
	chInvoke(t, h, plan.ID, artifactID)
	roll := chCastRoll(t, h, plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1) // consistent mar

	marPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/mar-choice"
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"

	// Player 0 chooses echo_present.
	code, body := h.post(0, marPath, map[string]any{"choice": "echo_present"})
	require.Equalf(t, http.StatusOK, code, "p0 mar-choice: %v", body)
	assert.EqualValues(t, 3, body["required_choices"], "gate target = player count")

	// Re-submitting is rejected (one choice per player).
	code, body = h.post(0, marPath, map[string]any{"choice": "total_control"})
	assert.Equalf(t, http.StatusConflict, code, "double mar-choice should 409: %v", body)

	// Completion blocked with only 1 of 3 submitted.
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should block at 1/3: %v", body)

	// Player 1 breaks the invoked artifact atomically.
	code, body = h.post(1, marPath, map[string]any{
		"choice": "break_artifact", "asset_id": artifactID, "marginalia_id": margIDs[0],
	})
	require.Equalf(t, http.StatusOK, code, "p1 mar break: %v", body)
	torn, err := h.q.GetMarginaliaByID(ctx, margIDs[0])
	require.NoError(t, err)
	assert.True(t, torn.IsTorn, "mar break_artifact should tear the marginalium in-call")

	// Still blocked at 2/3.
	code, body = h.post(preparerIdx, completePath, nil)
	require.Equalf(t, http.StatusConflict, code, "complete should block at 2/3: %v", body)

	// Player 2 (the preparer) chooses the final option.
	code, body = h.post(2, marPath, map[string]any{"choice": "total_control"})
	require.Equalf(t, http.StatusOK, code, "p2 mar-choice: %v", body)

	// Now completion succeeds.
	h.complete(plan.ID)
}
