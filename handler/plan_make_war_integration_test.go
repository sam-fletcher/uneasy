//go:build integration

package handler

// plan_make_war_integration_test.go — end-to-end HTTP coverage for Make War's
// cost-of-battle mechanics and the plan.make_war action-log: the break_asset
// cost (canonical breakMarginalia auto-destroy), surrender + asset seizure,
// and peace proposal → unanimous accept → war end.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// warSystemPosts returns the bodies of all plan.make_war action-log posts.
func warSystemPosts(t *testing.T, q *dbgen.Queries, gameID int64) []string {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "plan.make_war" {
			out = append(out, p.Body)
		}
	}
	return out
}

// anyContains reports whether any string in xs contains sub.
func anyContains(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}

// pinPowerRank forces playerID into a specific power rank by swapping slots
// with the current occupant (the (game,category,rank) row keys the slot).
func pinPowerRank(t *testing.T, h *planLifecycle, playerID int64, target int16) {
	t.Helper()
	ctx := context.Background()
	ranks, err := h.q.ListRankingsByGame(ctx, h.tg.Game.ID)
	require.NoError(t, err)

	var curRank int16
	var occupant *int64
	for _, r := range ranks {
		if r.Category != model.CategoryPower || r.PlayerID == nil {
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
		GameID: h.tg.Game.ID, PlayerID: &pid, Category: model.CategoryPower, Rank: target,
	}))
	if occupant != nil && curRank != 0 {
		require.NoError(t, h.q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
			GameID: h.tg.Game.ID, PlayerID: occupant, Category: model.CategoryPower, Rank: curRank,
		}))
	}
}

// warRoute builds the /api/plans/{id}/{name} extra-route path.
func warRoute(planID int64, name string) string {
	return "/api/plans/" + strconv.FormatInt(planID, 10) + "/" + name
}

// seedActiveWar prepares a Make War from the focus player against the next
// player, pins power ranks so the preparer is up-next to pay cost of battle
// (lowest power = first in reverse power order), submits the delay reveal so
// the war becomes active, and returns the plan id, the preparer/enemy indices,
// and the war-start row.
func seedActiveWar(t *testing.T, h *planLifecycle) (planID int64, prepIdx, enemyIdx int, warStartRow int16) {
	t.Helper()
	prepIdx = h.focusPlayerIdx()
	enemyIdx = (prepIdx + 1) % len(h.tg.Players)
	enemyID := h.tg.Players[enemyIdx].ID

	notes := "The realm shall burn"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeWar,
		EnemyPlayerIDs:   []int64{enemyID},
		PreparationNotes: &notes,
	})

	// Preparer = lowest power → first to pay in reverse power order.
	pinPowerRank(t, h, h.tg.Players[prepIdx].ID, 5)
	pinPowerRank(t, h, enemyID, 1)

	mw := game.LoadMakeWarData(&plan)
	require.NotNil(t, mw.DelayRevealID, "delay reveal should exist after prepare")
	revPath := "/api/reveals/" + strconv.FormatInt(*mw.DelayRevealID, 10) + "/submit"
	for _, idx := range []int{prepIdx, enemyIdx} {
		code, body := h.post(idx, revPath, map[string]any{"face": 2})
		require.Equalf(t, http.StatusOK, code, "reveal submit: %v", body)
	}

	refreshed, err := h.q.GetPlanByID(context.Background(), plan.ID)
	require.NoError(t, err)
	require.NotNil(t, refreshed.RowNumber, "war should have a row after delay reveal")
	return plan.ID, prepIdx, enemyIdx, *refreshed.RowNumber
}

// seedAssetWithMarginalium creates a peer owned by players[ownerIdx] carrying a
// single marginalium, returning the asset id and marginalia id.
func seedAssetWithMarginalium(t *testing.T, h *planLifecycle, ownerIdx int, name string) (int64, int64) {
	t.Helper()
	assetID := h.seedPeer(ownerIdx, name)
	m, err := h.q.CreateMarginalia(context.Background(), dbgen.CreateMarginaliaParams{
		AssetID: assetID, Position: 1, Text: "a note",
	})
	require.NoError(t, err)
	return assetID, m.ID
}

// ── Tests ────────────────────────────────────────────────────────────────────

// TestMakeWarHTTP_BreakAssetCost_DestroysAndLogs: paying the cost of battle via
// break_asset tears the marginalium (auto-destroying the asset when it was the
// last one) and logs the payment; the war-start beat is also logged.
func TestMakeWarHTTP_BreakAssetCost_DestroysAndLogs(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)

	// War-declared beat logged when the delay reveal resolved.
	require.True(t, anyContains(warSystemPosts(t, h.q, h.tg.Game.ID), "War breaks out"),
		"expected a War breaks out log post")

	assetA, margA := seedAssetWithMarginalium(t, h, prepIdx, "Frontline asset")
	h.jumpToRow(warRow + 1) // cost of battle is due the row after the war starts

	enemyID := h.tg.Players[enemyIdx].ID
	code, body := h.post(prepIdx, warRoute(planID, "pay-battle-cost"), map[string]any{
		"opponent_id":   enemyID,
		"choice":        "break_asset",
		"marginalia_id": margA,
	})
	require.Equalf(t, http.StatusOK, code, "pay-battle-cost: %v", body)

	ctx := context.Background()
	a, err := h.q.GetAssetByID(ctx, assetA)
	require.NoError(t, err)
	require.True(t, a.IsDestroyed, "breaking the last marginalium should destroy the asset")

	require.True(t, anyContains(warSystemPosts(t, h.q, h.tg.Game.ID), "broke an asset"),
		"expected a battle-cost log post")
}

// TestMakeWarHTTP_SurrenderSeizeAndEnd: paying with surrender ends the war,
// opens a claim, and the opponent seizes an asset — each beat logged.
func TestMakeWarHTTP_SurrenderSeizeAndEnd(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)

	_, margA := seedAssetWithMarginalium(t, h, prepIdx, "Sacrificed asset")
	seizable := h.seedPeer(prepIdx, "Spoils of war")
	h.jumpToRow(warRow + 1)

	prepID := h.tg.Players[prepIdx].ID
	enemyID := h.tg.Players[enemyIdx].ID

	// Preparer pays (break) and surrenders in the same call.
	code, body := h.post(prepIdx, warRoute(planID, "pay-battle-cost"), map[string]any{
		"opponent_id":   enemyID,
		"choice":        "break_asset",
		"marginalia_id": margA,
		"surrender":     true,
	})
	require.Equalf(t, http.StatusOK, code, "surrender pay: %v", body)

	ctx := context.Background()
	war, err := h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)
	require.Equal(t, "ended", war.Status, "war should end when a side fully surrenders")

	// Enemy seizes the surrendered player's other asset.
	code, body = h.post(enemyIdx, warRoute(planID, "take-surrender-asset"), map[string]any{
		"surrendered_id": prepID,
		"asset_id":       seizable,
	})
	require.Equalf(t, http.StatusOK, code, "take-surrender-asset: %v", body)

	got, err := h.q.GetAssetByID(ctx, seizable)
	require.NoError(t, err)
	require.Equal(t, enemyID, got.OwnerID, "seized asset should move to the claimant")

	posts := warSystemPosts(t, h.q, h.tg.Game.ID)
	require.True(t, anyContains(posts, "surrendered unconditionally"), "expected surrender log")
	require.True(t, anyContains(posts, "seized"), "expected asset-seized log")
	require.True(t, anyContains(posts, "war is over"), "expected war-ended log")
}

// TestMakeWarHTTP_SurrenderWithNoClaimableAssets_RowCanAdvance reproduces the
// surrender-claim soft-lock: a player surrenders while owning zero non-destroyed
// assets (all spent paying the cost of battle), so the opponent's open claim can
// never be fulfilled via take-surrender-asset. The row-advance gate must not
// treat such an unfulfillable claim as outstanding, or the game can never
// advance the row.
func TestMakeWarHTTP_SurrenderWithNoClaimableAssets_RowCanAdvance(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	ctx := context.Background()

	prepID := h.tg.Players[prepIdx].ID
	enemyID := h.tg.Players[enemyIdx].ID

	// Strip the surrendering player down to a single claimable asset: destroy
	// every asset they currently own, then seed one peer carrying a marginalium
	// that the break_asset surrender payment will itself destroy. After paying,
	// the preparer owns zero non-destroyed assets.
	owned, err := h.q.ListAssetsByOwner(ctx, prepID)
	require.NoError(t, err)
	for _, a := range owned {
		require.NoError(t, h.q.DestroyAsset(ctx, a.ID))
	}
	_, margA := seedAssetWithMarginalium(t, h, prepIdx, "Final holding")

	h.jumpToRow(warRow + 1)

	// Preparer pays (break, destroying their last asset) and surrenders. The
	// lone enemy is on the other side, so the war ends and a single surrender
	// claim opens against the preparer.
	code, body := h.post(prepIdx, warRoute(planID, "pay-battle-cost"), map[string]any{
		"opponent_id":   enemyID,
		"choice":        "break_asset",
		"marginalia_id": margA,
		"surrender":     true,
	})
	require.Equalf(t, http.StatusOK, code, "surrender pay: %v", body)

	war, err := h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)
	require.Equal(t, "ended", war.Status, "war should end when a side fully surrenders")

	// The soft-lock trigger: an open (unfulfilled) claim exists for the enemy…
	rawClaims, err := h.q.ListOpenSurrenderClaimsByWar(ctx, war.ID)
	require.NoError(t, err)
	require.Len(t, rawClaims, 1, "enemy should hold one open surrender claim")
	require.Equal(t, enemyID, rawClaims[0].ClaimantID)

	// …but the surrendered player has nothing left to seize.
	n, err := mwClaimableAssetCount(ctx, h.q, prepID)
	require.NoError(t, err)
	require.Zero(t, n, "surrendered player should own no non-destroyed assets")

	// The fix: the row-advance gate must not count an unfulfillable claim as
	// outstanding, so the row can advance.
	outstanding, err := mwOutstandingSurrenderClaimsForGame(ctx, h.q, h.tg.Game.ID)
	require.NoError(t, err)
	require.Empty(t, outstanding,
		"a claim against a player with no claimable assets must not block row advance")

	// …and the war-state view must agree with the gate: the unfulfillable claim
	// is hidden so the panel doesn't render an actionable-looking surrender claim
	// that can never be acted on.
	wsRaw, err := h.q.ListOpenSurrenderClaimsByWar(ctx, war.ID)
	require.NoError(t, err)
	require.Len(t, wsRaw, 1, "underlying row still exists (unfulfilled)")
	ws, err := buildWarState(ctx, h.q, war)
	require.NoError(t, err)
	require.Empty(t, ws.OpenClaims,
		"war-state view must not surface an unfulfillable surrender claim")

	game, err := h.q.GetGameByID(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/", nil)
	newRow, ended, err := advanceRowInner(req, h.q, h.manager, nil, &game)
	require.NoError(t, err, "row should advance despite the unfulfillable claim")
	require.False(t, ended)
	require.Equal(t, warRow+2, newRow, "row should advance past the surrender row")
}

// TestMakeWarHTTP_PeaceProposeAcceptEnds: the up-next payer proposes peace and a
// unanimous accept ends the war, logging both beats.
func TestMakeWarHTTP_PeaceProposeAcceptEnds(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)

	code, body := h.post(prepIdx, warRoute(planID, "propose-peace"), map[string]any{
		"terms": "White peace; both sides withdraw.",
	})
	require.Equalf(t, http.StatusOK, code, "propose-peace: %v", body)
	proposalID := int64(body["proposal_id"].(float64))

	code, body = h.post(enemyIdx, warRoute(planID, "vote-peace"), map[string]any{
		"proposal_id": proposalID,
		"accepted":    true,
	})
	require.Equalf(t, http.StatusOK, code, "vote-peace: %v", body)

	war, err := h.q.GetWarByOriginPlan(context.Background(), planID)
	require.NoError(t, err)
	require.Equal(t, "ended", war.Status, "unanimous accept should end the war")

	posts := warSystemPosts(t, h.q, h.tg.Game.ID)
	require.True(t, anyContains(posts, "proposed peace terms"), "expected peace-proposed log")
	require.True(t, anyContains(posts, "agreed to peace terms"), "expected peace-agreed log")
}

// TestMakeWarHTTP_ContinuingWar_NoCostAgainstSurrenderedOpponent is the
// integration-level regression for the cost-of-battle/surrendered-opponent bug
// (game.MissingBattleCosts). It builds a 2v2 war, surrenders one enemy while
// their ally fights on (war continues), then drives the real DB → mwSnapshotWar
// → MissingBattleCosts path and asserts no active player owes the cost of battle
// against the surrendered player. Pre-fix, the declarer side was charged a cost
// against the surrendered ally.
func TestMakeWarHTTP_ContinuingWar_NoCostAgainstSurrenderedOpponent(t *testing.T) {
	h := newPlanLifecycle(t, 4)
	ctx := context.Background()

	// 1v1 active war via the real prepare + delay-reveal flow.
	planID, prepIdx, enemyIdx, warStartRow := seedActiveWar(t, h)
	war, err := h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)

	// The two unused players become allies, one per side.
	used := map[int]bool{prepIdx: true, enemyIdx: true}
	var allyIdxs []int
	for i := range h.tg.Players {
		if !used[i] {
			allyIdxs = append(allyIdxs, i)
		}
	}
	require.Len(t, allyIdxs, 2)

	// Read the seeded sides so allies join the correct ones.
	parts, err := h.q.ListWarParticipants(ctx, war.ID)
	require.NoError(t, err)
	sideOf := map[int64]int16{}
	for _, p := range parts {
		sideOf[p.PlayerID] = p.Side
	}
	prepSide := sideOf[h.tg.Players[prepIdx].ID]
	enemySide := sideOf[h.tg.Players[enemyIdx].ID]
	require.NotEqual(t, prepSide, enemySide, "preparer and enemy must be on opposite sides")

	declAlly := h.tg.Players[allyIdxs[0]].ID
	enemyAlly := h.tg.Players[allyIdxs[1]].ID
	require.NoError(t, h.q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
		WarID: war.ID, PlayerID: declAlly, Side: prepSide, JoinedAtRow: warStartRow,
	}))
	require.NoError(t, h.q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
		WarID: war.ID, PlayerID: enemyAlly, Side: enemySide, JoinedAtRow: warStartRow,
	}))

	// The enemy ally surrenders; the original enemy stays active → war continues.
	surRow := warStartRow
	require.NoError(t, h.q.SetWarParticipantSurrendered(ctx, dbgen.SetWarParticipantSurrenderedParams{
		WarID: war.ID, PlayerID: enemyAlly, SurrenderedAtRow: &surRow,
	}))

	// Precondition: the surrendered ally is a fully-joined participant (so it
	// appears in snap.Sides) — otherwise the test wouldn't exercise the bug.
	parts, err = h.q.ListWarParticipants(ctx, war.ID)
	require.NoError(t, err)
	var allyJoinedAndSurrendered bool
	for _, p := range parts {
		if p.PlayerID == enemyAlly {
			allyJoinedAndSurrendered = p.EntryPaymentComplete && p.SurrenderedAtRow != nil
		}
	}
	require.True(t, allyJoinedAndSurrendered, "enemy ally must be joined and surrendered")

	// Cost of battle is due the row after the war started.
	costRow := warStartRow + 1
	require.NoError(t, h.q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: h.tg.Game.ID, CurrentRow: costRow,
	}))

	costs, err := mwOutstandingCostsForGame(ctx, h.q, h.tg.Game.ID, costRow)
	require.NoError(t, err)
	keys := costs[war.ID]
	require.NotEmpty(t, keys, "active participants still owe cost of battle in a continuing war")

	// The fix: no one owes a cost against the surrendered ally.
	for _, k := range keys {
		require.NotEqualf(t, enemyAlly, k.OpponentID,
			"no active player should owe cost of battle against the surrendered ally (got %+v)", k)
	}
	// Sanity: the live cross-side opponent is still charged — the preparer owes a
	// cost against the still-active original enemy.
	owedAgainstLiveEnemy := false
	for _, k := range keys {
		if k.PayerID == h.tg.Players[prepIdx].ID && k.OpponentID == h.tg.Players[enemyIdx].ID {
			owedAgainstLiveEnemy = true
		}
	}
	require.True(t, owedAgainstLiveEnemy, "active opponents are still charged the cost of battle")
}
