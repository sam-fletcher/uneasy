//go:build integration

package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// Auto-done: a player holding no ANY card they could still place has no move
// to make on a declare step — nothing to commit, nothing to retract — so the
// server never waits for their "I'm done" tap (allPlayersDoneForTrack), and
// resolves a track outright when nobody at the table can act
// (autoResolveIfNobodyCanDeclare). These tests cover all three ways into a
// declare step: the first entry at declare_power, resolveTrack's own advance,
// and PlaceSetAsides's advance.

// ── Fixture helpers ──────────────────────────────────────────────────────────

// dealCard puts one card in a player's hand and returns its player_cards row.
func dealCard(t *testing.T, q *dbgen.Queries, gameID, playerID int64, suit, value string) dbgen.PlayerCard {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, q.InsertPlayerCard(ctx, dbgen.InsertPlayerCardParams{
		GameID: gameID, PlayerID: playerID, CardSuit: suit, CardValue: value,
	}))
	rows, err := q.ListPlayerCardsByGame(ctx, gameID)
	require.NoError(t, err)
	for _, r := range rows {
		if r.PlayerID == playerID && r.CardSuit == suit && r.CardValue == value {
			return r
		}
	}
	t.Fatalf("dealt card %s%s to player %d but it isn't in the game's hands", value, suit, playerID)
	return dbgen.PlayerCard{}
}

// parkAtRankingStep moves a seeded game into the prologue at the given ranking
// step (pass "" to leave the step NULL, i.e. still choosing).
func parkAtRankingStep(t *testing.T, q *dbgen.Queries, gameID int64, step string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: gameID, Phase: model.PhasePrologue,
	}))
	var ptr *string
	if step != "" {
		ptr = &step
	}
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: gameID, PrologueRankingStep: ptr,
	}))
}

func rankingStep(t *testing.T, q *dbgen.Queries, gameID int64) string {
	t.Helper()
	game, err := q.GetGameByID(context.Background(), gameID)
	require.NoError(t, err)
	if game.PrologueRankingStep == nil {
		return ""
	}
	return *game.PrologueRankingStep
}

// rankAt returns the player at `rank` on a track, or nil for a dummy slot.
func rankAt(t *testing.T, q *dbgen.Queries, gameID int64, cat model.RankingCategory, rank int16) *int64 {
	t.Helper()
	rankings, err := q.ListRankingsByGame(context.Background(), gameID)
	require.NoError(t, err)
	for _, rk := range rankings {
		if rk.Category == cat && rk.Rank == rank {
			return rk.PlayerID
		}
	}
	t.Fatalf("no ranking row for %s rank %d", cat, rank)
	return nil
}

func countSystemPosts(t *testing.T, q *dbgen.Queries, gameID int64, code string) int {
	t.Helper()
	posts, err := q.ListGamePosts(context.Background(), gameID)
	require.NoError(t, err)
	n := 0
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == code {
			n++
		}
	}
	return n
}

func donePath(gameID int64) string {
	return "/api/tables/" + strconv.FormatInt(gameID, 10) + "/prologue/done"
}

func commitHeartsPath(gameID int64) string {
	return "/api/tables/" + strconv.FormatInt(gameID, 10) + "/prologue/committed-hearts"
}

// declareRouter mounts the endpoints a declare step is driven through.
func declareRouter(store *db.Store, manager *hub.Manager) http.Handler {
	router := chi.NewRouter()
	router.Use(appMiddleware.EnsureSession(store.Q))
	router.Post("/api/tables/{id}/prologue/done", SetPrologueDone(store, manager))
	router.Post("/api/tables/{id}/prologue/committed-hearts", CommitTrackHearts(store, manager))
	router.Post("/api/tables/{id}/prologue/place-set-asides", PlaceSetAsides(store, manager))
	return router
}

// ── (a) Only the card-holders' Done taps are waited for ──────────────────────

// TestSetPrologueDone_ResolvesOnceOnlyCardHoldersAreDone: two of four players
// hold an ANY card. The track must wait for both of THEM — and for neither of
// the other two, who have nothing to spend and so nothing to say. Their
// auto-done is derived, never written: no track_done row appears for them.
func TestSetPrologueDone_ResolvesOnceOnlyCardHoldersAreDone(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4)
	parkAtRankingStep(t, q, tg.Game.ID, gamepkg.PrologueStepDeclarePower)

	// Clubs feed power, so every player is ranked on it naturally. Only
	// players 0 and 1 also hold an ANY card.
	clubs := []string{"K", "A", "Q", "J"}
	for i, p := range tg.Players {
		dealCard(t, q, tg.Game.ID, p.ID, "C", clubs[i])
	}
	dealCard(t, q, tg.Game.ID, tg.Players[0].ID, "H", "K")
	dealCard(t, q, tg.Game.ID, tg.Players[1].ID, "H", "Q")

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := declareRouter(store, manager)

	// Player 0 alone is not enough: player 1 still holds a card and a choice.
	rec := postJSON(t, q, router, donePath(tg.Game.ID), tg.Players[0], map[string]any{
		"track": "power", "done": true,
	})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, gamepkg.PrologueStepDeclarePower, rankingStep(t, q, tg.Game.ID),
		"a player who can still act has not been asked yet")

	doneRows, err := q.ListTrackDoneByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, doneRows, 1, "the two empty-handed players must not get fabricated done rows")
	assert.Equal(t, tg.Players[0].ID, doneRows[0].PlayerID)

	// Player 1 is the last player who COULD act, so their tap resolves the
	// track — the two empty-handed players are never asked.
	rec = postJSON(t, q, router, donePath(tg.Game.ID), tg.Players[1], map[string]any{
		"track": "power", "done": true,
	})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	assert.Equal(t, gamepkg.PrologueStepDeclareKnowledge, rankingStep(t, q, tg.Game.ID),
		"power resolves on the last card-holder's Done; knowledge still has two ANY cards in play")
	assert.Equal(t, &tg.Players[1].ID, rankAt(t, q, tg.Game.ID, model.CategoryPower, 1),
		"the ace of clubs takes the top power slot")
	assert.Equal(t, 1, countSystemPosts(t, q, tg.Game.ID, "prologue.track_ranked"))
}

// ── (b) Entry-time resolution, and the halt at a real decision ───────────────

// TestEnterPrologueRanking_NoAnyCardsResolvesOnEntry: the table finished
// choosing without a single ANY card between them — 16 of the 36 tiles carry
// none, so this is an ordinary deal, not a freak one. Nobody can act on power,
// and no Done POST is ever coming, so entry itself must resolve it.
//
// The cascade then correctly STOPS at place_set_asides_power, which is a real
// decision; the second half of the test plays that decision and checks the
// third entry point — PlaceSetAsides's own advance — runs the remaining two
// empty tracks through to closing.
func TestEnterPrologueRanking_NoAnyCardsResolvesOnEntry(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4)
	parkAtRankingStep(t, q, tg.Game.ID, "") // choosing: no ranking step yet

	// No ANY cards anywhere. Players 2 and 3 hold no clubs, so power resolves
	// with two set-asides — the one thing on this board that still needs a
	// player.
	dealCard(t, q, tg.Game.ID, tg.Players[0].ID, "C", "A")
	dealCard(t, q, tg.Game.ID, tg.Players[1].ID, "C", "K")
	diamonds := []string{"A", "K", "Q", "J"}
	spades := []string{"A", "K", "Q", "J"}
	for i, p := range tg.Players {
		dealCard(t, q, tg.Game.ID, p.ID, "D", diamonds[i])
		dealCard(t, q, tg.Game.ID, p.ID, "S", spades[i])
	}

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)

	require.NoError(t, enterPrologueRanking(ctx, store, manager, tg.Game.ID))

	assert.Equal(t, gamepkg.PrologueStepPlaceSetAsidesPower, rankingStep(t, q, tg.Game.ID),
		"power resolved without a single Done POST, then stopped at a decision that needs a player")
	assert.Equal(t, &tg.Players[0].ID, rankAt(t, q, tg.Game.ID, model.CategoryPower, 1))
	assert.Equal(t, &tg.Players[1].ID, rankAt(t, q, tg.Game.ID, model.CategoryPower, 2))

	doneRows, err := q.ListTrackDoneByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, doneRows, "nobody was asked to be done, so nothing was recorded")
	assert.Equal(t, 0, countSystemPosts(t, q, tg.Game.ID, "prologue.track_ranked"),
		"power's standing is logged by the set-aside path, once it is actually final")

	// The top player places the set-asides. That advance lands on
	// declare_knowledge — which nobody can act on either.
	router := declareRouter(store, manager)
	rec := postJSON(t, q, router,
		"/api/tables/"+strconv.FormatInt(tg.Game.ID, 10)+"/prologue/place-set-asides",
		tg.Players[0], map[string]any{"ordering": []int64{tg.Players[2].ID, tg.Players[3].ID}})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), gamepkg.PrologueStepClosing,
		"the response reports the step the game actually ended on, not the one it passed through")

	assert.Equal(t, gamepkg.PrologueStepClosing, rankingStep(t, q, tg.Game.ID))
	assert.Equal(t, &tg.Players[0].ID, rankAt(t, q, tg.Game.ID, model.CategoryKnowledge, 1))
	assert.Equal(t, &tg.Players[0].ID, rankAt(t, q, tg.Game.ID, model.CategoryEsteem, 1))
	assert.Equal(t, model.PhasePrologue, rankingPhase(t, q, tg.Game.ID),
		"closing is a gated step, not an instant advance to the main event")
}

// ── (c) One tap cascading through every remaining track ──────────────────────

// TestSetPrologueDone_CascadesThroughEmptyTracksToClosing: the last ANY card at
// the table locks in on power (it is bright, so it does not refund), leaving
// nobody able to act on knowledge or esteem. One player's Done must therefore
// carry the game through both of them and into closing, inside a single
// request — with each track's ranking persisted and each one's standing logged
// on the way past.
func TestSetPrologueDone_CascadesThroughEmptyTracksToClosing(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 4)
	parkAtRankingStep(t, q, tg.Game.ID, gamepkg.PrologueStepDeclarePower)

	// Player 0 holds the table's only ANY card and no clubs, so committing it
	// to power is load-bearing (bright): without it they'd drop from the
	// second slot to last. Bright hearts lock at resolution rather than
	// refunding, which is what empties their hand for the rest of the ranking.
	wild := dealCard(t, q, tg.Game.ID, tg.Players[0].ID, "H", "K")
	clubs := []string{"", "A", "Q", "J"}
	diamonds := []string{"A", "K", "Q", "J"}
	spades := []string{"A", "K", "Q", "J"}
	for i, p := range tg.Players {
		if clubs[i] != "" {
			dealCard(t, q, tg.Game.ID, p.ID, "C", clubs[i])
		}
		dealCard(t, q, tg.Game.ID, p.ID, "D", diamonds[i])
		dealCard(t, q, tg.Game.ID, p.ID, "S", spades[i])
	}

	store := db.NewStore(pool)
	manager := hub.NewManager()
	manager.GetOrCreate(tg.Game.ID)
	router := declareRouter(store, manager)

	rec := postJSON(t, q, router, commitHeartsPath(tg.Game.ID), tg.Players[0], map[string]any{
		"track": "power", "card_ids": []int64{wild.ID},
	})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	// Player 0 is the only player with a card to spend, so this is the only
	// tap the whole ranking stage needs.
	rec = postJSON(t, q, router, donePath(tg.Game.ID), tg.Players[0], map[string]any{
		"track": "power", "done": true,
	})
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	assert.Equal(t, gamepkg.PrologueStepClosing, rankingStep(t, q, tg.Game.ID),
		"knowledge and esteem hold no decision for anyone, so the request runs them through to closing")
	assert.Equal(t, model.PhasePrologue, rankingPhase(t, q, tg.Game.ID))

	// The ANY card stayed spent on power: it was bright, so no refund.
	committed, err := q.ListCommittedHeartsByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, committed, 1)
	assert.Equal(t, gamepkg.PrologueTrackPower, committed[0].Track)
	assert.Equal(t, wild.ID, committed[0].CardID)

	// All three tracks ranked, with player 0's second power slot bought by
	// that card.
	assert.Equal(t, &tg.Players[1].ID, rankAt(t, q, tg.Game.ID, model.CategoryPower, 1))
	assert.Equal(t, &tg.Players[0].ID, rankAt(t, q, tg.Game.ID, model.CategoryPower, 2))
	assert.Equal(t, &tg.Players[0].ID, rankAt(t, q, tg.Game.ID, model.CategoryKnowledge, 1))
	assert.Equal(t, &tg.Players[0].ID, rankAt(t, q, tg.Game.ID, model.CategoryEsteem, 1))

	assert.Equal(t, 3, countSystemPosts(t, q, tg.Game.ID, "prologue.track_ranked"),
		"every track the cascade passed through logs its opening standing")
	assert.Equal(t, 1, countSystemPosts(t, q, tg.Game.ID, "prologue.closing_entered"))

	doneRows, err := q.ListTrackDoneByGame(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.Empty(t, doneRows, "resolution clears the flags it passed, and none were fabricated")
}

// rankingPhase is a small readability wrapper for the phase assertions above.
func rankingPhase(t *testing.T, q *dbgen.Queries, gameID int64) model.GamePhase {
	t.Helper()
	game, err := q.GetGameByID(context.Background(), gameID)
	require.NoError(t, err)
	return game.Phase
}
