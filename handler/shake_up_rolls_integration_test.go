//go:build integration

// handler/shake_up_rolls_integration_test.go — Shake-Up Overhaul Session 1:
// integration tests for real server-rolled shake-up dice, driven through the
// actual HTTP roll endpoints (/leverage, /ready) rather than
// gametest.WithShakeUpTokens' direct token grants, which bypass rolling
// entirely. This is the regression harness for the two launch bugs the
// session fixed:
//
//   - ShakeUpRoll's CreateDiceRoll call used Difficulty: 0 without
//     is_shake_up, tripping the CHECK constraint on every call (500).
//   - Even past that, the row never set is_shake_up and never resolved, so
//     it would trip uq_one_open_roll_per_game and haunt GetOpenRollByGame
//     as a phantom "active roll".
//
// Plus the turn-order, leverage-persistence, and banked-die rulings from
// SHAKEUP_RULES.md / adr/SHAKEUP_OVERHAUL_PLAN.md.
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

// shakeUpRollsHarness wraps a shake_up-phase game with an HTTP router wired to
// the roll endpoints + the shake-up snapshot, plus per-player session tokens.
type shakeUpRollsHarness struct {
	seeded  gametest.SeededGame
	q       *dbgen.Queries
	manager *hub.Manager
	router  http.Handler
	tokens  []string // tokens[i] authenticates as seeded.Players[i]
}

// newShakeUpRollsHarness seeds a fresh shake_up game (esteem/rolling, zero
// tokens, all tracks in seat order — see gametest.SeedShakeUp) with n players
// and mounts the roll + shake-up routes actually used to drive step 1.
func newShakeUpRollsHarness(t *testing.T, n int, opts ...gametest.Option) *shakeUpRollsHarness {
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
	r.Get("/api/tables/{id}/rolls/active", GetActiveRollForGame(store))
	r.Route("/api/rolls/{rollId}", func(rr chi.Router) {
		rr.Post("/leverage", LeverageRoll(store, manager))
		rr.Post("/use-banked-die", UseBankedDie(store, manager))
		rr.Post("/ready", SetReady(store, manager))
	})
	return &shakeUpRollsHarness{seeded: seeded, q: q, manager: manager, router: r, tokens: tokens}
}

// do issues an authenticated request as seeded.Players[playerIdx]. body may
// be nil. Returns (status, decoded JSON).
func (h *shakeUpRollsHarness) do(t *testing.T, playerIdx int, method, path string, body any) (int, map[string]any) {
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

func (h *shakeUpRollsHarness) gameID() int64 { return h.seeded.Game.ID }

// activeRoll fetches the game's active roll as playerIdx and requires 200.
func (h *shakeUpRollsHarness) activeRoll(t *testing.T, playerIdx int) map[string]any {
	t.Helper()
	status, body := h.do(t, playerIdx, "GET",
		fmt.Sprintf("/api/tables/%d/rolls/active", h.gameID()), nil)
	require.Equal(t, http.StatusOK, status, body)
	return body
}

// playerIdxByID returns the seeded player index for id, or fails the test.
func (h *shakeUpRollsHarness) playerIdxByID(t *testing.T, id int64) int {
	t.Helper()
	for i, p := range h.seeded.Players {
		if p.ID == id {
			return i
		}
	}
	require.Fail(t, "no seeded player with id %d", id)
	return -1
}

// readyAsActor fetches the active roll and readies it as its actor, driving
// auto-resolution (a shake-up roll's sole participant is the actor, so this
// always resolves on this call — see shakeUpOpenRollForRoller). Returns the
// resolved roll id and the actor's player id.
func (h *shakeUpRollsHarness) readyAsActor(t *testing.T) (rollID, actorID int64) {
	t.Helper()
	active := h.activeRoll(t, 0)
	roll := asMap(t, active, "roll")
	rollID = asInt64(t, roll["id"])
	actorID = asInt64(t, roll["actor_id"])
	actorIdx := h.playerIdxByID(t, actorID)

	status, _ := h.do(t, actorIdx, "POST", fmt.Sprintf("/api/rolls/%d/ready", rollID),
		map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)
	return rollID, actorID
}

// ── Tests ────────────────────────────────────────────────────────────────────

// Regression for both launch bugs: two players roll their esteem step-1 roll
// back-to-back through the real endpoints. Before this session, the first
// roll 500'd on the difficulty CHECK; even patched past that, the second
// roll would have tripped uq_one_open_roll_per_game as a phantom open roll.
func TestShakeUpRoll_RealEndpoints_TwoPlayersBackToBack(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()
	esteem := gamepkg.ShakeUpCategoryEsteem

	// Seat order p0→rank2, p1→rank4 (n=2); reverse-rank turn order is p1, p0.
	active := h.activeRoll(t, 0)
	roll := asMap(t, active, "roll")
	assert.Equal(t, h.seeded.Players[1].ID, asInt64(t, roll["actor_id"]),
		"lowest-status player (p1) rolls first")
	assert.Equal(t, "leverage", roll["stage"])
	assert.Equal(t, true, roll["is_shake_up"])
	assert.Equal(t, esteem, roll["shake_up_category"])
	assert.Equal(t, float64(0), roll["difficulty"], "difficulty sentinel")
	dice := active["dice"].([]any)
	require.Len(t, dice, 2, "2 base dice")

	// P1 rolls.
	rollID1, actor1 := h.readyAsActor(t)
	require.Equal(t, h.seeded.Players[1].ID, actor1)
	resolved1, err := h.q.GetDiceRollByID(ctx, rollID1)
	require.NoError(t, err)
	assert.Equal(t, "resolved", resolved1.Stage)
	require.NotNil(t, resolved1.Result)
	assert.Nil(t, resolved1.Outcome, "shake-up rolls have no make/mar outcome")

	p1, err := h.q.GetPlayerByID(ctx, actor1)
	require.NoError(t, err)
	assert.Equal(t, *resolved1.Result, p1.ShakeUpTokens, "tokens gained == distinct faces")

	// P0's roll should now be open automatically.
	active = h.activeRoll(t, 0)
	roll = asMap(t, active, "roll")
	assert.Equal(t, h.seeded.Players[0].ID, asInt64(t, roll["actor_id"]))
	assert.NotEqual(t, rollID1, asInt64(t, roll["id"]), "a fresh roll, not the same row")

	// Step should still be rolling — only one of two players has rolled.
	game, err := h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	require.NotNil(t, game.ShakeUpStep)
	assert.Equal(t, gamepkg.ShakeUpStepRolling, *game.ShakeUpStep)

	// P0 rolls.
	rollID2, actor2 := h.readyAsActor(t)
	require.Equal(t, h.seeded.Players[0].ID, actor2)
	resolved2, err := h.q.GetDiceRollByID(ctx, rollID2)
	require.NoError(t, err)
	assert.Equal(t, "resolved", resolved2.Stage)

	// Both have rolled → step flips to spending.
	game, err = h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	require.NotNil(t, game.ShakeUpStep)
	assert.Equal(t, gamepkg.ShakeUpStepSpending, *game.ShakeUpStep)

	// No roll left open.
	active = h.activeRoll(t, 0)
	assert.Nil(t, active["roll"])
}

// Turn order: only the actor may act on their own roll. commitGate rejects a
// non-participant even when they hold a perfectly valid asset to spend.
func TestShakeUpRoll_NonActor_LeverageRejected(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()

	active := h.activeRoll(t, 0)
	roll := asMap(t, active, "roll")
	rollID := asInt64(t, roll["id"])
	actorID := asInt64(t, roll["actor_id"])
	nonActorIdx := 1 - h.playerIdxByID(t, actorID)

	assets, err := h.q.ListAssetsByOwner(ctx, h.seeded.Players[nonActorIdx].ID)
	require.NoError(t, err)
	require.NotEmpty(t, assets)

	status, body := h.do(t, nonActorIdx, "POST", fmt.Sprintf("/api/rolls/%d/leverage", rollID),
		map[string]any{"asset_id": assets[0].ID})
	assert.Equal(t, http.StatusForbidden, status, body)
	assert.Contains(t, fmt.Sprint(body["error"]), "not a participant")
}

// Ruling 1: token gain is the count of distinct faces, not their sum. Preset
// both base dice to the same face (3, 3) so the sum (6) and the distinct-face
// count (1) are unambiguously different — a regression guard against
// reintroducing the old self-reported "sum" textbox semantics.
func TestShakeUpRoll_TokensAreDistinctFaces_NotSum(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()

	active := h.activeRoll(t, 0)
	roll := asMap(t, active, "roll")
	rollID := asInt64(t, roll["id"])
	actorID := asInt64(t, roll["actor_id"])

	dice, err := h.q.ListDiceByRoll(ctx, rollID)
	require.NoError(t, err)
	require.Len(t, dice, 2)
	face := int16(3)
	for _, d := range dice {
		require.NoError(t, h.q.SetDieFace(ctx, dbgen.SetDieFaceParams{ID: d.ID, Face: &face}))
	}

	actorIdx := h.playerIdxByID(t, actorID)
	status, _ := h.do(t, actorIdx, "POST", fmt.Sprintf("/api/rolls/%d/ready", rollID),
		map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)

	resolved, err := h.q.GetDiceRollByID(ctx, rollID)
	require.NoError(t, err)
	require.NotNil(t, resolved.Result)
	assert.Equal(t, int16(1), *resolved.Result, "two matching faces == 1 distinct face, not sum 6")

	player, err := h.q.GetPlayerByID(ctx, actorID)
	require.NoError(t, err)
	assert.Equal(t, int16(1), player.ShakeUpTokens)
}

// Ruling 4: leverage is real and persistent across categories — nothing
// refreshes assets between esteem/knowledge/power. An asset leveraged on the
// esteem roll must still read as leveraged (and be rejected for re-leverage)
// once the knowledge category's rolling step opens.
func TestShakeUpRoll_LeveragePersistsAcrossCategories(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()

	// P1 (first roller) leverages an asset on their esteem roll.
	active := h.activeRoll(t, 0)
	roll := asMap(t, active, "roll")
	rollID := asInt64(t, roll["id"])
	actorID := asInt64(t, roll["actor_id"])
	actorIdx := h.playerIdxByID(t, actorID)

	assets, err := h.q.ListAssetsByOwner(ctx, actorID)
	require.NoError(t, err)
	require.NotEmpty(t, assets)
	stakedAsset := assets[0]

	status, body := h.do(t, actorIdx, "POST", fmt.Sprintf("/api/rolls/%d/leverage", rollID),
		map[string]any{"asset_id": stakedAsset.ID})
	require.Equal(t, http.StatusOK, status, body)

	leveraged, err := h.q.GetAssetByID(ctx, stakedAsset.ID)
	require.NoError(t, err)
	require.True(t, leveraged.IsLeveraged)

	// Finish esteem: this roller readies (3 dice now), then the other rolls.
	status, _ = h.do(t, actorIdx, "POST", fmt.Sprintf("/api/rolls/%d/ready", rollID),
		map[string]any{"is_ready": true})
	require.Equal(t, http.StatusOK, status)
	h.readyAsActor(t) // the other player's esteem roll

	// The asset survives the roll unrefreshed.
	stillLeveraged, err := h.q.GetAssetByID(ctx, stakedAsset.ID)
	require.NoError(t, err)
	require.True(t, stillLeveraged.IsLeveraged, "leverage persists past the roll that spent it")

	// Drain esteem's tokens and advance straight to knowledge (spending-step
	// mechanics are Session 2's concern; jump the category boundary directly,
	// same as the existing currentShakeUpActor/effects tests do).
	require.NoError(t, h.q.ZeroShakeUpTokens(ctx, h.gameID()))
	require.NoError(t, maybeAdvanceShakeUpCategory(ctx, h.q, h.manager, h.gameID()))

	game, err := h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	require.NotNil(t, game.ShakeUpCategory)
	assert.Equal(t, gamepkg.ShakeUpCategoryKnowledge, *game.ShakeUpCategory)

	// Knowledge's first roller is the same player (rankings default to seat
	// order on every track) — attempting to leverage the still-leveraged asset
	// again must be rejected.
	active = h.activeRoll(t, 0)
	roll = asMap(t, active, "roll")
	knowledgeRollID := asInt64(t, roll["id"])
	require.Equal(t, actorID, asInt64(t, roll["actor_id"]), "same seat-order roller for knowledge")

	status, body = h.do(t, actorIdx, "POST", fmt.Sprintf("/api/rolls/%d/leverage", knowledgeRollID),
		map[string]any{"asset_id": stakedAsset.ID})
	assert.Equal(t, http.StatusConflict, status, body)
	assert.Contains(t, fmt.Sprint(body["error"]), "already leveraged")
}

// Ruling 3: banked dice are not spendable on shake-up rolls — assets only.
func TestShakeUpRoll_BankedDieRejected(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()

	active := h.activeRoll(t, 0)
	roll := asMap(t, active, "roll")
	rollID := asInt64(t, roll["id"])
	actorID := asInt64(t, roll["actor_id"])
	actorIdx := h.playerIdxByID(t, actorID)

	banked, err := h.q.CreateBankedDie(ctx, dbgen.CreateBankedDieParams{
		GameID: h.gameID(), PlayerID: actorID, Source: "liaise",
	})
	require.NoError(t, err)

	status, body := h.do(t, actorIdx, "POST", fmt.Sprintf("/api/rolls/%d/use-banked-die", rollID),
		map[string]any{"banked_die_id": banked.ID})
	assert.Equal(t, http.StatusConflict, status, body)
	assert.Contains(t, fmt.Sprint(body["error"]), "Shake-Up")
}

// The game ends once every category's tokens have drained after power. Drives
// all three categories' rolls through the real endpoints (spending is
// short-circuited the same way as the leverage-persistence test above — it's
// Session 2's concern) and asserts the phase transitions to ended with no
// dangling roll or roller.
func TestShakeUpRoll_AllCategoriesDrain_PhaseEnds(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()

	for _, cat := range []string{
		gamepkg.ShakeUpCategoryEsteem, gamepkg.ShakeUpCategoryKnowledge, gamepkg.ShakeUpCategoryPower,
	} {
		game, err := h.q.GetGameByID(ctx, h.gameID())
		require.NoError(t, err)
		require.Equal(t, model.PhaseShakeUp, game.Phase, "category %s: still in shake-up", cat)
		require.NotNil(t, game.ShakeUpCategory)
		require.Equal(t, cat, *game.ShakeUpCategory)

		h.readyAsActor(t) // p1 (or whoever is first this category)
		h.readyAsActor(t) // the other player

		require.NoError(t, h.q.ZeroShakeUpTokens(ctx, h.gameID()))
		require.NoError(t, maybeAdvanceShakeUpCategory(ctx, h.q, h.manager, h.gameID()))
	}

	game, err := h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	assert.Equal(t, model.PhaseEnded, game.Phase)

	active := h.activeRoll(t, 0)
	assert.Nil(t, active["roll"], "no roll should remain open once the game has ended")
}

// A game with dummy tokens (n=2, dummies occupy ranks 1/3/5 — see
// game.DummyRanks) must never create a roll for a dummy slot: exactly 2
// rolls total (one per real player) should resolve the esteem category, and
// the step must advance to spending without a third roll ever appearing.
func TestShakeUpRoll_DummiesNeverRoll(t *testing.T) {
	h := newShakeUpRollsHarness(t, 2)
	ctx := context.Background()

	seenActors := map[int64]bool{}
	for range 2 {
		_, actorID := h.readyAsActor(t)
		require.False(t, seenActors[actorID], "each real player rolls exactly once")
		seenActors[actorID] = true
	}
	assert.Len(t, seenActors, 2)
	for _, p := range h.seeded.Players {
		assert.True(t, seenActors[p.ID], "every real player rolled")
	}

	game, err := h.q.GetGameByID(ctx, h.gameID())
	require.NoError(t, err)
	require.NotNil(t, game.ShakeUpStep)
	assert.Equal(t, gamepkg.ShakeUpStepSpending, *game.ShakeUpStep,
		"step advances once the (only) real players have rolled — no roll ever waited on a dummy")

	active := h.activeRoll(t, 0)
	assert.Nil(t, active["roll"], "no dummy roll left dangling open")
}
