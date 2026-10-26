//go:build integration

package handler

// plan_propose_duel_integration_test.go — end-to-end HTTP coverage for the
// Propose Duel post-roll result: stake claiming (make + mar), the
// staked-only / over-budget guards on the claim, leverage of all stakes, and
// the plan.propose_duel action-log entry — plus the duel's information model
// (what duel-state lets each seat see mid-bout).
//
// The bout dice are server-rolled, so a test that cares about the outcome pins
// them (pinDuelDice) rather than forcing the finished roll: the duel's final
// roll is created already-resolved and its outcome is applied the instant it
// lands, so there is no window in which to override it afterwards.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	appMiddleware "uneasy/middleware"
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

// Dice pairs for pinDuelDice. runDuelBouts always declares "high", so with the
// preparer holding initiative these decide the whole duel: the side with the
// higher die takes every bout (a declarer who calls high on the lower die
// loses, and the other side wins the exchange).
//
//	preparer 6 vs target 1 → preparer sweeps → actor dice {6,1} → result 2 → make
//	preparer 1 vs target 6 → target sweeps   → no actor dice    → result 0 → mar
const (
	preparerSweepsPrep, preparerSweepsTarg int16 = 6, 1
	targetSweepsPrep, targetSweepsTarg     int16 = 1, 6
)

// seedDuelToBouts prepares a propose_duel from the focus player against
// targIdx, pins both duellists' esteem ranks (preparer high / target low so
// difficulty is deterministic), elects "fight myself" champions, and stakes the
// given assets. It stops there: the duel is in its 'bouts' phase with the
// preparer holding initiative and nothing declared. prepStakeAssets and
// targStakeAssets are the asset IDs each side stakes.
func seedDuelToBouts(
	t *testing.T,
	h *planLifecycle,
	targIdx int,
	prepStakeAssets, targStakeAssets []int64,
) (planID int64, prepIdx int) {
	t.Helper()
	return seedDuelToBoutsRanked(t, h, targIdx, prepStakeAssets, targStakeAssets, 5)
}

// seedDuelToBoutsRanked is seedDuelToBouts with the target's esteem rank chosen.
// The rank sets both the difficulty (status = 6 − rank) and how many assets the
// target may stake (1 + status), so a test needing a big pile against a small
// result picks a higher-status target: rank 5 → difficulty 1, ≤ 2 stakes;
// rank 4 → difficulty 2, ≤ 3 stakes.
func seedDuelToBoutsRanked(
	t *testing.T,
	h *planLifecycle,
	targIdx int,
	prepStakeAssets, targStakeAssets []int64,
	targRank int16,
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

	// Deterministic difficulty from the target's pinned rank; the preparer at
	// rank 1 holds initiative (lower rank = higher status).
	pinEsteemRank(t, h, h.tg.Players[targIdx].ID, targRank)
	pinEsteemRank(t, h, h.tg.Players[prepIdx].ID, 1)

	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID) // duel resolve returns no roll; sets phase=setup

	// Champion election is optional and simultaneous; nil = fight in person.
	// Both sides pass here so the duel proceeds without champions.
	for _, idx := range []int{prepIdx, targIdx} {
		code, body := h.post(idx, duelRoute(plan.ID, "elect-champion"),
			map[string]any{"asset_id": nil})
		require.Equalf(t, http.StatusOK, code, "elect-champion: %v", body)
	}

	// Commit stakes — a single combined step (count = len(asset_ids)). The
	// opponent's stakes stay hidden until both commit; the second commit
	// advances the duel straight to the bouts.
	codeP, bodyP := h.post(prepIdx, duelRoute(plan.ID, "select-stakes"),
		map[string]any{"asset_ids": prepStakeAssets})
	require.Equalf(t, http.StatusOK, codeP, "prep select-stakes: %v", bodyP)
	codeT, bodyT := h.post(targIdx, duelRoute(plan.ID, "select-stakes"),
		map[string]any{"asset_ids": targStakeAssets})
	require.Equalf(t, http.StatusOK, codeT, "target select-stakes: %v", bodyT)
	return plan.ID, prepIdx
}

// pinDuelDice overwrites the hidden die under every stake each side has put up,
// making the bouts' outcome deterministic. Call it after seedDuelToBouts and
// before any bout is declared.
func pinDuelDice(t *testing.T, h *planLifecycle, planID int64, prepIdx, targIdx int, prepDie, targDie int16) {
	t.Helper()
	for idx, die := range map[int]int16{prepIdx: prepDie, targIdx: targDie} {
		_, err := h.pool.Exec(context.Background(),
			`UPDATE duel_staked_assets SET hidden_die = $1 WHERE plan_id = $2 AND player_id = $3`,
			die, planID, h.tg.Players[idx].ID)
		require.NoError(t, err)
	}
}

// seedDuelToRoll seeds a duel through its bouts with unpinned (random) dice and
// returns once the final roll exists. Use seedDuelToRollPinned when the make/mar
// outcome matters.
func seedDuelToRoll(
	t *testing.T,
	h *planLifecycle,
	targIdx int,
	prepStakeAssets, targStakeAssets []int64,
) (planID int64, prepIdx int) {
	t.Helper()
	planID, prepIdx = seedDuelToBouts(t, h, targIdx, prepStakeAssets, targStakeAssets)
	runDuelBouts(t, h, planID, prepIdx, targIdx)
	return planID, prepIdx
}

// seedDuelToRollPinned is seedDuelToRoll with the stakes' dice fixed first, so
// the final roll's outcome is known. See the preparerSweeps / targetSweeps
// pairs.
func seedDuelToRollPinned(
	t *testing.T,
	h *planLifecycle,
	targIdx int,
	prepStakeAssets, targStakeAssets []int64,
	prepDie, targDie int16,
) (planID int64, prepIdx int) {
	t.Helper()
	planID, prepIdx = seedDuelToBouts(t, h, targIdx, prepStakeAssets, targStakeAssets)
	pinDuelDice(t, h, planID, prepIdx, targIdx, prepDie, targDie)
	runDuelBouts(t, h, planID, prepIdx, targIdx)

	_, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err, "final roll should exist after bouts")
	return planID, prepIdx
}

// duelStateAs GETs /api/plans/{id}/duel-state as players[idx] and returns the
// raw body alongside its decoded bouts. The harness router doesn't mount this
// read route, so a one-route router with the same session middleware does.
func duelStateAs(t *testing.T, h *planLifecycle, idx int, planID int64) (raw string, bouts []map[string]any) {
	t.Helper()
	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(h.q))
	r.Get("/api/plans/{planId}/duel-state", GetDuelState(h.store))

	req := httptest.NewRequest("GET", duelRoute(planID, "duel-state"), nil)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[idx]})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusOK, rec.Code, "duel-state: %s", rec.Body.String())

	var out struct {
		Bouts []map[string]any `json:"bouts"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return rec.Body.String(), out.Bouts
}

// planStatusOf reads a plan's current status and stored result.
func planStatusOf(t *testing.T, h *planLifecycle, planID int64) (model.PlanStatus, string) {
	t.Helper()
	p, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(t, err)
	result := ""
	if p.Result != nil {
		result = *p.Result
	}
	return p.Status, result
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

// TestProposeDuelHTTP_Make_TakesStakeLeveragesAndLogs: on a make, the preparer's
// result (2) covers the target's lone stake, so "take it" is the only legal
// claim — the server applies it the moment the roll lands, with no picker and no
// "Complete plan" click. The stake moves, all stakes end leveraged, the plan has
// resolved itself, and the action log records the result.
func TestProposeDuelHTTP_Make_TakesStakeLeveragesAndLogs(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	planID, prepIdx := seedDuelToRollPinned(t, h, targIdx,
		[]int64{prepPeer}, []int64{targPeer}, preparerSweepsPrep, preparerSweepsTarg)

	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	require.NotNil(t, roll.Outcome)
	require.Equal(t, "make", *roll.Outcome, "pinned dice: the preparer sweeps")

	// No make-choice and no complete were sent: the duel finished by itself.
	status, result := planStatusOf(t, h, planID)
	require.Equal(t, model.PlanResolved, status, "a forced claim must not wait for a click")
	require.Equal(t, "make", result)

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

// TestProposeDuelHTTP_Mar_TargetTakesPreparerStake: on a mar the target wins and
// takes the preparer's staked asset. The difficulty (1) covers the preparer's
// lone stake, so the claim is forced and applied automatically — and the plan
// resolves without the PREPARER, who just lost, having to click "Complete".
func TestProposeDuelHTTP_Mar_TargetTakesPreparerStake(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	planID, _ := seedDuelToRollPinned(t, h, targIdx,
		[]int64{prepPeer}, []int64{targPeer}, targetSweepsPrep, targetSweepsTarg)

	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	require.NotNil(t, roll.Outcome)
	require.Equal(t, "mar", *roll.Outcome, "pinned dice: the target sweeps")

	status, result := planStatusOf(t, h, planID)
	require.Equal(t, model.PlanResolved, status, "the loser must not be left holding a Complete button")
	require.Equal(t, "mar", result)

	got, err := h.q.GetAssetByID(context.Background(), prepPeer)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[targIdx].ID, got.OwnerID, "preparer peer should move to target")

	posts := duelSystemPosts(t, h.q, h.tg.Game.ID)
	require.NotEmpty(t, posts)
	_, ok := duelPostWith(posts, "won the duel")
	require.True(t, ok, "expected a duel-outcome post")
}

// TestProposeDuelHTTP_FinalRollAutoResolves: the duel's final roll has no
// leverage window. The bout dice are the whole roll, so once the bouts finish
// the server resolves the roll immediately — it lands at stage='resolved' with
// a result/outcome, ready for the make-choice claim, rather than parking in the
// leverage stage waiting on input that the rules don't permit.
func TestProposeDuelHTTP_FinalRollAutoResolves(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	planID, _ := seedDuelToRoll(t, h, targIdx,
		[]int64{prepPeer}, []int64{targPeer})

	// No forceRoll: assert the bout-respond handler resolved it on its own.
	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	require.Equal(t, "resolved", roll.Stage, "final roll should skip leverage and resolve")
	require.NotNil(t, roll.Result, "auto-resolved roll should have a result")
	require.NotNil(t, roll.Outcome, "auto-resolved roll should have an outcome")
	require.Contains(t, []string{"make", "mar"}, *roll.Outcome)
}

// TestProposeDuelHTTP_AllowsLeveragedStake: an already-leveraged asset may be
// staked. The rules place no leverage (or type) restriction on stakes, and
// resolution leverages every stake again — a no-op for one already leveraged.
func TestProposeDuelHTTP_AllowsLeveragedStake(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	// Pre-leverage the preparer's stake; it must still be stakeable.
	require.NoError(t, h.q.SetAssetLeveraged(context.Background(),
		dbgen.SetAssetLeveragedParams{ID: prepPeer, IsLeveraged: true}))

	// seedDuelToRoll drives select-stakes; if the leveraged asset were rejected
	// its internal require would fail before we reach the roll.
	planID, _ := seedDuelToRoll(t, h, targIdx, []int64{prepPeer}, []int64{targPeer})

	_, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err, "leveraged stake should flow through to the final roll")
}

// TestProposeDuelHTTP_StakeLogGatedOnBothCommitting: the stake-commit narration
// must not fire on the first commit (it names the count, which would leak it and
// hand the second mover an advantage). Both sides' commits are logged together
// only once both have committed.
func TestProposeDuelHTTP_StakeLogGatedOnBothCommitting(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	targetID := h.tg.Players[targIdx].ID

	prepPeer := h.seedPeer(prepIdx, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	notes := "Courtyard duel at dawn"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanProposeDuel,
		TargetPlayerID:   &targetID,
		DuelType:         "arms",
		PreparationNotes: &notes,
	})
	pinEsteemRank(t, h, targetID, 5)
	pinEsteemRank(t, h, h.tg.Players[prepIdx].ID, 1)
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)

	// Preparer commits first → no stake-commit narration yet.
	code, body := h.post(prepIdx, duelRoute(plan.ID, "select-stakes"),
		map[string]any{"asset_ids": []int64{prepPeer}})
	require.Equalf(t, http.StatusOK, code, "prep select-stakes: %v", body)
	_, leaked := duelPostWith(duelSystemPosts(t, h.q, h.tg.Game.ID), "on the line")
	require.False(t, leaked, "stake-commit log must not appear before both commit")

	// Target commits second → both commits are narrated together.
	code, body = h.post(targIdx, duelRoute(plan.ID, "select-stakes"),
		map[string]any{"asset_ids": []int64{targPeer}})
	require.Equalf(t, http.StatusOK, code, "target select-stakes: %v", body)
	var onTheLine int
	for _, p := range duelSystemPosts(t, h.q, h.tg.Game.ID) {
		if strings.Contains(p, "on the line") {
			onTheLine++
		}
	}
	require.Equal(t, 2, onTheLine, "both duellists' stake commits logged once both are in")
}

// TestProposeDuelHTTP_RejectsNonStakedAndOverBudget: when the winner's budget is
// SMALLER than the loser's pile, the claim is a real pick, so the duel waits for
// it — and the claim must be limited to the loser's staked assets and may not
// exceed the result-derived budget. The preparer sweeps (result 2) against a
// target who staked three, so exactly two may be taken.
func TestProposeDuelHTTP_RejectsNonStakedAndOverBudget(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeerA := h.seedPeer(targIdx, "Target stake A")
	targPeerB := h.seedPeer(targIdx, "Target stake B")
	targPeerC := h.seedPeer(targIdx, "Target stake C")
	targUnstaked := h.seedPeer(targIdx, "Target unstaked") // owned but never staked

	// Preparer stakes 1, target stakes 3 → one bout (preparer runs out). The
	// target sits at esteem rank 4: difficulty 2 (so the preparer's result of 2
	// is still a make) and room for three stakes.
	planID, prepIdx := seedDuelToBoutsRanked(t, h, targIdx,
		[]int64{prepPeer}, []int64{targPeerA, targPeerB, targPeerC}, 4)
	pinDuelDice(t, h, planID, prepIdx, targIdx, preparerSweepsPrep, preparerSweepsTarg)
	runDuelBouts(t, h, planID, prepIdx, targIdx)

	ctx := context.Background()
	roll, err := h.q.GetDiceRollByPlanID(ctx, &planID)
	require.NoError(t, err)
	require.NotNil(t, roll.Result)
	require.NotNil(t, roll.Outcome)
	require.Equal(t, "make", *roll.Outcome)
	require.EqualValues(t, 2, *roll.Result, "budget = result = 2, against a pile of 3")

	// A genuine subset pick is NOT auto-applied: the duel is still waiting.
	status, _ := planStatusOf(t, h, planID)
	require.Equal(t, model.PlanResolving, status, "a real choice must still be asked for")

	prepID := prepIdx

	// (a) Claiming a non-staked (but owner-owned) asset is rejected…
	code, _ := h.post(prepID, duelRoute(planID, "make-choice"),
		map[string]any{"result": "make", "choices": []string{strconv.FormatInt(targUnstaked, 10)}})
	require.NotEqual(t, http.StatusOK, code, "non-staked asset should be rejected")

	// …and a rejected claim leaves no half-applied state: nothing was leveraged.
	for _, aid := range []int64{prepPeer, targPeerA, targPeerB, targPeerC} {
		a, err := h.q.GetAssetByID(ctx, aid)
		require.NoError(t, err)
		require.Falsef(t, a.IsLeveraged, "rejected claim must not leverage staked asset %d", aid)
	}

	// (b) Claiming more stakes than the result budget allows is rejected.
	code, _ = h.post(prepID, duelRoute(planID, "make-choice"), map[string]any{
		"result": "make",
		"choices": []string{
			strconv.FormatInt(targPeerA, 10), strconv.FormatInt(targPeerB, 10), strconv.FormatInt(targPeerC, 10),
		},
	})
	require.Equal(t, http.StatusUnprocessableEntity, code, "over-budget claim should be 422")

	// (c) A valid two-stake claim succeeds, and the plan resolves with it.
	code, body := h.post(prepID, duelRoute(planID, "make-choice"), map[string]any{
		"result":  "make",
		"choices": []string{strconv.FormatInt(targPeerA, 10), strconv.FormatInt(targPeerB, 10)},
	})
	require.Equalf(t, http.StatusOK, code, "valid claim: %v", body)

	status, result := planStatusOf(t, h, planID)
	require.Equal(t, model.PlanResolved, status, "the claim was the last step; no Complete click")
	require.Equal(t, "make", result)

	for _, aid := range []int64{targPeerA, targPeerB} {
		got, err := h.q.GetAssetByID(ctx, aid)
		require.NoError(t, err)
		require.Equal(t, h.tg.Players[prepIdx].ID, got.OwnerID)
	}
	// The unclaimed stake stays with its owner, but is still leveraged.
	c, err := h.q.GetAssetByID(ctx, targPeerC)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[targIdx].ID, c.OwnerID)
	require.True(t, c.IsLeveraged, "unclaimed stake should still be leveraged")
}

// TestProposeDuelHTTP_PerformStepsWinnerDrivesPreparerSeat proves a Make Demands
// perform_steps win transfers the PREPARER's side of the bouts to the winner:
// they declare/respond with the preparer's stakes (and the bout records the
// preparer as the combatant), the preparer is locked out of their own seat, and
// the winner cannot usurp the TARGET duellist's seat — that third party responds
// as themselves.
func TestProposeDuelHTTP_PerformStepsWinnerDrivesPreparerSeat(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()

	prepIdx := h.focusPlayerIdx()
	targIdx := (prepIdx + 1) % 5
	demIdx := (prepIdx + 2) % 5
	targetID := h.tg.Players[targIdx].ID

	prepPeer := h.seedPeer(prepIdx, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	notes := "Courtyard duel at dawn"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanProposeDuel,
		TargetPlayerID:   &targetID,
		DuelType:         "arms",
		PreparationNotes: &notes,
	})
	// Preparer at esteem rank 1 holds initiative; target at rank 5.
	pinEsteemRank(t, h, targetID, 5)
	pinEsteemRank(t, h, h.tg.Players[prepIdx].ID, 1)
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)

	// Fight in person; stake one asset each so a single bout settles the duel.
	for _, idx := range []int{prepIdx, targIdx} {
		code, body := h.post(idx, duelRoute(plan.ID, "elect-champion"), map[string]any{"asset_id": nil})
		require.Equalf(t, http.StatusOK, code, "elect-champion: %v", body)
	}
	code, body := h.post(prepIdx, duelRoute(plan.ID, "select-stakes"), map[string]any{"asset_ids": []int64{prepPeer}})
	require.Equalf(t, http.StatusOK, code, "prep select-stakes: %v", body)
	code, body = h.post(targIdx, duelRoute(plan.ID, "select-stakes"), map[string]any{"asset_ids": []int64{targPeer}})
	require.Equalf(t, http.StatusOK, code, "target select-stakes: %v", body)

	// Phase is bouts; the preparer holds initiative and declares first.
	require.Equal(t, h.tg.Players[prepIdx].ID, duelInitiative(t, h, plan.ID), "preparer holds initiative")

	// A resolved, made demand hands perform_steps to the demander.
	h.seedMadeDemand(demIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[demIdx].ID,
	})

	prepStake := firstUnresolvedStakeID(t, h, plan.ID, h.tg.Players[prepIdx].ID)

	// The preparer is locked out of declaring their own bout.
	code, body = h.post(prepIdx, duelRoute(plan.ID, "bout-declare"),
		map[string]any{"stake_id": prepStake, "declaration": "high"})
	require.Equalf(t, http.StatusForbidden, code, "preparer locked out of their bout seat: %v", body)

	// The demander declares in the preparer's stead, using the preparer's stake.
	code, body = h.post(demIdx, duelRoute(plan.ID, "bout-declare"),
		map[string]any{"stake_id": prepStake, "declaration": "high"})
	require.Equalf(t, http.StatusOK, code, "winner drives the preparer's bout: %v", body)

	// The bout records the PREPARER as declarer (the seat), not the winner.
	latest, err := h.q.GetLatestDuelBout(ctx, plan.ID)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[prepIdx].ID, latest.DeclarerID, "bout declarer is the preparer's seat")

	// The winner cannot usurp the TARGET's seat: responding is the target's role.
	targStake := firstUnresolvedStakeID(t, h, plan.ID, h.tg.Players[targIdx].ID)
	code, _ = h.post(demIdx, duelRoute(plan.ID, "bout-respond"), map[string]any{"stake_id": targStake})
	require.Equal(t, http.StatusForbidden, code, "winner cannot respond for the target duellist")

	// The target responds as themselves (third-party seat untouched).
	code, body = h.post(targIdx, duelRoute(plan.ID, "bout-respond"), map[string]any{"stake_id": targStake})
	require.Equalf(t, http.StatusOK, code, "target responds as themselves: %v", body)
}

// TestProposeDuelHTTP_PerformStepsWinnerCannotDriveTargetMar proves the
// target-driven-mar tightening (TASK B): on a marred duel the TARGET claims the
// preparer's stakes, and a perform_steps winner — who inherits only the
// PREPARER's own outcomes — cannot reach that mar make-choice. The target drives
// it as themselves.
//
// The preparer stakes two against the target's one, so the mar budget
// (difficulty 1) is smaller than the pile: the claim is a genuine pick, which is
// what keeps a make-choice for the winner to be refused.
func TestProposeDuelHTTP_PerformStepsWinnerCannotDriveTargetMar(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5
	demIdx := (prepIdx0 + 2) % 5

	prepPeerA := h.seedPeer(prepIdx0, "Preparer stake A")
	prepPeerB := h.seedPeer(prepIdx0, "Preparer stake B")
	targPeer := h.seedPeer(targIdx, "Target stake")

	planID, _ := seedDuelToRollPinned(t, h, targIdx,
		[]int64{prepPeerA, prepPeerB}, []int64{targPeer}, targetSweepsPrep, targetSweepsTarg)
	roll, err := h.q.GetDiceRollByPlanID(ctx, &planID)
	require.NoError(t, err)
	require.NotNil(t, roll.Outcome)
	require.Equal(t, "mar", *roll.Outcome)

	// perform_steps demand → the demander holds the preparer's resolution role.
	h.seedMadeDemand(demIdx, planID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[demIdx].ID,
	})

	mcPath := duelRoute(planID, "make-choice")
	claim := map[string]any{"result": "mar", "choices": []string{strconv.FormatInt(prepPeerA, 10)}}

	// The winner cannot drive the target-driven mar.
	code, body := h.post(demIdx, mcPath, claim)
	require.Equalf(t, http.StatusForbidden, code, "winner must not reach the target-driven mar: %v", body)

	// The target (a third party to the demand) still claims the stake as themselves.
	code, body = h.post(targIdx, mcPath, claim)
	require.Equalf(t, http.StatusOK, code, "the target drives the mar as themselves: %v", body)

	got, err := h.q.GetAssetByID(ctx, prepPeerA)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[targIdx].ID, got.OwnerID, "the preparer's stake moves to the target")
}

// ── Information model ────────────────────────────────────────────────────────

// TestProposeDuelHTTP_DuelState_ResponderCannotReadDeclarersDie is the leak's
// regression test, over real HTTP. The bout row is written at DECLARE time with
// the declarer's hidden die in it; duel-state used to return those rows raw, so
// the responder could read the die from the API and then pick a stake knowing
// whether it would beat it. Invisible in the UI — the panel never printed it —
// so only a test keeps it closed.
//
// While the bout is open: the declarer sees their die, the responder and a
// bystander see null. Once it resolves, everyone sees both dice.
func TestProposeDuelHTTP_DuelState_ResponderCannotReadDeclarersDie(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5
	bystanderIdx := (prepIdx0 + 2) % 5

	// Two stakes each, so the duel is still in its bouts after the first one.
	prepA := h.seedPeer(prepIdx0, "Preparer stake A")
	prepB := h.seedPeer(prepIdx0, "Preparer stake B")
	targA := h.seedPeer(targIdx, "Target stake A")
	targB := h.seedPeer(targIdx, "Target stake B")

	planID, prepIdx := seedDuelToBouts(t, h, targIdx, []int64{prepA, prepB}, []int64{targA, targB})
	// Distinct, memorable faces: the declarer holds 6, the responder 2.
	pinDuelDice(t, h, planID, prepIdx, targIdx, 6, 2)

	// The preparer holds initiative: opens with a stake, calling low.
	declStake := firstUnresolvedStakeID(t, h, planID, h.tg.Players[prepIdx].ID)
	code, body := h.post(prepIdx, duelRoute(planID, "bout-declare"),
		map[string]any{"stake_id": declStake, "declaration": "low"})
	require.Equalf(t, http.StatusOK, code, "bout-declare: %v", body)

	// ── Bout open ──
	rawResp, boutsResp := duelStateAs(t, h, targIdx, planID)
	require.Len(t, boutsResp, 1)
	assert.Nil(t, boutsResp[0]["declarer_die"], "the responder must not be able to read the declarer's die")
	assert.NotContains(t, rawResp, `"declarer_die":6`, "…anywhere in the response body")
	// The call itself is public and does reach them.
	assert.Equal(t, "low", boutsResp[0]["declaration"])
	assert.EqualValues(t, declStake, boutsResp[0]["declarer_stake_id"])

	_, boutsBy := duelStateAs(t, h, bystanderIdx, planID)
	require.Len(t, boutsBy, 1)
	assert.Nil(t, boutsBy[0]["declarer_die"], "nor may a third player at the table")

	_, boutsDecl := duelStateAs(t, h, prepIdx, planID)
	require.Len(t, boutsDecl, 1)
	assert.EqualValues(t, 6, boutsDecl[0]["declarer_die"], "the declarer still sees their own die")

	// ── Resolve it ──
	respStake := firstUnresolvedStakeID(t, h, planID, h.tg.Players[targIdx].ID)
	code, body = h.post(targIdx, duelRoute(planID, "bout-respond"), map[string]any{"stake_id": respStake})
	require.Equalf(t, http.StatusOK, code, "bout-respond: %v", body)

	for name, idx := range map[string]int{"declarer": prepIdx, "responder": targIdx, "bystander": bystanderIdx} {
		_, bouts := duelStateAs(t, h, idx, planID)
		require.Lenf(t, bouts, 1, name)
		assert.EqualValuesf(t, 6, bouts[0]["declarer_die"], "%s: a resolved bout's dice are public", name)
		assert.EqualValuesf(t, 2, bouts[0]["responder_die"], "%s: a resolved bout's dice are public", name)
	}
}

// TestProposeDuelHTTP_BoutDeclarationIsLogged: the call (which stake, high or
// low) is spoken aloud at the table, so it belongs in the shared action log. It
// was written out in full and commented out. The declarer's die is NOT part of
// it — the log is read by the responder too.
func TestProposeDuelHTTP_BoutDeclarationIsLogged(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")
	planID, prepIdx := seedDuelToBouts(t, h, targIdx, []int64{prepPeer}, []int64{targPeer})

	declStake := firstUnresolvedStakeID(t, h, planID, h.tg.Players[prepIdx].ID)
	code, body := h.post(prepIdx, duelRoute(planID, "bout-declare"),
		map[string]any{"stake_id": declStake, "declaration": "high"})
	require.Equalf(t, http.StatusOK, code, "bout-declare: %v", body)

	// Bout still open — nobody has answered — yet the call is already on record.
	line, ok := duelPostWith(duelSystemPosts(t, h.q, h.tg.Game.ID), "opens with")
	require.True(t, ok, "expected a declaration post once the bout is declared")
	assert.Contains(t, line, "Bout 1")
	assert.Contains(t, line, "Preparer stake", "names the stake the bout opens with")
	assert.Contains(t, line, "calling high")
	assert.Contains(t, line, "must answer")
	assert.NotContains(t, line, "rolled", "the die is not revealed until the responder answers")
}

// TestProposeDuelHTTP_SubsetClaimOnAMarStillAsksTheTarget: the counterpart to the
// forced claim. When the mar budget (difficulty 1) is smaller than the
// preparer's pile (2), the target genuinely chooses, so nothing auto-applies, no
// stake is leveraged yet, and the Waiting-On bar names the target.
func TestProposeDuelHTTP_SubsetClaimOnAMarStillAsksTheTarget(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5

	prepA := h.seedPeer(prepIdx0, "Preparer stake A")
	prepB := h.seedPeer(prepIdx0, "Preparer stake B")
	targPeer := h.seedPeer(targIdx, "Target stake")

	planID, _ := seedDuelToRollPinned(t, h, targIdx,
		[]int64{prepA, prepB}, []int64{targPeer}, targetSweepsPrep, targetSweepsTarg)

	status, _ := planStatusOf(t, h, planID)
	require.Equal(t, model.PlanResolving, status, "a real pick must wait for the winner")
	h.assertWaitees("mar subset claim", model.RowStatePlanResolving, h.tg.Players[targIdx].ID)

	ctx := context.Background()
	for _, aid := range []int64{prepA, prepB, targPeer} {
		a, err := h.q.GetAssetByID(ctx, aid)
		require.NoError(t, err)
		require.Falsef(t, a.IsLeveraged, "nothing leverages until the claim is made (asset %d)", aid)
	}

	// The target takes one; the other stake stays with the preparer, leveraged.
	code, body := h.post(targIdx, duelRoute(planID, "make-choice"),
		map[string]any{"result": "mar", "choices": []string{strconv.FormatInt(prepB, 10)}})
	require.Equalf(t, http.StatusOK, code, "target claims: %v", body)

	status, result := planStatusOf(t, h, planID)
	require.Equal(t, model.PlanResolved, status)
	require.Equal(t, "mar", result)

	taken, err := h.q.GetAssetByID(ctx, prepB)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[targIdx].ID, taken.OwnerID)
	kept, err := h.q.GetAssetByID(ctx, prepA)
	require.NoError(t, err)
	require.Equal(t, h.tg.Players[prepIdx0].ID, kept.OwnerID)
	require.True(t, kept.IsLeveraged)
}
