//go:build integration

// handler/shake_up_reaction_gate_integration_test.go — Shake-Up Overhaul
// Session 2: integration tests for the spending-step reaction gate (ruling
// 5 — the spender cannot rush the commit; every other token-holding player
// must adjust or explicitly pass first) and take-target validation (ruling
// 8 — takes cannot target your own assets), driven through the real HTTP
// announce/adjust/pass/commit endpoints.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/gametest"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// ── harness ──────────────────────────────────────────────────────────────────

// shakeUpSpendHarness wraps a shake_up-phase game with an HTTP router wired to
// the spending-step endpoints (announce/adjust/pass/commit) plus the shake-up
// snapshot, and per-player session tokens.
type shakeUpSpendHarness struct {
	seeded  gametest.SeededGame
	q       *dbgen.Queries
	manager *hub.Manager
	router  http.Handler
	tokens  []string // tokens[i] authenticates as seeded.Players[i]
}

// newShakeUpSpendHarness seeds a fresh shake_up game with n players and mounts
// the spending-step routes. Callers typically pass
// gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending) and
// gametest.WithShakeUpTokens(n) to land directly in the spending step with a
// token pool to spend, mirroring newShakeUpGame's usage elsewhere.
func newShakeUpSpendHarness(t *testing.T, n int, opts ...gametest.Option) *shakeUpSpendHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	seeded := newShakeUpGame(t, q, n, opts...)
	store := db.NewStore(pool)
	manager := hub.NewManager()

	tokens := make([]string, n)
	for i, p := range seeded.Players {
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
			Token: tok, AccountID: p.AccountID,
		})
		require.NoError(t, err)
		tokens[i] = tok
	}

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Get("/api/tables/{id}/shake-up", GetShakeUp(store))
	r.Post("/api/tables/{id}/shake-up/spend", ShakeUpAnnounce(store, manager))
	r.Post("/api/tables/{id}/shake-up/adjust", ShakeUpAdjust(store, manager))
	r.Post("/api/tables/{id}/shake-up/pass", ShakeUpPass(store, manager))
	r.Post("/api/tables/{id}/shake-up/commit", ShakeUpCommit(store, manager))
	return &shakeUpSpendHarness{seeded: seeded, q: q, manager: manager, router: r, tokens: tokens}
}

// do issues an authenticated request as seeded.Players[playerIdx]. body may
// be nil. Returns (status, decoded JSON).
func (h *shakeUpSpendHarness) do(t *testing.T, playerIdx int, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func (h *shakeUpSpendHarness) gameID() int64 { return h.seeded.Game.ID }

// snapshot fetches GET /shake-up as playerIdx and requires 200.
func (h *shakeUpSpendHarness) snapshot(t *testing.T, playerIdx int) map[string]any {
	t.Helper()
	status, body := h.do(t, playerIdx, "GET", fmt.Sprintf("/api/tables/%d/shake-up", h.gameID()), nil)
	require.Equal(t, http.StatusOK, status, body)
	return body
}

// currentActor reads GetShakeUp's current_actor field (only meaningful when
// no spend is open) and returns the seeded player index.
func (h *shakeUpSpendHarness) currentActorIdx(t *testing.T) int {
	t.Helper()
	snap := h.snapshot(t, 0)
	actorID := asInt64(t, snap["current_actor"])
	return h.playerIdxByID(t, actorID)
}

func (h *shakeUpSpendHarness) playerIdxByID(t *testing.T, id int64) int {
	t.Helper()
	for i, p := range h.seeded.Players {
		if p.ID == id {
			return i
		}
	}
	require.Fail(t, "no seeded player with id %d", id)
	return -1
}

// announce issues a spend announcement as playerIdx and requires 200,
// returning the decoded spend id.
func (h *shakeUpSpendHarness) announce(t *testing.T, playerIdx int, body map[string]any) int64 {
	t.Helper()
	status, resp := h.do(t, playerIdx, "POST", fmt.Sprintf("/api/tables/%d/shake-up/spend", h.gameID()), body)
	require.Equal(t, http.StatusOK, status, resp)
	spend := asMap(t, resp, "spend")
	return asInt64(t, spend["id"])
}

// pass issues a pass as playerIdx and requires 200.
func (h *shakeUpSpendHarness) pass(t *testing.T, playerIdx int, spendID int64) {
	t.Helper()
	status, resp := h.do(t, playerIdx, "POST", fmt.Sprintf("/api/tables/%d/shake-up/pass", h.gameID()),
		map[string]any{"spend_id": spendID})
	require.Equal(t, http.StatusOK, status, resp)
}

// commit issues a commit as playerIdx, returning (status, body) for the
// caller to assert on (tests exercise both the 200 and 409 cases).
func (h *shakeUpSpendHarness) commit(t *testing.T, playerIdx int, spendID int64) (int, map[string]any) {
	t.Helper()
	return h.do(t, playerIdx, "POST", fmt.Sprintf("/api/tables/%d/shake-up/commit", h.gameID()),
		map[string]any{"spend_id": spendID})
}

// openSpendPayload fetches GetShakeUp's open_spend object as playerIdx.
func (h *shakeUpSpendHarness) openSpendPayload(t *testing.T, playerIdx int) map[string]any {
	t.Helper()
	snap := h.snapshot(t, playerIdx)
	return asMap(t, snap, "open_spend")
}

// pendingReactorIDs decodes open_spend.pending_reactor_ids into a []int64.
func pendingReactorIDs(t *testing.T, openSpend map[string]any) []int64 {
	t.Helper()
	raw, ok := openSpend["pending_reactor_ids"].([]any)
	require.True(t, ok, "expected pending_reactor_ids to be an array, got %T", openSpend["pending_reactor_ids"])
	out := make([]int64, len(raw))
	for i, v := range raw {
		out[i] = asInt64(t, v)
	}
	return out
}

// sumShakeUpTokens totals every player's current token pool.
func sumShakeUpTokens(t *testing.T, q *dbgen.Queries, gameID int64) int {
	t.Helper()
	tokens, err := q.ListShakeUpTokensByGame(context.Background(), gameID)
	require.NoError(t, err)
	total := 0
	for _, tk := range tokens {
		total += int(tk.ShakeUpTokens)
	}
	return total
}

// ── Tests ────────────────────────────────────────────────────────────────────

// TestShakeUpReactionGate_FullAuctionFlow pins ruling 5 end to end: an
// announced spend can't be committed until every other token-holding player
// has explicitly passed; once they have, the commit succeeds.
func TestShakeUpReactionGate_FullAuctionFlow(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))

	spenderIdx := h.currentActorIdx(t)
	var otherIdxs []int
	for i := range h.seeded.Players {
		if i != spenderIdx {
			otherIdxs = append(otherIdxs, i)
		}
	}
	require.Len(t, otherIdxs, 2)

	spendID := h.announce(t, spenderIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})

	// commit_ready is false and both others are pending right after announce.
	open := h.openSpendPayload(t, spenderIdx)
	assert.Equal(t, false, open["commit_ready"])
	assert.ElementsMatch(t, []int64{h.seeded.Players[otherIdxs[0]].ID, h.seeded.Players[otherIdxs[1]].ID},
		pendingReactorIDs(t, open))

	// The spender cannot rush the commit.
	status, body := h.commit(t, spenderIdx, spendID)
	assert.Equal(t, http.StatusConflict, status, body)
	assert.Contains(t, fmt.Sprint(body["error"]), "waiting on")

	// First reactor passes — still blocked (one pending reactor left).
	h.pass(t, otherIdxs[0], spendID)
	open = h.openSpendPayload(t, spenderIdx)
	assert.Equal(t, false, open["commit_ready"])
	assert.Equal(t, []int64{h.seeded.Players[otherIdxs[1]].ID}, pendingReactorIDs(t, open))

	status, body = h.commit(t, spenderIdx, spendID)
	assert.Equal(t, http.StatusConflict, status, body)

	// Second reactor passes — commit unlocks.
	h.pass(t, otherIdxs[1], spendID)
	open = h.openSpendPayload(t, spenderIdx)
	assert.Equal(t, true, open["commit_ready"])
	assert.Empty(t, pendingReactorIDs(t, open))

	status, body = h.commit(t, spenderIdx, spendID)
	assert.Equal(t, http.StatusOK, status, body)
}

// TestShakeUpReactionGate_AdjustResetsPasses pins that any new cost
// adjustment reopens the reaction window for everyone — including a reactor
// who had already passed — so a stale pass can't smuggle a later, different
// cost through the gate.
func TestShakeUpReactionGate_AdjustResetsPasses(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))

	spenderIdx := h.currentActorIdx(t)
	var otherIdxs []int
	for i := range h.seeded.Players {
		if i != spenderIdx {
			otherIdxs = append(otherIdxs, i)
		}
	}
	a, b := otherIdxs[0], otherIdxs[1]

	spendID := h.announce(t, spenderIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})

	h.pass(t, a, spendID)
	h.pass(t, b, spendID)
	open := h.openSpendPayload(t, spenderIdx)
	require.Equal(t, true, open["commit_ready"], "both reactors passed")

	// b adjusts the cost — this must wipe every pass, including a's, so both
	// must react again even though a never touched this adjustment.
	status, resp := h.do(t, b, "POST", fmt.Sprintf("/api/tables/%d/shake-up/adjust", h.gameID()),
		map[string]any{"spend_id": spendID, "adjustment": 1})
	require.Equal(t, http.StatusOK, status, resp)

	open = h.openSpendPayload(t, spenderIdx)
	assert.Equal(t, false, open["commit_ready"], "adjustment must reset both passes")
	assert.ElementsMatch(t, []int64{h.seeded.Players[a].ID, h.seeded.Players[b].ID}, pendingReactorIDs(t, open))

	status, body := h.commit(t, spenderIdx, spendID)
	assert.Equal(t, http.StatusConflict, status, body, "commit blocked again until everyone re-passes")

	// Re-pass (including b, the adjuster) unlocks it again.
	h.pass(t, a, spendID)
	h.pass(t, b, spendID)
	status, body = h.commit(t, spenderIdx, spendID)
	assert.Equal(t, http.StatusOK, status, body)
}

// TestShakeUpReactionGate_ZeroTokenExemption pins that a player at 0 tokens
// never blocks a commit — neither on the spend where they spent their last
// token adjusting, nor on any later spend, without ever having to pass.
func TestShakeUpReactionGate_ZeroTokenExemption(t *testing.T) {
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
	drained, holds := otherIdxs[0], otherIdxs[1]

	// Bring `drained` down to exactly 1 token so their next adjustment spends
	// their last one.
	drainedID := h.seeded.Players[drained].ID
	require.NoError(t, h.q.ZeroShakeUpTokens(ctx, h.gameID()))
	_, err := h.q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{ID: drainedID, ShakeUpTokens: 1})
	require.NoError(t, err)
	for _, idx := range []int{spenderIdx, holds} {
		_, err = h.q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{ID: h.seeded.Players[idx].ID, ShakeUpTokens: 5})
		require.NoError(t, err)
	}

	spendID1 := h.announce(t, spenderIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})

	// `drained` spends their last token adjusting.
	status, resp := h.do(t, drained, "POST", fmt.Sprintf("/api/tables/%d/shake-up/adjust", h.gameID()),
		map[string]any{"spend_id": spendID1, "adjustment": 1})
	require.Equal(t, http.StatusOK, status, resp)
	freshTokens, err := h.q.GetShakeUpTokens(ctx, drainedID)
	require.NoError(t, err)
	require.EqualValues(t, 0, freshTokens, "the adjustment spent their last token")

	// `drained` is now exempt from the gate on THIS spend without ever passing.
	open := h.openSpendPayload(t, spenderIdx)
	assert.Equal(t, []int64{h.seeded.Players[holds].ID}, pendingReactorIDs(t, open),
		"the drained player must not appear as a pending reactor")

	h.pass(t, holds, spendID1)
	status, body := h.commit(t, spenderIdx, spendID1)
	require.Equal(t, http.StatusOK, status, body)

	// A LATER spend: `drained` still holds 0 tokens and must be exempt again,
	// with no pass row of their own on this new spend either. Turn order
	// advances past the just-committed spender, so re-derive whose turn it is
	// rather than assuming the same announcer — the exemption is what's under
	// test, not who happens to hold the turn.
	actor2Idx := h.currentActorIdx(t)
	require.NotEqual(t, drained, actor2Idx, "the drained player has no tokens and can never hold the turn")
	spendID2 := h.announce(t, actor2Idx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})
	open = h.openSpendPayload(t, actor2Idx)
	var wantPending []int64
	for i := range h.seeded.Players {
		if i != actor2Idx && i != drained {
			wantPending = append(wantPending, h.seeded.Players[i].ID)
		}
	}
	assert.ElementsMatch(t, wantPending, pendingReactorIDs(t, open))

	for i := range h.seeded.Players {
		if i != actor2Idx && i != drained {
			h.pass(t, i, spendID2)
		}
	}
	status, body = h.commit(t, actor2Idx, spendID2)
	assert.Equal(t, http.StatusOK, status, body)
}

// TestShakeUpTakeTarget_OwnAssetRejectedAtAnnounce pins ruling 8: takes
// cannot target your own assets — rejected server-side at announce.
func TestShakeUpTakeTarget_OwnAssetRejectedAtAnnounce(t *testing.T) {
	h := newShakeUpSpendHarness(t, 2,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	spenderID := h.seeded.Players[spenderIdx].ID
	assets, err := h.q.ListAssetsByOwner(ctx, spenderID)
	require.NoError(t, err)
	var ownPeer dbgen.Asset
	for _, a := range assets {
		if a.AssetType == model.AssetPeer {
			ownPeer = a
			break
		}
	}
	require.NotZero(t, ownPeer.ID, "seeded player must own a peer asset")

	status, body := h.do(t, spenderIdx, "POST", fmt.Sprintf("/api/tables/%d/shake-up/spend", h.gameID()),
		map[string]any{"option_key": gamepkg.ShakeUpOptTakePeer, "target_asset_id": ownPeer.ID})
	assert.Equal(t, http.StatusForbidden, status, body)
	assert.Contains(t, fmt.Sprint(body["error"]), "own asset")

	// No spend was created — the actor's turn is untouched, and their token
	// pool wasn't charged.
	fresh, err := h.q.GetPlayerByID(ctx, spenderID)
	require.NoError(t, err)
	assert.EqualValues(t, 5, fresh.ShakeUpTokens, "a rejected announce must not charge the base cost")
}

// TestShakeUpTakeTarget_OwnAssetRejectedAtCommit pins the authoritative
// commit-time recheck in shakeUpTakeAsset: even a spend that names the
// spender's own asset (bypassing announce validation, e.g. a stale announce
// after a swap) must be rejected when applied, leaving the asset untouched.
func TestShakeUpTakeTarget_OwnAssetRejectedAtCommit(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	seeded := newShakeUpGame(t, q, 2)
	gameID := seeded.Game.ID
	playerID := seeded.Players[0].ID

	assets, err := q.ListAssetsByOwner(ctx, playerID)
	require.NoError(t, err)
	var ownPeer dbgen.Asset
	for _, a := range assets {
		if a.AssetType == model.AssetPeer {
			ownPeer = a
			break
		}
	}
	require.NotZero(t, ownPeer.ID)

	spend := &dbgen.ShakeUpSpend{
		OptionKey:     gamepkg.ShakeUpOptTakePeer,
		PlayerID:      playerID,
		TargetAssetID: &ownPeer.ID,
	}
	err = applyShakeUpEffect(ctx, q, manager, gameID, spend, 1)
	require.Error(t, err, "taking your own asset must be rejected even at commit time")

	got, err := q.GetAssetByID(ctx, ownPeer.ID)
	require.NoError(t, err)
	assert.Equal(t, playerID, got.OwnerID, "the asset must not have moved")
}

// TestShakeUpTakeMainCharacterPeer_DoesNotWedgePhase pins ruling 7: taking a
// main-character peer succeeds and does not wedge the phase — no
// MC-replacement gate runs during shake_up (ComputeRowState is
// main_event-only), so the game keeps advancing categories and ends normally
// even though the old owner is left without a main character.
func TestShakeUpTakeMainCharacterPeer_DoesNotWedgePhase(t *testing.T) {
	h := newShakeUpSpendHarness(t, 2,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(5))
	ctx := context.Background()

	spenderIdx := h.currentActorIdx(t)
	targetIdx := 1 - spenderIdx
	targetID := h.seeded.Players[targetIdx].ID

	assets, err := h.q.ListAssetsByOwner(ctx, targetID)
	require.NoError(t, err)
	var mc dbgen.Asset
	for _, a := range assets {
		if a.AssetType == model.AssetPeer && a.IsMainCharacter {
			mc = a
			break
		}
	}
	require.NotZero(t, mc.ID, "target must have a main character to take")

	spendID := h.announce(t, spenderIdx, map[string]any{
		"option_key": gamepkg.ShakeUpOptTakePeer, "target_asset_id": mc.ID,
	})
	h.pass(t, targetIdx, spendID)
	status, body := h.commit(t, spenderIdx, spendID)
	require.Equal(t, http.StatusOK, status, body)

	got, err := h.q.GetAssetByID(ctx, mc.ID)
	require.NoError(t, err)
	assert.Equal(t, h.seeded.Players[spenderIdx].ID, got.OwnerID, "asset transfers to the spender")
	assert.False(t, got.IsMainCharacter, "TransferAsset clears the flag on a cross-owner move")

	// The old owner now holds zero main characters. In main_event this would
	// trip RowStateAwaitMainCharacterChoice; in shake_up nothing gates on it —
	// draining the category and advancing must work exactly as it does for any
	// other take.
	require.NoError(t, h.q.ZeroShakeUpTokens(ctx, h.gameID()))
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, h.q, h.manager, h.gameID()))
	game, err := h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	require.NotNil(t, game.ShakeUpCategory)
	assert.Equal(t, gamepkg.ShakeUpCategoryKnowledge, *game.ShakeUpCategory, "category advanced past esteem")

	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, h.q, h.manager, h.gameID()))
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, h.q, h.manager, h.gameID()))
	game, err = h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	assert.Equal(t, model.PhaseEnded, game.Phase, "the game ends normally despite the MC-less player")
}

// TestShakeUpAuction_TokensStrictlyDecrease is a termination sanity check:
// across a full auction (repeated announce -> react -> commit cycles), the
// total token pool strictly decreases every single commit, so a category can
// never spin forever — it always drains to zero.
func TestShakeUpAuction_TokensStrictlyDecrease(t *testing.T) {
	h := newShakeUpSpendHarness(t, 3,
		gametest.WithShakeUpStep(gamepkg.ShakeUpStepSpending), gametest.WithShakeUpTokens(4))

	total := sumShakeUpTokens(t, h.q, h.gameID())
	require.Equal(t, 12, total)

	rounds := 0
	for ; rounds < 50; rounds++ {
		snap := h.snapshot(t, 0)
		actorRaw, ok := snap["current_actor"]
		if !ok || actorRaw == nil {
			break // category drained — no one holds tokens
		}
		actorIdx := h.playerIdxByID(t, asInt64(t, actorRaw))

		before := sumShakeUpTokens(t, h.q, h.gameID())
		spendID := h.announce(t, actorIdx, map[string]any{"option_key": gamepkg.ShakeUpOptBumpKnowledge})

		// On the first round, exercise the adjust path too (a negative
		// adjustment, so the refund/burn edge case gets covered) before
		// everyone passes — adjusting resets passes, so it must happen first
		// or the later passes would be wiped out from under it.
		if rounds == 0 {
			adjusterIdx := (actorIdx + 1) % len(h.seeded.Players)
			status, resp := h.do(t, adjusterIdx, "POST", fmt.Sprintf("/api/tables/%d/shake-up/adjust", h.gameID()),
				map[string]any{"spend_id": spendID, "adjustment": -1})
			require.Equal(t, http.StatusOK, status, resp)
		}
		for i, p := range h.seeded.Players {
			if p.ID == h.seeded.Players[actorIdx].ID {
				continue
			}
			h.pass(t, i, spendID)
		}

		status, body := h.commit(t, actorIdx, spendID)
		require.Equal(t, http.StatusOK, status, body)

		after := sumShakeUpTokens(t, h.q, h.gameID())
		assert.Less(t, after, before, "round %d: total tokens must strictly decrease", rounds)
	}
	require.Less(t, rounds, 50, "auction must terminate well within the bound")
	assert.Equal(t, 0, sumShakeUpTokens(t, h.q, h.gameID()), "category fully drains")
}
