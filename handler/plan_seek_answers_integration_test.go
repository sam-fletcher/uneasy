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
	"strings"
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
	assert.False(t, intact.IsTorn, "the rejected break must not tear a marginalium")
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

	code, body = h.post(preparerIdx, revealPath, map[string]any{"asset_id": assetB})
	require.Equalf(t, http.StatusConflict, code, "reveal beyond the picked count must be rejected: %v", body)
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
	assert.False(t, intact.IsTorn, "the rejected break must not tear a marginalium")
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
	// them — the difficulty was already locked at resolve, so this is safe.
	saPinKnowledgeRank(t, h, plan.PreparerID, 1)

	h.makeChoice(plan.ID, "make", []string{"ask_question"})

	askPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/ask-question"
	answerPath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/answer-question"
	completePath := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/complete"

	code, body := h.post(preparerIdx, askPath, map[string]any{"target_id": targetID, "question": "Where were you?"})
	require.Equalf(t, http.StatusOK, code, "ask: %v", body)
	assert.Equal(t, false, body["vetoable"], "a lower-ranked target cannot veto")

	rd := loadResolutionData(mustGetPlan(t, h, plan.ID).ResolutionData)
	require.NotNil(t, rd.SeekAnswers.PendingQuestion)

	// The WaitingOn bar must name the target: the row blocks on them.
	rs, err := ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RowStateAwaitQuestionAnswer, rs.Kind)
	require.Len(t, rs.ActingPlayerIDs, 1)
	assert.Equal(t, targetID, rs.ActingPlayerIDs[0])

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

	// The block clears once answered — the row no longer waits on the target.
	rs, err = ComputeRowState(context.Background(), h.q, h.tg.Game.ID)
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
	saPinKnowledgeRank(t, h, targetID, 1) // outranks the preparer (rank 3)

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
