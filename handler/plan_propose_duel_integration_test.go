//go:build integration

package handler

// plan_propose_duel_integration_test.go — end-to-end HTTP coverage for the
// Propose Duel post-roll result: stake claiming (make + mar), the
// staked-only / over-budget guards on the claim, leverage of all stakes, and
// the plan.propose_duel action-log entry.

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// duelSystemPosts returns the bodies of all plan.propose_duel action-log posts.
func duelSystemPosts(t *testing.T, q *dbgen.Queries, gameID int64) []string {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "plan.propose_duel" {
			out = append(out, p.Body)
		}
	}
	return out
}

// duelPostWith returns the first plan.propose_duel post body containing substr.
// The duel emits many narrative posts (champions, stakes, bouts, outcome), so
// callers locate the one they care about rather than assuming a position.
func duelPostWith(posts []string, substr string) (string, bool) {
	for _, p := range posts {
		if strings.Contains(p, substr) {
			return p, true
		}
	}
	return "", false
}

// pinEsteemRank forces playerID into a specific esteem rank by swapping slots
// with the current occupant (the (game,category,rank) row keys the slot, not
// the player — see saPinKnowledgeRank).
func pinEsteemRank(t *testing.T, h *planLifecycle, playerID int64, target int16) {
	t.Helper()
	ctx := context.Background()
	ranks, err := h.q.ListRankingsByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)

	var curRank int16
	var occupant *int64
	for _, r := range ranks {
		if r.Category != model.CategoryEsteem || r.PlayerID == nil {
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
		GameID: h.tg.Game.ID, PlayerID: &pid, Category: model.CategoryEsteem, Rank: target,
	}))
	if occupant != nil && curRank != 0 {
		require.NoError(t, h.q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: h.tg.Game.ID, PlayerID: occupant, Category: model.CategoryEsteem, Rank: curRank,
		}))
	}
}

// idxOfPlayer returns the Players index for a player ID.
func idxOfPlayer(t *testing.T, h *planLifecycle, playerID int64) int {
	t.Helper()
	for i, p := range h.tg.Players {
		if p.ID == playerID {
			return i
		}
	}
	t.Fatalf("player id %d not in test players", playerID)
	return -1
}

// duelInitiative reads the side currently holding initiative from the plan's
// resolution data.
func duelInitiative(t *testing.T, h *planLifecycle, planID int64) int64 {
	t.Helper()
	plan, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(t, err)
	d := game.LoadDuelData(plan.ResolutionData)
	require.NotNil(t, d.InitiativePlayerID, "no initiative set")
	return *d.InitiativePlayerID
}

// firstUnresolvedStakeID returns the stake id of the player's first unresolved
// staked asset.
func firstUnresolvedStakeID(t *testing.T, h *planLifecycle, planID, playerID int64) int64 {
	t.Helper()
	stakes, err := h.q.ListDuelStakesByPlanPlayer(context.Background(),
		dbgen.ListDuelStakesByPlanPlayerParams{PlanID: planID, PlayerID: playerID})
	require.NoError(t, err)
	for _, s := range stakes {
		if !s.IsResolved {
			return s.ID
		}
	}
	t.Fatalf("player %d has no unresolved stake", playerID)
	return 0
}

// route builds the /api/plans/{id}/{name} extra-route path.
func duelRoute(planID int64, name string) string {
	return "/api/plans/" + strconv.FormatInt(planID, 10) + "/" + name
}

// seedDuelToRoll prepares a propose_duel from the focus player against targIdx,
// pins both duellists' esteem ranks (preparer high / target low so difficulty
// is deterministic), elects "fight myself" champions, stakes the given assets,
// runs the bouts, and returns the created final roll. prepStakeAssets and
// targStakeAssets are the asset IDs each side stakes.
func seedDuelToRoll(
	t *testing.T,
	h *planLifecycle,
	targIdx int,
	prepStakeAssets, targStakeAssets []int64,
) (planID int64, prepIdx int) {
	t.Helper()
	prepIdx = h.focusPlayerIdx()
	targetID := h.tg.Players[targIdx].ID
	notes := "Courtyard duel at dawn"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanProposeDuel,
		TargetPlayerID:   &targetID,
		DuelType:         "arms",
		PreparationNotes: &notes,
	})

	// Deterministic difficulty: target at esteem rank 5 → difficulty 1; the
	// preparer at rank 1 holds initiative (lower rank = higher status).
	pinEsteemRank(t, h, h.tg.Players[targIdx].ID, 5)
	pinEsteemRank(t, h, h.tg.Players[prepIdx].ID, 1)

	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID) // duel resolve returns no roll; sets phase=setup

	// Champion election: the initiative-holder declares first, then the other.
	init := duelInitiative(t, h, plan.ID)
	initIdx := idxOfPlayer(t, h, init)
	otherIdx := prepIdx
	if initIdx == prepIdx {
		otherIdx = targIdx
	}
	for _, idx := range []int{initIdx, otherIdx} {
		code, body := h.post(idx, duelRoute(plan.ID, "elect-champion"),
			map[string]any{"asset_id": nil})
		require.Equalf(t, http.StatusOK, code, "elect-champion: %v", body)
	}

	// Stake-count reveal (simultaneous).
	codeP, bodyP := h.post(prepIdx, duelRoute(plan.ID, "stake-reveal"),
		map[string]any{"count": len(prepStakeAssets)})
	require.Equalf(t, http.StatusOK, codeP, "prep stake-reveal: %v", bodyP)
	codeT, bodyT := h.post(targIdx, duelRoute(plan.ID, "stake-reveal"),
		map[string]any{"count": len(targStakeAssets)})
	require.Equalf(t, http.StatusOK, codeT, "target stake-reveal: %v", bodyT)

	// Select specific stakes.
	codeP, bodyP = h.post(prepIdx, duelRoute(plan.ID, "select-stakes"),
		map[string]any{"asset_ids": prepStakeAssets})
	require.Equalf(t, http.StatusOK, codeP, "prep select-stakes: %v", bodyP)
	codeT, bodyT = h.post(targIdx, duelRoute(plan.ID, "select-stakes"),
		map[string]any{"asset_ids": targStakeAssets})
	require.Equalf(t, http.StatusOK, codeT, "target select-stakes: %v", bodyT)

	// Run bouts until the final roll is created (one side runs out of stakes).
	runDuelBouts(t, h, plan.ID, prepIdx, targIdx)

	_, err := h.q.GetDiceRollByPlanID(context.Background(), &plan.ID)
	require.NoError(t, err, "final roll should exist after bouts")
	return plan.ID, prepIdx
}

// runDuelBouts drives declare/respond pairs until the handler creates the
// plan's final dice roll.
func runDuelBouts(t *testing.T, h *planLifecycle, planID int64, prepIdx, targIdx int) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if _, err := h.q.GetDiceRollByPlanID(context.Background(), &planID); err == nil {
			return
		}
		init := duelInitiative(t, h, planID)
		initIdx := idxOfPlayer(t, h, init)
		respIdx := prepIdx
		if initIdx == prepIdx {
			respIdx = targIdx
		}
		initStake := firstUnresolvedStakeID(t, h, planID, h.tg.Players[initIdx].ID)
		respStake := firstUnresolvedStakeID(t, h, planID, h.tg.Players[respIdx].ID)

		code, body := h.post(initIdx, duelRoute(planID, "bout-declare"),
			map[string]any{"stake_id": initStake, "declaration": "high"})
		require.Equalf(t, http.StatusOK, code, "bout-declare: %v", body)
		code, body = h.post(respIdx, duelRoute(planID, "bout-respond"),
			map[string]any{"stake_id": respStake})
		require.Equalf(t, http.StatusOK, code, "bout-respond: %v", body)
	}
	t.Fatal("bouts did not complete within 20 iterations")
}

// ── Tests ────────────────────────────────────────────────────────────────────

// TestProposeDuelHTTP_Make_TakesStakeLeveragesAndLogs: on a make, the preparer
// claims one of the target's staked assets; all stakes end leveraged; the
// action-log records the result.
func TestProposeDuelHTTP_Make_TakesStakeLeveragesAndLogs(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	planID, prepIdx := seedDuelToRoll(t, h, targIdx,
		[]int64{prepPeer}, []int64{targPeer})

	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	h.forceRoll(roll.ID, "make", 2) // result 2 ≥ difficulty 1 → consistent make

	// Preparer claims the target's staked peer.
	code, body := h.post(prepIdx, duelRoute(planID, "make-choice"),
		map[string]any{"result": "make", "choices": []string{strconv.FormatInt(targPeer, 10)}})
	require.Equalf(t, http.StatusOK, code, "make-choice: %v", body)
	h.complete(planID)

	ctx := context.Background()
	// Target's peer transferred to the preparer.
	got, err := h.q.GetAssetByID(ctx, targPeer)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[prepIdx].ID, got.OwnerID, "target peer should move to preparer")

	// Both staked assets leveraged.
	for _, aid := range []int64{prepPeer, targPeer} {
		a, err := h.q.GetAssetByID(ctx, aid)
		require.NoError(t, err)
		require.Truef(t, a.IsLeveraged, "staked asset %d should be leveraged", aid)
	}

	// Action-log emitted.
	posts := duelSystemPosts(t, h.q, h.tg.Game.ID)
	require.NotEmpty(t, posts, "expected a plan.propose_duel action-log post")
	outcome, ok := duelPostWith(posts, "won the duel")
	require.True(t, ok, "expected a duel-outcome post")
	require.Contains(t, outcome, "leveraged")
}

// TestProposeDuelHTTP_Mar_TargetTakesPreparerStake: on a mar, the target (a
// non-preparer) drives the claim and takes the preparer's staked asset.
func TestProposeDuelHTTP_Mar_TargetTakesPreparerStake(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	planID, prepIdx := seedDuelToRoll(t, h, targIdx,
		[]int64{prepPeer}, []int64{targPeer})

	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	h.forceRoll(roll.ID, "mar", 0) // result 0 < difficulty 1 → consistent mar

	// The target (non-preparer) claims the preparer's staked peer.
	code, body := h.post(targIdx, duelRoute(planID, "make-choice"),
		map[string]any{"result": "mar", "choices": []string{strconv.FormatInt(prepPeer, 10)}})
	require.Equalf(t, http.StatusOK, code, "mar make-choice: %v", body)
	h.complete(planID)

	ctx := context.Background()
	got, err := h.q.GetAssetByID(ctx, prepPeer)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[targIdx].ID, got.OwnerID, "preparer peer should move to target")
	_ = prepIdx

	posts := duelSystemPosts(t, h.q, h.tg.Game.ID)
	require.NotEmpty(t, posts)
	_, ok := duelPostWith(posts, "won the duel")
	require.True(t, ok, "expected a duel-outcome post")
}

// TestProposeDuelHTTP_RejectsNonStakedAndOverBudget: the claim must be limited
// to the loser's staked assets and may not exceed the result-derived budget.
func TestProposeDuelHTTP_RejectsNonStakedAndOverBudget(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeerA := h.seedPeer(targIdx, "Target stake A")
	targPeerB := h.seedPeer(targIdx, "Target stake B")
	targUnstaked := h.seedPeer(targIdx, "Target unstaked") // owned but never staked

	// Preparer stakes 1, target stakes 2 → one bout (preparer runs out).
	planID, prepIdx := seedDuelToRoll(t, h, targIdx,
		[]int64{prepPeer}, []int64{targPeerA, targPeerB})

	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	h.forceRoll(roll.ID, "make", 1) // difficulty 1, budget = result = 1

	prepID := prepIdx

	// (a) Claiming a non-staked (but owner-owned) asset is rejected.
	code, _ := h.post(prepID, duelRoute(planID, "make-choice"),
		map[string]any{"result": "make", "choices": []string{strconv.FormatInt(targUnstaked, 10)}})
	require.NotEqual(t, http.StatusOK, code, "non-staked asset should be rejected")

	// (b) Claiming more stakes than the result budget allows is rejected.
	code, _ = h.post(prepID, duelRoute(planID, "make-choice"), map[string]any{
		"result":  "make",
		"choices": []string{strconv.FormatInt(targPeerA, 10), strconv.FormatInt(targPeerB, 10)},
	})
	require.Equal(t, http.StatusUnprocessableEntity, code, "over-budget claim should be 422")

	// (c) A single valid staked claim succeeds.
	code, body := h.post(prepID, duelRoute(planID, "make-choice"),
		map[string]any{"result": "make", "choices": []string{strconv.FormatInt(targPeerA, 10)}})
	require.Equalf(t, http.StatusOK, code, "valid claim: %v", body)
	h.complete(planID)

	ctx := context.Background()
	got, err := h.q.GetAssetByID(ctx, targPeerA)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[prepIdx].ID, got.OwnerID)
	// The unclaimed stake is still leveraged.
	b, err := h.q.GetAssetByID(ctx, targPeerB)
	require.NoError(t, err)
	require.True(t, b.IsLeveraged, "unclaimed stake should still be leveraged")
}
