//go:build integration

// handler/plan_seek_answers_integration_test.go — mechanical-effect coverage
// for Seek Answers. These guard the rules-correct behaviour added after the
// audit found the handler ignored the mar self-flaw penalty and bypassed the
// canonical break helper:
//
//   - make "break_resource": tears one marginalium, auto-destroys on the last,
//     and rejects flawing the same resource twice ("overlooked until now").
//   - mar penalty: the preparer must describe a flaw in (difficulty − result)
//     of their *own* resources; completion is blocked until satisfied.

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
	roll := h.resolve(plan.ID)
	require.Equal(t, int16(3), roll.Difficulty, "pinned knowledge rank should set difficulty")
	require.NotNil(t, roll, "Seek Answers creates its roll on resolve")

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
// break tears one marginalium and destroys the resource when it was the last,
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
	assert.Equal(t, true, body["destroyed"], "tearing the last marginalium destroys the asset")

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

	// No make-list options — just suffer the penalty.
	h.makeChoice(plan.ID, "mar", []string{})

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

	// Sanity: own resources lost their marginalium and were destroyed.
	for _, id := range []int64{ownA, ownB} {
		a, err := h.q.GetAssetByID(ctx, id)
		require.NoError(t, err)
		assert.True(t, a.IsDestroyed, "self-flawed single-marginalium resource should be destroyed")
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
