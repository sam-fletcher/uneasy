//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
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

// TestResolveTrack_EsteemFourPlayerNoSetAsideEntersClosing is the
// characterization test for the esteem-stall bug: in a 4-5 player game,
// when every player has at least one natural spade (so the esteem track
// resolves with zero set-asides), resolveTrack's nextStep == "" branch used
// to leave games.prologue_ranking_step as NULL — a hard stall no endpoint
// could recover from, since PlaceSetAsides's advanceToMainEvent call was
// never reached and no other handler moves a NULL-step prologue forward.
//
// After the closing-stage rewire, this same board must land on the
// `closing` step instead of NULL.
func TestResolveTrack_EsteemFourPlayerNoSetAsideEntersClosing(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)

	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepDeclareEsteem
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: tg.Game.ID, PrologueRankingStep: &step,
	}))

	// Every player holds one natural spade (esteem's suit) — nobody has zero
	// cards on the track, so ComputeTrackRankingFromCommitments produces no
	// set-asides at all.
	spadeValues := []string{"A", "2", "3", "4"}
	for i, p := range tg.Players {
		require.NoError(t, q.InsertPlayerCard(ctx, dbgen.InsertPlayerCardParams{
			GameID: tg.Game.ID, PlayerID: p.ID, CardSuit: "S", CardValue: spadeValues[i],
		}))
	}

	router := chi.NewRouter()
	router.Use(appMiddleware.EnsureSession(q))
	router.Post("/api/tables/{id}/prologue/done", SetPrologueDone(store, manager))
	path := "/api/tables/" + strconv.FormatInt(tg.Game.ID, 10) + "/prologue/done"

	postDone := func(t *testing.T, actor dbgen.Player) *httptest.ResponseRecorder {
		t.Helper()
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{
			Token: tok, AccountID: actor.AccountID,
		})
		require.NoError(t, err)

		raw, err := json.Marshal(map[string]any{"track": "esteem", "done": true})
		require.NoError(t, err)
		req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// The first three players marking done must not resolve the track yet.
	for _, p := range tg.Players[:3] {
		rec := postDone(t, p)
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	}
	fresh, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.NotNil(t, fresh.PrologueRankingStep)
	require.Equal(t, gamepkg.PrologueStepDeclareEsteem, *fresh.PrologueRankingStep, "track resolves only once every player is done")

	// The last player's done flag resolves the track. With zero set-asides
	// this used to leave prologue_ranking_step NULL (the stall) instead of
	// advancing to the closing step.
	rec := postDone(t, tg.Players[3])
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	fresh, err = q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.NotNilf(t, fresh.PrologueRankingStep, "esteem resolved with zero set-asides must not strand the game at a NULL step")
	require.Equal(t, gamepkg.PrologueStepClosing, *fresh.PrologueRankingStep)
	require.Equal(t, model.PhasePrologue, fresh.Phase, "closing is a gated step, not an instant advance to main_event")
}

// TestPlaceSetAsides_LastTrackEntersClosing_NotInstantAdvance covers the
// other path into the closing step: PlaceSetAsides finishing the last
// track (esteem) with multiple set-asides. Before the rewire, a 4-5 player
// game hitting this branch called advanceToMainEvent immediately, with no
// beat at all for the other players. Now every player count must land on
// closing and wait for the ready gate.
func TestPlaceSetAsides_LastTrackEntersClosing_NotInstantAdvance(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)

	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepPlaceSetAsidesEsteem
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: tg.Game.ID, PrologueRankingStep: &step,
	}))

	// Esteem board: dummy at rank 3 (4p), the real top player at rank 1,
	// the other three players still unranked (the set-asides the top
	// player is about to place).
	require.NoError(t, q.DeleteRankingsByCategory(ctx, dbgen.DeleteRankingsByCategoryParams{
		GameID: tg.Game.ID, Category: model.CategoryEsteem,
	}))
	topPlayer := tg.Players[0]
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: &topPlayer.ID, Category: model.CategoryEsteem, Rank: 1,
	}))
	require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
		GameID: tg.Game.ID, PlayerID: nil, Category: model.CategoryEsteem, Rank: 3,
	}))

	router := chi.NewRouter()
	router.Use(appMiddleware.EnsureSession(q))
	router.Post("/api/tables/{id}/prologue/place-set-asides", PlaceSetAsides(store, manager))
	path := "/api/tables/" + strconv.FormatInt(tg.Game.ID, 10) + "/prologue/place-set-asides"

	ordering := []int64{tg.Players[1].ID, tg.Players[2].ID, tg.Players[3].ID}
	rec := postJSON(t, q, router, path, topPlayer, map[string]any{"ordering": ordering})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	fresh, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.NotNil(t, fresh.PrologueRankingStep)
	assert.Equal(t, gamepkg.PrologueStepClosing, *fresh.PrologueRankingStep)
	assert.Equal(t, model.PhasePrologue, fresh.Phase,
		"a 4-5 player game finishing the last track's set-asides must wait at closing, not auto-advance")
}

// ── Shared closing-stage test helpers ───────────────────────────────────────

// moveGameToClosing parks a seeded game (normally SeedMainEvent's
// main_event phase) at prologue/closing, as if ranking had just resolved.
// Only safe for tests that never actually reach advanceToMainEvent — the
// underlying newTestGame/SeedMainEvent seed already wrote
// public_record_rows once, and a second write collides. Tests that do
// expect to advance use newClosingGame instead.
func moveGameToClosing(t *testing.T, q *dbgen.Queries, gameID int64) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: gameID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepClosing
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: gameID, PrologueRankingStep: &step,
	}))
}

// newClosingGame builds an n-player game already parked at prologue/closing
// with real (non-placeholder) main-character names and full rankings across
// all three tracks — everything advanceToMainEvent needs — but, unlike
// newTestGame/SeedMainEvent, no public_record_rows/current_row/focus_player
// pre-seeded. Tests that drive the ready gate all the way to an actual
// advance use this instead of newTestGame, since SeedMainEvent's own
// public_record_rows seed would collide with advanceToMainEvent's.
func newClosingGame(t *testing.T, q *dbgen.Queries, n int) (dbgen.Game, []dbgen.Player) {
	t.Helper()
	ctx := context.Background()

	game, err := q.CreateGame(ctx, "CLOS"+randSuffix())
	require.NoError(t, err)

	players := make([]dbgen.Player, n)
	for i := range n {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username:     fmt.Sprintf("clos-p%d-%s", i+1, randSuffix()),
			PasswordHash: "x",
		})
		require.NoError(t, err)
		p, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   fmt.Sprintf("P%d", i+1),
			AccountID:     acct.ID,
			IsFacilitator: i == 0,
		})
		require.NoError(t, err)
		seat := int16(i + 1)
		require.NoError(t, q.SetPlayerSeatOrder(ctx, dbgen.SetPlayerSeatOrderParams{
			ID: p.ID, SeatOrder: &seat,
		}))
		p.SeatOrder = &seat
		players[i] = p

		mc, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          game.ID,
			OwnerID:         p.ID,
			CreatorID:       p.ID,
			AssetType:       model.AssetPeer,
			Name:            p.DisplayName + "'s main character",
			IsMainCharacter: true,
		})
		require.NoError(t, err)
		// One note per asset, for the same reason the MC carries a real name:
		// this fixture exists to drive the ready gate through to an advance, and
		// the gate refuses Ready while any owned asset is blank.
		_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: mc.ID, Position: 1, Text: "Bears the weight of expectation.",
		})
		require.NoError(t, err)
	}
	require.NoError(t, q.SetFacilitator(ctx, dbgen.SetFacilitatorParams{
		FacilitatorID: &players[0].ID, ID: game.ID,
	}))

	open := gamepkg.OpenRanks(n)
	dummies := gamepkg.DummyRanks(n)
	for _, cat := range []model.RankingCategory{model.CategoryPower, model.CategoryKnowledge, model.CategoryEsteem} {
		for pos, p := range players {
			pid := p.ID
			require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID: game.ID, PlayerID: &pid, Category: cat, Rank: open[pos],
			}))
		}
		for _, rank := range dummies {
			require.NoError(t, q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID: game.ID, PlayerID: nil, Category: cat, Rank: rank,
			}))
		}
	}

	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepClosing
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: game.ID, PrologueRankingStep: &step,
	}))

	fresh, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	return fresh, players
}

// giveEveryAssetOneMarginalia stamps position 1 of every live asset in a game
// that has none, so the closing gate's blank-asset condition passes. The
// counterpart of gametest.WithStartingMarginalia for fixtures that need to
// start blank and become valid mid-test.
func giveEveryAssetOneMarginalia(t *testing.T, q *dbgen.Queries, gameID int64) {
	t.Helper()
	ctx := context.Background()
	assets, err := q.ListAssetsByGame(ctx, gameID)
	require.NoError(t, err)
	for i := range assets {
		existing, err := q.ListMarginaliaByAsset(ctx, assets[i].ID)
		require.NoError(t, err)
		if len(existing) > 0 {
			continue
		}
		_, err = q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: assets[i].ID, Position: 1, Text: "A note in the margin.",
		})
		require.NoError(t, err)
	}
}

// setMainCharacterName overwrites a player's main-character asset name
// directly (bypassing UpdateAsset), for setting up placeholder/named
// fixtures without exercising the rename endpoint itself.
func setMainCharacterName(t *testing.T, q *dbgen.Queries, gameID, playerID int64, name string) {
	t.Helper()
	ctx := context.Background()
	mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: gameID, OwnerID: playerID,
	})
	require.NoError(t, err)
	require.NoError(t, q.UpdateAssetName(ctx, dbgen.UpdateAssetNameParams{ID: mc.ID, Name: name}))
}

// closingRouter mounts the two closing-stage endpoints under test.
func closingRouter(store *db.Store, manager *hub.Manager) http.Handler {
	router := chi.NewRouter()
	router.Use(appMiddleware.EnsureSession(store.Q))
	router.Post("/api/tables/{id}/prologue/closing-ready", ClosingReady(store, manager))
	router.Post("/api/tables/{id}/prologue/extra-peer", CreateExtraPeer(store, manager))
	return router
}

// postJSON authenticates as actor (a fresh session cookie per call, mirroring
// the other prologue integration tests) and POSTs body as JSON.
func postJSON(
	t *testing.T, q *dbgen.Queries, router http.Handler, path string, actor dbgen.Player, body any,
) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: tok, AccountID: actor.AccountID})
	require.NoError(t, err)

	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func closingReadyPath(gameID int64) string {
	return "/api/tables/" + strconv.FormatInt(gameID, 10) + "/prologue/closing-ready"
}

func extraPeerPath(gameID int64) string {
	return "/api/tables/" + strconv.FormatInt(gameID, 10) + "/prologue/extra-peer"
}

func closingReadyMap(t *testing.T, q *dbgen.Queries, gameID int64) map[int64]bool {
	t.Helper()
	rows, err := q.ListClosingReadyByGame(context.Background(), gameID)
	require.NoError(t, err)
	out := make(map[int64]bool, len(rows))
	for _, r := range rows {
		out[r.PlayerID] = r.Ready
	}
	return out
}

// ── Ready gate ───────────────────────────────────────────────────────────────

func TestClosingReady_PlaceholderNameRefused(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4) // 4p: no extra-peer requirement, isolates the name gate
	moveGameToClosing(t, q, tg.Game.ID)
	setMainCharacterName(t, q, tg.Game.ID, tg.Players[0].ID, model.MainCharacterPlaceholder)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := closingRouter(store, manager)

	rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "name your main character first")

	ready := closingReadyMap(t, q, tg.Game.ID)
	assert.False(t, ready[tg.Players[0].ID], "gate failure must not record a ready row")

	fresh, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhasePrologue, fresh.Phase)
}

func TestClosingReady_MissingExtraPeerRefused(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3) // ≤3p: extra peer required; MC name is already valid from the seed
	moveGameToClosing(t, q, tg.Game.ID)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := closingRouter(store, manager)

	rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "create your extra peer first")
}

// TestClosingReady_BlankAssetRefused covers the blank-asset hard gate
// (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D2). Seeded assets carry no
// marginalia by default — exactly the state a real prologue leaves card assets
// in — and a blank asset is invulnerable, so the closing step is where they get
// closed out.
func TestClosingReady_BlankAssetRefused(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4) // 4p: no extra-peer requirement; seeded MC names are already valid
	moveGameToClosing(t, q, tg.Game.ID)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := closingRouter(store, manager)

	rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "give every asset at least one marginalia first")

	ready := closingReadyMap(t, q, tg.Game.ID)
	assert.False(t, ready[tg.Players[0].ID], "gate failure must not record a ready row")

	fresh, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhasePrologue, fresh.Phase)
}

// TestClosingReady_AllowedOnceEveryAssetHasMarginalia is the other half of the
// blank gate: the same player, same game, passes as soon as every asset they
// own carries a note.
func TestClosingReady_AllowedOnceEveryAssetHasMarginalia(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 4)
	moveGameToClosing(t, q, tg.Game.ID)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := closingRouter(store, manager)

	rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	require.Equal(t, http.StatusConflict, rec.Code, "precondition: the seeded assets start blank")

	giveEveryAssetOneMarginalia(t, q, tg.Game.ID)

	rec = postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.True(t, closingReadyMap(t, q, tg.Game.ID)[tg.Players[0].ID])
}

// TestClosingReady_BlankAssetReportedAfterTheOtherHardItems pins the gate's
// message order: a player who fails several conditions at once is told about
// the main character (and, in small games, the extra peer) first. The
// blank-asset item is the broadest and least specific, so it goes last —
// closingReadyGateFailure and readyBlockedReason (closing.ts) must agree.
func TestClosingReady_BlankAssetReportedAfterTheOtherHardItems(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3) // ≤3p so both the name and extra-peer items are in play
	moveGameToClosing(t, q, tg.Game.ID)
	setMainCharacterName(t, q, tg.Game.ID, tg.Players[0].ID, model.MainCharacterPlaceholder)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := closingRouter(store, manager)

	// All three conditions fail. The name comes first.
	rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "name your main character first")

	// Name fixed; the extra peer now outranks the blank assets.
	setMainCharacterName(t, q, tg.Game.ID, tg.Players[0].ID, "Lady Ashcombe")
	rec = postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "create your extra peer first")

	// Extra peer created (it carries its title marginalia, so it is never
	// blank itself); only the blank starting assets remain.
	rec = postJSON(t, q, router, extraPeerPath(tg.Game.ID), tg.Players[0], map[string]any{
		"title_name": "The Spymaster", "peer_text": "A quiet informant",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	rec = postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "give every asset at least one marginalia first")
}

func TestClosingReady_UnreadyAlwaysAllowedDespiteGateFailure(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 4)
	moveGameToClosing(t, q, tg.Game.ID)
	setMainCharacterName(t, q, tg.Game.ID, tg.Players[0].ID, model.MainCharacterPlaceholder)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := closingRouter(store, manager)

	rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": false})
	assert.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// ── All-ready advance ────────────────────────────────────────────────────────

func TestClosingReady_AllReadyAdvancesToMainEvent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	game, players := newClosingGame(t, q, 4) // valid MC names; no extra-peer requirement at 4p

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(game.ID)
	router := closingRouter(store, manager)

	for _, p := range players[:3] {
		rec := postJSON(t, q, router, closingReadyPath(game.ID), p, map[string]any{"ready": true})
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	}
	fresh, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	require.Equal(t, model.PhasePrologue, fresh.Phase, "must not advance until every player is ready")

	rec := postJSON(t, q, router, closingReadyPath(game.ID), players[3], map[string]any{"ready": true})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	fresh, err = q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseMainEvent, fresh.Phase)
	assert.Equal(t, int16(1), fresh.CurrentRow)
	require.NotNil(t, fresh.FocusPlayerID, "focus player must be set")
	assert.Nil(t, fresh.PrologueRankingStep, "the ranking step clears once the game advances")

	rows, err := q.ListPublicRecordRows(ctx, game.ID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rows), 13)

	posts, err := q.ListGamePosts(ctx, game.ID)
	require.NoError(t, err)
	found := false
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "phase.changed" && p.Body == "Main event begins" {
			found = true
			break
		}
	}
	assert.True(t, found, "phase.changed boundary post for main_event must be emitted")
}

// TestClosingReady_AdvanceTimeRevalidation_ClearsStaleReady covers the
// integrity backstop: there is no un-ready hook on UpdateAsset, so a player
// could rename their main character back to the placeholder after already
// readying. The all-ready check must catch this at advance time, clear only
// that player's ready flag, and refuse to advance.
func TestClosingReady_AdvanceTimeRevalidation_ClearsStaleReady(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4, gametest.WithStartingMarginalia())
	moveGameToClosing(t, q, tg.Game.ID)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := closingRouter(store, manager)

	// Players 1-3 ready up validly first.
	for _, p := range tg.Players[1:] {
		rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), p, map[string]any{"ready": true})
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	}

	// The last-to-ready player quietly goes stale: their MC gets renamed
	// back to the placeholder outside the ready toggle entirely.
	staleReadyPlayer := tg.Players[3]
	setMainCharacterName(t, q, tg.Game.ID, staleReadyPlayer.ID, model.MainCharacterPlaceholder)

	// Player 0 is the last to ready — this request completes the all-ready
	// condition and triggers re-validation.
	rec := postJSON(t, q, router, closingReadyPath(tg.Game.ID), tg.Players[0], map[string]any{"ready": true})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	fresh, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhasePrologue, fresh.Phase, "a stale ready flag must block the advance")

	ready := closingReadyMap(t, q, tg.Game.ID)
	assert.False(t, ready[staleReadyPlayer.ID], "the stale player's ready flag is cleared, not silently ignored")
	assert.True(t, ready[tg.Players[0].ID])
	assert.True(t, ready[tg.Players[1].ID])
	assert.True(t, ready[tg.Players[2].ID])
}

// TestCreateExtraPeer_CompletesAllReadyCondition_Advances exercises the
// shared all-ready helper from the CreateExtraPeer call site: if every
// player already carries a ready=true row, the last player's extra-peer
// creation — the final hard-gate condition for ≤3p games — must itself
// trigger the advance rather than requiring a redundant ready re-toggle.
func TestCreateExtraPeer_CompletesAllReadyCondition_Advances(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	game, players := newClosingGame(t, q, 3)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(game.ID)
	router := closingRouter(store, manager)

	// Players 0 and 1 create their extra peer (distinct titles — each may
	// only be claimed once) and ready up normally.
	titles := []string{"The General", "The Lawyer"}
	for i, p := range players[:2] {
		rec := postJSON(t, q, router, extraPeerPath(game.ID), p, map[string]any{
			"title_name": titles[i], "peer_text": p.DisplayName + "'s retainer",
		})
		require.Equalf(t, http.StatusOK, rec.Code, "extra-peer body: %s", rec.Body.String())
		rec = postJSON(t, q, router, closingReadyPath(game.ID), p, map[string]any{"ready": true})
		require.Equalf(t, http.StatusOK, rec.Code, "ready body: %s", rec.Body.String())
	}
	// Player 2's ready flag is set directly, simulating the only way it can
	// be true before their own peer exists: the server never accepts that
	// combination through the endpoint itself (this is exactly the
	// integrity gap the shared all-ready check exists to close from the
	// other direction — this test isolates the CreateExtraPeer call site).
	require.NoError(t, q.SetClosingReady(ctx, dbgen.SetClosingReadyParams{
		GameID: game.ID, PlayerID: players[2].ID, Ready: true,
	}))

	fresh, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	require.Equal(t, model.PhasePrologue, fresh.Phase, "not advanced yet: player 2 has no extra peer")

	rec := postJSON(t, q, router, extraPeerPath(game.ID), players[2], map[string]any{
		"title_name": "The Spymaster", "peer_text": "A quiet informant",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	fresh, err = q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseMainEvent, fresh.Phase,
		"creating the last missing extra peer completed the all-ready condition")
}

// TestCreateExtraPeer_ResponseIncludesTitleMarginalia guards the response
// shape: the created peer must come back enriched with its title marginalia,
// like every other asset-create endpoint. A raw (marginalia-less) asset here
// makes the client's optimistic append carry `marginalia: undefined`, which
// throws in firstEmptySlotIndex inside the Retinue's derived and freezes the
// view (the bug this regresses).
func TestCreateExtraPeer_ResponseIncludesTitleMarginalia(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	game, players := newClosingGame(t, q, 2)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(game.ID)
	router := closingRouter(store, manager)

	rec := postJSON(t, q, router, extraPeerPath(game.ID), players[0], map[string]any{
		"title_name": "The Spymaster", "peer_text": "A quiet informant",
	})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp struct {
		Asset struct {
			ID         int64 `json:"id"`
			Marginalia []struct {
				Text string `json:"text"`
			} `json:"marginalia"`
		} `json:"asset"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotZero(t, resp.Asset.ID)
	require.Len(t, resp.Asset.Marginalia, 1, "the title marginalia must be present in the response")
	assert.NotEmpty(t, resp.Asset.Marginalia[0].Text)
}

// TestClosingReady_ConcurrentLastReady_Idempotent fires two identical
// "last player readies" requests concurrently. Depending on how the two
// requests interleave, the loser sees either a redundant 200 (both reach
// maybeAdvanceFromClosing; the SetClosingReady upsert's row lock serializes
// them, and the second one's fresh re-read finds the step already cleared
// and no-ops) or a 409 from requirePrologueStep's outer, pre-transaction
// check (the game had already fully advanced to main_event by the time its
// snapshot was loaded) — both are correct, request-level outcomes.
// What must never happen is a 500 from double-inserting public_record_rows
// or a duplicate main_event boundary post — that's the actual property
// under test.
func TestClosingReady_ConcurrentLastReady_Idempotent(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	game, players := newClosingGame(t, q, 4)

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(game.ID)
	router := closingRouter(store, manager)

	for _, p := range players[:3] {
		rec := postJSON(t, q, router, closingReadyPath(game.ID), p, map[string]any{"ready": true})
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	}

	last := players[3]
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := postJSON(t, q, router, closingReadyPath(game.ID), last, map[string]any{"ready": true})
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()

	for _, c := range codes {
		assert.Containsf(t, []int{http.StatusOK, http.StatusConflict}, c,
			"a losing concurrent request may see a redundant 200 or a 409 (already advanced), never a 500")
	}

	fresh, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseMainEvent, fresh.Phase)

	rows, err := q.ListPublicRecordRows(ctx, game.ID)
	require.NoError(t, err)
	assert.Len(t, rows, 13, "the record rows must be seeded exactly once, not twice")

	posts, err := q.ListGamePosts(ctx, game.ID)
	require.NoError(t, err)
	boundaryCount := 0
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == "phase.changed" && p.Body == "Main event begins" {
			boundaryCount++
		}
	}
	assert.Equal(t, 1, boundaryCount, "the main_event boundary post must be emitted exactly once")
}
