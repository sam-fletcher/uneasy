//go:build integration

// handler/shake_up_pay_abandon_integration_test.go — ADR-008 ("pay or
// abandon"): integration tests for the resolution branch at commit time
// (extra == 0 auto-commit, extra > 0 affordable pay/abandon, extra >
// remaining pool forced abandon) and the adjust-time cost floor, driven
// through the real HTTP announce/adjust/pass/commit endpoints.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/gametest"
	"uneasy/model"
)

// setShakeUpTokens drains every player in the game to zero, then grants each
// seeded player the amount named at their index — a precise per-player token
// board for tests that need specific affordability edges, mirroring the
// drain-then-grant pattern already used by the reaction-gate tests.
func setShakeUpTokens(t *testing.T, q *dbgen.Queries, gameID int64, players []dbgen.Player, amounts []int16) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, q.ZeroShakeUpTokens(ctx, gameID))
	for i, amt := range amounts {
		if amt == 0 {
			continue
		}
		_, err := q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{ID: players[i].ID, ShakeUpTokens: amt})
		require.NoError(t, err)
	}
}

// lastAbandonedPost returns the body + decoded "forced" field of the most
// recent shake_up.abandoned post, or fails the test if none exists. Decodes
// system_data rather than substring-matching it — Postgres's jsonb text
// output isn't guaranteed to match Go's compact json.Marshal spacing.
func lastAbandonedPost(t *testing.T, q *dbgen.Queries, gameID int64) (body string, forced bool) {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	for i := len(posts) - 1; i >= 0; i-- {
		if posts[i].SystemCode != nil && *posts[i].SystemCode == "shake_up.abandoned" {
			var data struct {
				Forced bool `json:"forced"`
			}
			require.NoError(t, json.Unmarshal(posts[i].SystemData, &data))
			return posts[i].Body, data.Forced
		}
	}
	require.Fail(t, "no shake_up.abandoned post found")
	return "", false
}

// hasCommittedPost reports whether a shake_up.committed post exists for the
// game (used to assert its ABSENCE on an abandoned spend).
func hasCommittedPost(t *testing.T, q *dbgen.Queries, gameID int64) bool {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "shake_up.committed" {
			return true
		}
	}
	return false
}

// targetPeerAsset returns a peer asset owned by playerID (there is always at
// least one straight out of seeding).
func targetPeerAsset(t *testing.T, q *dbgen.Queries, playerID int64) dbgen.Asset {
	t.Helper()
	assets, err := q.ListAssetsByOwner(context.Background(), playerID)
	require.NoError(t, err)
	for _, a := range assets {
		if a.AssetType == model.AssetPeer {
			return a
		}
	}
	require.Fail(t, "player %d has no peer asset", playerID)
	return dbgen.Asset{}
}

// ── Adjust-time cost floor (ADR-008 §1) ─────────────────────────────────────

// TestShakeUpAdjust_CostFloorRejectsBelowOne pins that a -1 adjustment while
// the running cost already sits at 1 (the base, untouched) is rejected
// server-side, and — critically — rejected BEFORE the adjuster is charged,
// so a bounced adjustment costs nothing.
func TestShakeUpAdjust_CostFloorRejectsBelowOne(t *testing.T) {
	h := newShakeUpSpendHarness(t, 2,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	otherIdx := 1 - spenderIdx
	otherID := h.seeded.Players[otherIdx].ID

	spendID := h.announce(t, spenderIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})

	status, body := h.adjust(t, otherIdx, spendID, -1)
	assert.Equal(t, http.StatusConflict, status, body)
	assert.Contains(t, fmt.Sprint(body["error"]), "cost can't go below 1")

	fresh, err := h.q.GetPlayerByID(ctx, otherID)
	require.NoError(t, err)
	assert.EqualValues(t, 5, fresh.ShakeUpTokens, "a rejected adjustment must not charge the bidder")

	open := h.openSpendPayload(t, spenderIdx)
	assert.Empty(t, open["adjustments"], "no adjustment row should have been recorded")
}

// ── extra == 0: auto-commit, intent ignored (no back-out) ──────────────────

// TestShakeUpCommit_NoAbandonWhenExtraIsZero pins that when the cost was
// never raised, the spend auto-commits exactly as before — even if the
// spender's request names intent "abandon", since "must spend it" forbids
// regret-based withdrawal once nothing has changed.
func TestShakeUpCommit_NoAbandonWhenExtraIsZero(t *testing.T) {
	h := newShakeUpSpendHarness(t, 2,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	otherIdx := 1 - spenderIdx

	spendID := h.announce(t, spenderIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})
	h.pass(t, otherIdx, spendID)

	status, body := h.commitWithIntent(t, spenderIdx, spendID, "abandon")
	require.Equal(t, http.StatusOK, status, body, "extra == 0 must auto-commit regardless of intent")
	assert.Equal(t, "committed", body["outcome"])

	spend, err := h.q.GetShakeUpSpend(ctx, spendID)
	require.NoError(t, err)
	assert.True(t, spend.CommittedAt.Valid)
	assert.False(t, spend.AbandonedAt.Valid)
	assert.True(t, spend.Applied)
	assert.True(t, hasCommittedPost(t, h.q, h.gameID()))
}

// ── extra > 0, affordable: pay or abandon is the spender's real choice ─────

// TestShakeUpCommit_AffordablePay pins the "pay" branch: the spender can
// afford the raise, chooses to pay it, and the effect applies at the raised
// cost.
func TestShakeUpCommit_AffordablePay(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	var otherIdxs []int
	for i := range h.seeded.Players {
		if i != spenderIdx {
			otherIdxs = append(otherIdxs, i)
		}
	}
	targetIdx := otherIdxs[0]
	targetID := h.seeded.Players[targetIdx].ID
	targetAsset := targetPeerAsset(t, h.q, targetID)

	spendID := h.announce(t, spenderIdx, map[string]any{
		"option_key": gamepkg.ShakeUpOptTakePeer, "target_asset_id": targetAsset.ID,
	})

	status, resp := h.adjust(t, otherIdxs[1], spendID, 1)
	require.Equal(t, http.StatusOK, status, resp)
	h.pass(t, targetIdx, spendID)
	h.pass(t, otherIdxs[1], spendID)

	spenderID := h.seeded.Players[spenderIdx].ID
	before, err := h.q.GetPlayerByID(ctx, spenderID)
	require.NoError(t, err)
	require.EqualValues(t, 4, before.ShakeUpTokens, "base cost already deducted at announce")

	status, body := h.commitWithIntent(t, spenderIdx, spendID, "pay")
	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, "committed", body["outcome"])
	assert.EqualValues(t, 2, body["final_cost"])

	after, err := h.q.GetPlayerByID(ctx, spenderID)
	require.NoError(t, err)
	assert.EqualValues(t, 3, after.ShakeUpTokens, "the extra token must be charged on top of the base cost")

	got, err := h.q.GetAssetByID(ctx, targetAsset.ID)
	require.NoError(t, err)
	assert.Equal(t, spenderID, got.OwnerID, "paying must apply the take effect")

	spend, err := h.q.GetShakeUpSpend(ctx, spendID)
	require.NoError(t, err)
	assert.True(t, spend.Applied)
	assert.True(t, spend.CommittedAt.Valid)
}

// TestShakeUpCommit_AffordableAbandon pins the "abandon" branch: the spender
// COULD afford the raise but declines — no further charge, no effect, and the
// spend closes terminally abandoned (not forced).
func TestShakeUpCommit_AffordableAbandon(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	var otherIdxs []int
	for i := range h.seeded.Players {
		if i != spenderIdx {
			otherIdxs = append(otherIdxs, i)
		}
	}
	targetIdx := otherIdxs[0]
	targetID := h.seeded.Players[targetIdx].ID
	targetAsset := targetPeerAsset(t, h.q, targetID)

	spendID := h.announce(t, spenderIdx, map[string]any{
		"option_key": gamepkg.ShakeUpOptTakePeer, "target_asset_id": targetAsset.ID,
	})
	status, resp := h.adjust(t, otherIdxs[1], spendID, 1)
	require.Equal(t, http.StatusOK, status, resp)
	h.pass(t, targetIdx, spendID)
	h.pass(t, otherIdxs[1], spendID)

	spenderID := h.seeded.Players[spenderIdx].ID
	status, body := h.commitWithIntent(t, spenderIdx, spendID, "abandon")
	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, "abandoned", body["outcome"])
	assert.Equal(t, false, body["forced"], "the spender could afford it — this is a voluntary abandon")

	after, err := h.q.GetPlayerByID(ctx, spenderID)
	require.NoError(t, err)
	assert.EqualValues(t, 4, after.ShakeUpTokens, "abandon must not charge the extra beyond the base cost")

	got, err := h.q.GetAssetByID(ctx, targetAsset.ID)
	require.NoError(t, err)
	assert.Equal(t, targetID, got.OwnerID, "abandon must not apply the take effect")

	spend, err := h.q.GetShakeUpSpend(ctx, spendID)
	require.NoError(t, err)
	assert.False(t, spend.Applied)
	assert.False(t, spend.CommittedAt.Valid)
	assert.True(t, spend.AbandonedAt.Valid)

	postBody, forced := lastAbandonedPost(t, h.q, h.gameID())
	assert.Contains(t, postBody, "abandons")
	assert.False(t, forced)

	// The abandoned spend must not read as "open" — the auction must not
	// wedge — and it still consumed the announcer's turn.
	_, err = h.q.GetOpenShakeUpSpend(ctx, h.gameID())
	assert.Error(t, err, "an abandoned spend must not appear as the open spend")
}

// ── extra > remaining pool: forced abandon is the only resolution ──────────

// TestShakeUpCommit_ForcedAbandon pins that when the raise exceeds the
// spender's remaining pool, a "pay" request is rejected outright (never
// capped) and only "abandon" succeeds, marked forced.
func TestShakeUpCommit_ForcedAbandon(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	var otherIdxs []int
	for i := range h.seeded.Players {
		if i != spenderIdx {
			otherIdxs = append(otherIdxs, i)
		}
	}
	raiserIdx, passerIdx := otherIdxs[0], otherIdxs[1]

	// Spender gets exactly 2 tokens (1 left after the base cost); the others
	// keep plenty to raise twice and still react.
	amounts := make([]int16, 3)
	amounts[spenderIdx] = 2
	amounts[raiserIdx] = 5
	amounts[passerIdx] = 5
	setShakeUpTokens(t, h.q, h.gameID(), h.seeded.Players, amounts)

	targetID := h.seeded.Players[raiserIdx].ID
	targetAsset := targetPeerAsset(t, h.q, targetID)

	spendID := h.announce(t, spenderIdx, map[string]any{
		"option_key": gamepkg.ShakeUpOptTakePeer, "target_asset_id": targetAsset.ID,
	})
	spenderID := h.seeded.Players[spenderIdx].ID
	afterAnnounce, err := h.q.GetPlayerByID(ctx, spenderID)
	require.NoError(t, err)
	require.EqualValues(t, 1, afterAnnounce.ShakeUpTokens, "1 left after paying the base cost")

	// Raise the cost by 2 (running cost 1 -> 3, extra = 2), which exceeds the
	// spender's remaining 1 token.
	status, resp := h.adjust(t, raiserIdx, spendID, 1)
	require.Equal(t, http.StatusOK, status, resp)
	status, resp = h.adjust(t, raiserIdx, spendID, 1)
	require.Equal(t, http.StatusOK, status, resp)
	h.pass(t, raiserIdx, spendID)
	h.pass(t, passerIdx, spendID)

	open := h.openSpendPayload(t, spenderIdx)
	require.Equal(t, true, open["commit_ready"])

	// "pay" must be rejected outright, not capped, and must not charge
	// anything.
	status, body := h.commitWithIntent(t, spenderIdx, spendID, "pay")
	assert.Equal(t, http.StatusConflict, status, body)
	assert.Contains(t, fmt.Sprint(body["error"]), "cannot afford")

	stillOpen, err := h.q.GetOpenShakeUpSpend(ctx, h.gameID())
	require.NoError(t, err, "a rejected pay must leave the spend open")
	assert.Equal(t, spendID, stillOpen.ID)

	unchanged, err := h.q.GetPlayerByID(ctx, spenderID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, unchanged.ShakeUpTokens, "a rejected pay must not charge the spender")

	// Only "abandon" is left, and it must be marked forced.
	status, body = h.commitWithIntent(t, spenderIdx, spendID, "abandon")
	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, "abandoned", body["outcome"])
	assert.Equal(t, true, body["forced"], "the raise exceeded the spender's pool — this must be forced")

	got, err := h.q.GetAssetByID(ctx, targetAsset.ID)
	require.NoError(t, err)
	assert.Equal(t, targetID, got.OwnerID, "forced abandon must not apply the take effect")

	final, err := h.q.GetPlayerByID(ctx, spenderID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, final.ShakeUpTokens, "the spender is never drained below what they agreed to")

	_, forced := lastAbandonedPost(t, h.q, h.gameID())
	assert.True(t, forced)
}

// TestShakeUpCommit_AbandonAdvancesTurnAndCategory pins that an abandoned
// spend still consumed the announcer's turn: currentShakeUpActor moves on to
// the next player in reverse-rank order exactly as it does after a commit,
// and the category still drains and advances once every pool hits zero.
func TestShakeUpCommit_AbandonAdvancesTurnAndCategory(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(1))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	var otherIdxs []int
	for i := range h.seeded.Players {
		if i != spenderIdx {
			otherIdxs = append(otherIdxs, i)
		}
	}

	// Everyone has exactly 1 token: the spender pays it at announce, leaving
	// them 0 — any raise is automatically unaffordable, forcing an abandon
	// without needing a separate adjuster setup.
	spendID := h.announce(t, spenderIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})
	status, resp := h.adjust(t, otherIdxs[0], spendID, 1)
	require.Equal(t, http.StatusOK, status, resp)
	h.pass(t, otherIdxs[0], spendID)
	h.pass(t, otherIdxs[1], spendID)

	status, body := h.commitWithIntent(t, spenderIdx, spendID, "abandon")
	require.Equal(t, http.StatusOK, status, body)
	assert.Equal(t, true, body["forced"])

	// Turn order: the spender is now at 0 tokens (never drained further) and
	// otherIdxs[0] spent their only token adjusting, so only otherIdxs[1]
	// still holds one — they must be next.
	nextActorIdx := h.currentActorIdx(t)
	assert.Equal(t, otherIdxs[1], nextActorIdx, "turn must advance past the abandoned spender")

	// Drain the last holder and confirm the category still advances. Still in
	// esteem, so the option must be one of esteem's four.
	spendID2 := h.announce(t, otherIdxs[1], map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})
	status, body = h.commit(t, otherIdxs[1], spendID2)
	require.Equal(t, http.StatusOK, status, body)

	game, err := h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	require.NotNil(t, game.ShakeUpCategory)
	assert.Equal(t, gamepkg.ShakeUpCategoryKnowledge, *game.ShakeUpCategory,
		"category must advance past esteem once every pool is empty, abandoned spend included")
}
