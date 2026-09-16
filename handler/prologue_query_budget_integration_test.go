//go:build integration

package handler

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// Query-budget tests for the prologue claim path.
//
// Production talks to a remote Postgres at ~25ms per round trip, so the
// number of *serial queries* a request issues is its latency. These tests
// count every statement (including BEGIN/COMMIT) through a pgx tracer and
// pin the claim's budget, so an N+1 or a duplicated lookup fails a test
// instead of resurfacing as "submitting a tile feels slow".

// queryCounter is a pgx QueryTracer that counts statements.
type queryCounter struct{ n atomic.Int64 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// openCountingPool returns a second pool on the test database whose every
// statement increments the returned counter. The shared harness pool has
// already migrated and truncated the schema.
func openCountingPool(t *testing.T) (*pgxpool.Pool, *queryCounter) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv(testDBURLEnv))
	require.NoError(t, err)
	counter := &queryCounter{}
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, counter
}

// seedPrologueGame creates a prologue-phase game with n players, each holding
// a main character, and returns the game plus the players in join order.
func seedPrologueGame(t *testing.T, ctx context.Context, q *dbgen.Queries, name string, n int) (dbgen.Game, []dbgen.Player) {
	t.Helper()
	game, err := q.CreateGame(ctx, name)
	require.NoError(t, err)
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{ID: game.ID, Phase: model.PhasePrologue}))
	players := make([]dbgen.Player, 0, n)
	for i := range n {
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: fmt.Sprintf("qb%d-%s", i, game.JoinCode), PasswordHash: "x",
		})
		require.NoError(t, err)
		p, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID: game.ID, DisplayName: fmt.Sprintf("Player %d", i), AccountID: acct.ID,
		})
		require.NoError(t, err)
		_, err = q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID: game.ID, OwnerID: p.ID, CreatorID: p.ID,
			AssetType: model.AssetPeer, Name: fmt.Sprintf("MC %d", i), IsMainCharacter: true,
		})
		require.NoError(t, err)
		players = append(players, p)
	}
	return game, players
}

// countClaimQueries runs one two-make Titles claim for the first player of a
// fresh n-player game inside a transaction and returns the statement count.
func countClaimQueries(t *testing.T, n int) int64 {
	t.Helper()
	ctx := context.Background()
	harness := openTestDB(t) // migrates + truncates
	game, players := seedPrologueGame(t, ctx, dbgen.New(harness), fmt.Sprintf("Budget%d", n), n)

	pool, counter := openCountingPool(t)
	q := dbgen.New(pool)
	body := &chooseRequestBody{
		SheetType:       gamepkg.PrologueSheetTitles,
		ChoiceName:      "The Monarch",
		AssetText:       "Lady of the Vale",
		AssetMarginalia: "Keeper of the old ways",
		MarginaliumText: "Beloved of the people",
		CardAssets: []CardAssetText{
			{Suit: "C", Value: "K", Text: "Household Guard"},
			{Suit: "D", Value: "K", Text: "Crown Jewels"},
		},
	}
	choice := gamepkg.FindPrologueChoice(body.SheetType, body.ChoiceName)
	require.NotNil(t, choice)

	// Warm the pool so connection setup queries don't count.
	require.NoError(t, pool.Ping(ctx))
	counter.n.Store(0)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = recordPrologueChoice(ctx, q.WithTx(tx), hub.NewManager(), game.ID, players[0].ID, body, choice)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return counter.n.Load()
}

// The claim's cost must not scale with the roster: the turn-order check is
// one grouped count, never one count per player.
func TestRecordPrologueChoice_QueryCountIndependentOfRoster(t *testing.T) {
	two := countClaimQueries(t, 2)
	six := countClaimQueries(t, 6)
	assert.Equal(t, two, six, "claim queries grew with player count: 2p=%d 6p=%d", two, six)
}

// recordPrologueChoiceQueryBudget is the measured statement count for a
// two-make Titles claim, BEGIN and COMMIT included:
//
//	begin, roster, grouped count, claimed?, create choice, create asset,
//	create marginalia, log post, [title: owner assets, MC marginalia, create
//	title, MC re-read, log post], 2 × [card owner, create asset, hand row,
//	log post], commit
//
// Raise it only with a reason in the commit; the point is that a change
// which adds a round trip to the hottest prologue action has to say so.
const recordPrologueChoiceQueryBudget = 23

func TestRecordPrologueChoice_QueryBudget(t *testing.T) {
	got := countClaimQueries(t, 5)
	assert.LessOrEqual(t, got, int64(recordPrologueChoiceQueryBudget),
		"two-make Titles claim issued %d statements; budget is %d", got, recordPrologueChoiceQueryBudget)
	t.Logf("two-make Titles claim: %d statements (budget %d)", got, recordPrologueChoiceQueryBudget)
}

// loadPrologueTurns is on the table-load, profile-load and every-claim paths;
// it must stay at exactly two statements whatever the roster size.
func TestLoadPrologueTurns_TwoQueries(t *testing.T) {
	ctx := context.Background()
	harness := openTestDB(t)
	game, players := seedPrologueGame(t, ctx, dbgen.New(harness), "TurnsBudget", 5)
	hq := dbgen.New(harness)
	for i, p := range players[:3] {
		_, err := hq.CreatePrologueChoice(ctx, dbgen.CreatePrologueChoiceParams{
			GameID: game.ID, PlayerID: p.ID, TurnNumber: 1,
			SheetType: gamepkg.PrologueSheetTitles, ChoiceName: fmt.Sprintf("T%d", i),
		})
		require.NoError(t, err)
	}

	pool, counter := openCountingPool(t)
	require.NoError(t, pool.Ping(ctx))
	counter.n.Store(0)

	turns, err := loadPrologueTurns(ctx, dbgen.New(pool), game.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, counter.n.Load())

	// And the snapshot agrees with the per-player counts it replaced.
	require.NotNil(t, turns.active)
	assert.Equal(t, players[3].ID, turns.active.ID, "fourth joiner is on-turn after three first turns")
	assert.Equal(t, 4, turns.nextTurn)
	for _, p := range players {
		n, err := hq.CountPrologueChoicesByPlayer(ctx, dbgen.CountPrologueChoicesByPlayerParams{GameID: game.ID, PlayerID: p.ID})
		require.NoError(t, err)
		assert.EqualValues(t, n, turns.takenBy(p.ID))
	}
}
