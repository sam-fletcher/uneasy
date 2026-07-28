//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/gametest"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// Integration coverage for the endgame vote (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md
// Session 1): the window guard, the row 7 → 8 advance blocking on BOTH
// rowAdvanceBlockReason callers, all-voted auto-resolution completing the
// deferred advance, and a tied 4-player vote resolving to the facilitator's side.

// voteHarness bundles the router and the per-player session plumbing every case
// below needs. The routes are mounted individually so each test drives exactly
// the endpoints it means to.
type voteHarness struct {
	q       *dbgen.Queries
	store   *db.Store
	manager *hub.Manager
	router  *chi.Mux
	game    dbgen.Game
	players []dbgen.Player
}

func newVoteHarness(t *testing.T, playerCount int, opts ...gametest.Option) *voteHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, playerCount, opts...)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	// A hub must exist for broadcastRowState's auto-kickoff path to run.
	manager.GetOrCreate(tg.Game.ID)

	router := chi.NewRouter()
	router.Use(appMiddleware.EnsureSession(q))
	router.Post("/api/tables/{id}/pass-focus", PassFocus(store, manager))
	router.Post("/api/tables/{id}/refresh-assets", RefreshAssets(store, manager))
	router.Post("/api/tables/{id}/prepare-plan", PreparePlan(store, manager))
	router.Get("/api/tables/{id}/ending-vote", GetEndingVote(store))
	router.Post("/api/tables/{id}/ending-vote", CastEndingVote(store, manager))

	return &voteHarness{
		q: q, store: store, manager: manager, router: router,
		game: tg.Game, players: tg.Players,
	}
}

// as issues a request to path as actor, with a fresh session cookie.
func (h *voteHarness) as(t *testing.T, actor dbgen.Player, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = h.q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: actor.AccountID,
	})
	require.NoError(t, err)

	var reader *bytes.Reader
	if body != nil {
		raw, mErr := json.Marshal(body)
		require.NoError(t, mErr)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func (h *voteHarness) path(suffix string) string {
	return "/api/tables/" + strconv.FormatInt(h.game.ID, 10) + suffix
}

func (h *voteHarness) reload(t *testing.T) dbgen.Game {
	t.Helper()
	g, err := h.q.GetGameByID(context.Background(), h.game.ID)
	require.NoError(t, err)
	return g
}

func (h *voteHarness) rowState(t *testing.T) model.RowState {
	t.Helper()
	rs, err := ComputeRowState(context.Background(), h.q, h.game.ID)
	require.NoError(t, err)
	return rs
}

// hasPostWithCode reports whether the game's chat log holds a post with the
// given system_code, and returns the first such post.
func (h *voteHarness) postWithCode(t *testing.T, code string) *dbgen.ScenePost {
	t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.game.ID)
	require.NoError(t, err)
	for i := range posts {
		if posts[i].SystemCode != nil && *posts[i].SystemCode == code {
			return &posts[i]
		}
	}
	return nil
}

// ── Opening the vote ─────────────────────────────────────────────────────────

// The PassFocus half of "both rowAdvanceBlockReason callers pick the vote up".
// Row 7's advance is the last one that costs the table no information (max fixed
// delay is Host Festivity's 6, and 7 + 6 = 13), so it is where the vote sits.
func TestEndingVote_PassFocusOnRow7_OpensVoteAndBlocksAdvance(t *testing.T) {
	h := newVoteHarness(t, 3, gametest.WithCurrentRow(endgameVoteRow))

	rec := h.as(t, h.players[0], "POST", h.path("/pass-focus"), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "pass-focus failed: %s", rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "the table is voting on how the game ends", resp["advance_blocked"])

	g := h.reload(t)
	assert.True(t, g.EndingVoteOpen, "the row-advance gate opens the vote")
	assert.Equal(t, int16(endgameVoteRow), g.CurrentRow,
		"the advance is what's paused — current_row stays 7 for the vote's whole duration")
	assert.Nil(t, g.EndingMode)

	rs := h.rowState(t)
	assert.Equal(t, model.RowStateAwaitEndgameVote, rs.Kind)
	assert.ElementsMatch(t,
		[]int64{h.players[0].ID, h.players[1].ID, h.players[2].ID},
		rs.ActingPlayerIDs,
		"every seated player owes a vote — the Waiting On bar must name all of them")

	post := h.postWithCode(t, "endgame.vote_opened")
	require.NotNil(t, post, "the vote opening gets a boundary log post")
	assert.Equal(t, model.SeverityBoundary, post.Severity)
	require.NotNil(t, post.RowNumber, "vote posts anchor to row 7 via logRow, not &game.CurrentRow")
	assert.Equal(t, int16(endgameVoteRow), *post.RowNumber)
}

// The autoPassFocus half. RefreshAssets with an empty list is the cheapest
// step-5 action that reaches autoPassFocus, which re-runs the same gate as a
// post-commit side effect.
func TestEndingVote_AutoPassFocusOnRow7_OpensVoteAndBlocksAdvance(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow))

	rec := h.as(t, h.players[0], "POST", h.path("/refresh-assets"),
		map[string]any{"asset_ids": []int64{}})
	require.Equalf(t, http.StatusOK, rec.Code, "refresh-assets failed: %s", rec.Body.String())

	g := h.reload(t)
	assert.True(t, g.EndingVoteOpen,
		"autoPassFocus must pick the vote check up too — there is no third row-advance path")
	assert.Equal(t, int16(endgameVoteRow), g.CurrentRow)
	assert.Equal(t, model.RowStateAwaitEndgameVote, h.rowState(t).Kind)
}

// Opening is idempotent: a second pass over an already-open vote must not
// re-post the boundary narration or otherwise churn.
func TestEndingVote_OpeningIsIdempotent(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow))
	ctx := context.Background()

	require.Equalf(t, http.StatusOK,
		h.as(t, h.players[0], "POST", h.path("/pass-focus"), nil).Code, "first pass")
	// Focus has moved to players[1]; they pass too, hitting the same gate.
	require.Equalf(t, http.StatusOK,
		h.as(t, h.players[1], "POST", h.path("/pass-focus"), nil).Code, "second pass")

	posts, err := h.q.ListGamePosts(ctx, h.game.ID)
	require.NoError(t, err)
	opened := 0
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "endgame.vote_opened" {
			opened++
		}
	}
	assert.Equal(t, 1, opened, "the vote-opened post is written once, not on every pass")
	assert.True(t, h.reload(t).EndingVoteOpen)
}

// A game whose ending mode is already settled has nothing to vote on: row 7
// advances normally. This is also what keeps a dev-seeded game (whose
// DevAdvanceRow deliberately skips the play-state gates) from tripping over the
// vote once a test sets the mode explicitly.
func TestEndingVote_NotOpenedWhenModeAlreadySet(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow))
	ctx := context.Background()

	mode := EndingModeSmoothLanding
	require.NoError(t, h.q.SetEndingMode(ctx, dbgen.SetEndingModeParams{
		ID: h.game.ID, EndingMode: &mode,
	}))

	rec := h.as(t, h.players[0], "POST", h.path("/pass-focus"), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "pass-focus failed: %s", rec.Body.String())

	g := h.reload(t)
	assert.False(t, g.EndingVoteOpen)
	assert.Equal(t, int16(endgameVoteRow+1), g.CurrentRow, "the row advances as normal")
}

// Other rows are untouched — the vote is pinned to one boundary, not "any row
// from which something might overflow".
func TestEndingVote_NotOpenedOnOtherRows(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow-1))

	rec := h.as(t, h.players[0], "POST", h.path("/pass-focus"), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "pass-focus failed: %s", rec.Body.String())

	g := h.reload(t)
	assert.False(t, g.EndingVoteOpen)
	assert.Equal(t, int16(endgameVoteRow), g.CurrentRow)
}

// ── The window guard ─────────────────────────────────────────────────────────

// ending_vote_open is the single window authority: there is no early voting.
func TestEndingVote_RejectedWhileWindowClosed(t *testing.T) {
	h := newVoteHarness(t, 2)

	rec := h.as(t, h.players[0], "POST", h.path("/ending-vote"),
		map[string]any{"mode": EndingModeSmoothLanding})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "not voting on how the game ends")

	votes, err := h.q.ListEndingVotesByGame(context.Background(), h.game.ID)
	require.NoError(t, err)
	assert.Empty(t, votes, "a refused vote must not be recorded")
}

// ...and no late voting either: once the tally has closed the window, a further
// vote is refused rather than silently re-opening the decision.
func TestEndingVote_RejectedAfterWindowCloses(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	for _, p := range h.players {
		require.Equalf(t, http.StatusOK,
			h.as(t, p, "POST", h.path("/ending-vote"),
				map[string]any{"mode": EndingModeExplosiveFinale}).Code,
			"vote failed for player %d", p.ID)
	}
	require.False(t, h.reload(t).EndingVoteOpen)

	rec := h.as(t, h.players[0], "POST", h.path("/ending-vote"),
		map[string]any{"mode": EndingModeSmoothLanding})
	assert.Equal(t, http.StatusConflict, rec.Code)
	g := h.reload(t)
	require.NotNil(t, g.EndingMode)
	assert.Equal(t, EndingModeExplosiveFinale, *g.EndingMode, "the settled mode stands")
}

func TestEndingVote_LongCampaignRejected(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	rec := h.as(t, h.players[0], "POST", h.path("/ending-vote"),
		map[string]any{"mode": EndingModeLongCampaign})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "long_campaign is not yet implemented")
}

func TestEndingVote_UnknownModeRejected(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	rec := h.as(t, h.players[0], "POST", h.path("/ending-vote"),
		map[string]any{"mode": "fireworks"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// No plan may be prepared while the vote is up — not just an overflowing one.
// Focus HAS already passed by the time the vote opens, so without this guard the
// new focus player could slip a plan in behind the vote via a direct API call,
// and it would land relative to row 7 rather than row 8. The guard sits above
// even the notes check, so an otherwise-invalid body still gets the vote's
// answer rather than a misleading one.
func TestEndingVote_PreparePlanRejectedWhileVoteOpen(t *testing.T) {
	h := newVoteHarness(t, 2, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	g := h.reload(t)
	require.NotNil(t, g.FocusPlayerID)
	var focus dbgen.Player
	for _, p := range h.players {
		if p.ID == *g.FocusPlayerID {
			focus = p
		}
	}
	require.NotZero(t, focus.ID, "the vote opens with someone holding focus")

	rec := h.as(t, focus, "POST", h.path("/prepare-plan"), map[string]any{
		"plan_type":         model.PlanSpreadPropaganda,
		"preparation_notes": "slipping one in behind the vote",
	})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "voting on how the game ends")

	plans, err := h.q.ListPlansByGame(context.Background(), h.game.ID)
	require.NoError(t, err)
	assert.Empty(t, plans, "nothing was prepared")
}

// ── Casting, changing, and reading votes ─────────────────────────────────────

func TestEndingVote_PartialBallotWaitsAndNamesTheStragglers(t *testing.T) {
	h := newVoteHarness(t, 3, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	rec := h.as(t, h.players[0], "POST", h.path("/ending-vote"),
		map[string]any{"mode": EndingModeExplosiveFinale})
	require.Equalf(t, http.StatusOK, rec.Code, "vote failed: %s", rec.Body.String())

	var resp endingVoteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Open)
	assert.Nil(t, resp.Mode, "one vote settles nothing")
	require.Len(t, resp.Votes, 1)
	assert.Equal(t, h.players[0].ID, resp.Votes[0].PlayerID)
	assert.Equal(t, EndingModeExplosiveFinale, resp.Votes[0].Mode,
		"votes are public — who voted and for what")
	assert.ElementsMatch(t, []int64{h.players[1].ID, h.players[2].ID}, resp.PendingPlayerIDs)

	g := h.reload(t)
	assert.True(t, g.EndingVoteOpen)
	assert.Nil(t, g.EndingMode)
	assert.Equal(t, int16(endgameVoteRow), g.CurrentRow, "the advance stays paused")

	rs := h.rowState(t)
	assert.Equal(t, model.RowStateAwaitEndgameVote, rs.Kind)
	assert.ElementsMatch(t, []int64{h.players[1].ID, h.players[2].ID}, rs.ActingPlayerIDs,
		"a player who has voted is no longer named")

	post := h.postWithCode(t, "endgame.vote_cast")
	require.NotNil(t, post, "each vote gets a default-severity log post")
	assert.Equal(t, model.SeverityDefault, post.Severity)
	require.NotNil(t, post.RowNumber)
	assert.Equal(t, int16(endgameVoteRow), *post.RowNumber)
}

// A player may change their mind for as long as the window is open — the upsert
// replaces their row rather than adding a second one.
func TestEndingVote_ChangeVoteWhileOpen(t *testing.T) {
	h := newVoteHarness(t, 3, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	require.Equal(t, http.StatusOK,
		h.as(t, h.players[0], "POST", h.path("/ending-vote"),
			map[string]any{"mode": EndingModeExplosiveFinale}).Code)
	rec := h.as(t, h.players[0], "POST", h.path("/ending-vote"),
		map[string]any{"mode": EndingModeSmoothLanding})
	require.Equalf(t, http.StatusOK, rec.Code, "changing a vote failed: %s", rec.Body.String())

	var resp endingVoteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Votes, 1, "a change replaces the vote, it does not add one")
	assert.Equal(t, EndingModeSmoothLanding, resp.Votes[0].Mode)
	assert.Nil(t, resp.Mode, "two players still owe a vote")

	posts, err := h.q.ListGamePosts(context.Background(), h.game.ID)
	require.NoError(t, err)
	var bodies []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "endgame.vote_cast" {
			bodies = append(bodies, p.Body)
		}
	}
	require.Len(t, bodies, 2, "the first vote and the change each get a post")
	assert.Contains(t, bodies[0], "votes for")
	assert.Contains(t, bodies[1], "changes their vote to")
}

// Re-casting the same mode is a no-op, and an impatient double-tap must not spam
// the log with it.
func TestEndingVote_RecastingTheSameModeIsSilent(t *testing.T) {
	h := newVoteHarness(t, 3, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	for range 3 {
		require.Equal(t, http.StatusOK,
			h.as(t, h.players[0], "POST", h.path("/ending-vote"),
				map[string]any{"mode": EndingModeSmoothLanding}).Code)
	}

	posts, err := h.q.ListGamePosts(context.Background(), h.game.ID)
	require.NoError(t, err)
	cast := 0
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "endgame.vote_cast" {
			cast++
		}
	}
	assert.Equal(t, 1, cast, "only the vote that actually changed something is logged")

	votes, err := h.q.ListEndingVotesByGame(context.Background(), h.game.ID)
	require.NoError(t, err)
	require.Len(t, votes, 1)
	assert.Equal(t, EndingModeSmoothLanding, votes[0].Mode)
}

func TestEndingVote_GetIsReadableByAnySeatedPlayer(t *testing.T) {
	h := newVoteHarness(t, 3, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)
	require.Equal(t, http.StatusOK,
		h.as(t, h.players[1], "POST", h.path("/ending-vote"),
			map[string]any{"mode": EndingModeSmoothLanding}).Code)

	// players[2] hasn't voted, and isn't the facilitator.
	rec := h.as(t, h.players[2], "GET", h.path("/ending-vote"), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "GET failed: %s", rec.Body.String())

	var resp endingVoteResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Open)
	require.Len(t, resp.Votes, 1)
	assert.Equal(t, h.players[1].ID, resp.Votes[0].PlayerID)
	assert.Equal(t, EndingModeSmoothLanding, resp.Votes[0].Mode)
	assert.ElementsMatch(t, []int64{h.players[0].ID, h.players[2].ID}, resp.PendingPlayerIDs)
}

// ── Resolution and the deferred advance ──────────────────────────────────────

// The last vote tallies, closes the window, and performs the advance the blocked
// PassFocus would have made — so the "Row 8 begins" post fires in its normal
// order, just later.
func TestEndingVote_AllVotedResolvesAndCompletesDeferredAdvance(t *testing.T) {
	h := newVoteHarness(t, 3, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)
	ctx := context.Background()

	for i, p := range h.players {
		mode := EndingModeExplosiveFinale
		if i == 0 {
			mode = EndingModeSmoothLanding // 2–1 for explosive; clause 1 decides
		}
		rec := h.as(t, p, "POST", h.path("/ending-vote"), map[string]any{"mode": mode})
		require.Equalf(t, http.StatusOK, rec.Code, "vote failed: %s", rec.Body.String())
	}

	g := h.reload(t)
	require.NotNil(t, g.EndingMode)
	assert.Equal(t, EndingModeExplosiveFinale, *g.EndingMode)
	assert.False(t, g.EndingVoteOpen, "the tally closes the window")
	assert.Equal(t, int16(endgameVoteRow+1), g.CurrentRow,
		"resolution performs the deferred row advance inline")

	resolved := h.postWithCode(t, "endgame.mode_set")
	require.NotNil(t, resolved, "the resolution gets a boundary post naming the mode")
	assert.Equal(t, model.SeverityBoundary, resolved.Severity)
	assert.Contains(t, resolved.Body, "Explosive Finale")
	require.NotNil(t, resolved.RowNumber)
	assert.Equal(t, int16(endgameVoteRow), *resolved.RowNumber,
		"the vote's own posts stay on row 7 — the advance is what was paused")

	rowBegins := h.postWithCode(t, "row.advanced")
	require.NotNil(t, rowBegins, "the deferred advance still writes its boundary post")
	require.NotNil(t, rowBegins.RowNumber)
	assert.Equal(t, int16(endgameVoteRow+1), *rowBegins.RowNumber)
	assert.Less(t, resolved.ID, rowBegins.ID,
		"the resolution reads before 'Row 8 begins', which is the whole point of the gap")

	// The vote is over: the row state is back to normal play on row 8.
	assert.NotEqual(t, model.RowStateAwaitEndgameVote, h.rowState(t).Kind)

	// And the mode now actually gates preparation (the ending_vote_open guard is
	// gone, the mode-aware branch is Session 3's).
	votes, err := h.q.ListEndingVotesByGame(ctx, h.game.ID)
	require.NoError(t, err)
	assert.Len(t, votes, 3, "the ballot is kept for the record")
}

// A tie on an even roster: the tied option the facilitator voted for wins. With
// two modes this is the only tie-break that can fire — a tie needs an even split
// down the middle, so the facilitator is necessarily on one side.
func TestEndingVote_TiedFourPlayerVoteGoesToTheFacilitatorsSide(t *testing.T) {
	h := newVoteHarness(t, 4, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	require.True(t, h.players[0].IsFacilitator, "the seed makes player[0] the facilitator")

	modes := []string{
		EndingModeExplosiveFinale, // facilitator
		EndingModeSmoothLanding,
		EndingModeSmoothLanding,
		EndingModeExplosiveFinale,
	}
	for i, p := range h.players {
		rec := h.as(t, p, "POST", h.path("/ending-vote"), map[string]any{"mode": modes[i]})
		require.Equalf(t, http.StatusOK, rec.Code, "vote failed: %s", rec.Body.String())
	}

	g := h.reload(t)
	require.NotNil(t, g.EndingMode)
	assert.Equal(t, EndingModeExplosiveFinale, *g.EndingMode,
		"2–2 breaks to the side the facilitator voted for")
	assert.False(t, g.EndingVoteOpen)
	assert.Equal(t, int16(endgameVoteRow+1), g.CurrentRow)
}

// The mirror of the above, so the test isn't just asserting "explosive wins":
// the same 2–2 split with the facilitator on the other side settles the other
// way.
func TestEndingVote_TiedFourPlayerVote_OtherSide(t *testing.T) {
	h := newVoteHarness(t, 4, gametest.WithCurrentRow(endgameVoteRow))
	h.openVote(t)

	modes := []string{
		EndingModeSmoothLanding, // facilitator
		EndingModeExplosiveFinale,
		EndingModeExplosiveFinale,
		EndingModeSmoothLanding,
	}
	for i, p := range h.players {
		require.Equal(t, http.StatusOK,
			h.as(t, p, "POST", h.path("/ending-vote"), map[string]any{"mode": modes[i]}).Code)
	}

	g := h.reload(t)
	require.NotNil(t, g.EndingMode)
	assert.Equal(t, EndingModeSmoothLanding, *g.EndingMode)
}

// openVote drives the real row-advance path to open the vote, rather than
// writing games.ending_vote_open directly — so every case below is exercising
// the state the gate actually produces (focus already passed, row still 7).
func (h *voteHarness) openVote(t *testing.T) {
	t.Helper()
	g := h.reload(t)
	require.Equal(t, int16(endgameVoteRow), g.CurrentRow,
		"seed the harness with gametest.WithCurrentRow(endgameVoteRow)")
	require.NotNil(t, g.FocusPlayerID)
	var focus dbgen.Player
	for _, p := range h.players {
		if p.ID == *g.FocusPlayerID {
			focus = p
		}
	}
	require.NotZero(t, focus.ID)

	rec := h.as(t, focus, "POST", h.path("/pass-focus"), nil)
	require.Equalf(t, http.StatusOK, rec.Code, "pass-focus failed: %s", rec.Body.String())
	require.True(t, h.reload(t).EndingVoteOpen, "the row-advance gate must have opened the vote")
}
