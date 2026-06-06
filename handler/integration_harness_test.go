//go:build integration

// Package handler — integration test harness.
//
// Tests in this file (and other *_integration_test.go files) talk to a real
// Postgres database. They are gated by the `integration` build tag so the
// default `go test ./...` run stays fast and hermetic.
//
// # Running
//
//	TEST_DATABASE_URL=postgres://uneasy:uneasy@localhost:5432/uneasy_test?sslmode=disable \
//	  go test -tags=integration ./handler/...
//
// If TEST_DATABASE_URL is unset, tests that use the harness are skipped.
// The database it points at is truncated between test cases; do not aim
// the harness at a production or dev database.
package handler

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/gametest"
	"uneasy/model"
)

// testDBURLEnv is the env var tests read to find the test Postgres. If
// unset, the harness skips.
const testDBURLEnv = "TEST_DATABASE_URL"

var (
	harnessOnce sync.Once
	harnessPool *pgxpool.Pool
	harnessErr  error
)

// openTestDB returns a pool pointing at the test database, applying
// migrations exactly once per `go test` invocation. Skips the calling
// test if TEST_DATABASE_URL is unset or the DB is unreachable.
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(testDBURLEnv)
	if url == "" {
		t.Skipf("set %s to run integration tests", testDBURLEnv)
	}
	harnessOnce.Do(func() {
		pool, err := pgxpool.New(context.Background(), url)
		if err != nil {
			harnessErr = fmt.Errorf("connect: %w", err)
			return
		}
		if err := pool.Ping(context.Background()); err != nil {
			harnessErr = fmt.Errorf("ping: %w", err)
			return
		}
		if err := db.RunMigrations(url); err != nil {
			harnessErr = fmt.Errorf("migrate: %w", err)
			return
		}
		harnessPool = pool
	})
	if harnessErr != nil {
		t.Skipf("test DB unavailable: %v", harnessErr)
	}
	truncateAll(t, harnessPool)
	return harnessPool
}

// truncateAll wipes every user table in the public schema between tests.
// Enumerating information_schema avoids drift when migrations add, rename,
// or drop tables — TRUNCATE ... CASCADE handles FK ordering automatically
// and RESTART IDENTITY keeps generated IDs predictable for debug output.
// schema_migrations is preserved so we don't re-run migrations each test.
func truncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name <> 'schema_migrations'
	`)
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		require.NoError(t, err)
		tables = append(tables, `"`+name+`"`)
	}
	if len(tables) == 0 {
		return
	}

	stmt := "TRUNCATE " + joinComma(tables) + " RESTART IDENTITY CASCADE"
	_, err = pool.Exec(ctx, stmt)
	require.NoError(t, err)
}

func joinComma(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// testGame is the return shape of newTestGame. All IDs are fresh — callers
// may index players[i] freely; they are stored in seat_order 1..N with
// power ranks also 1..N (player[0] = highest-ranked).
type testGame struct {
	Game    dbgen.Game
	Players []dbgen.Player
}

// newTestGame is a thin wrapper around gametest.SeedMainEvent that
// generates fresh usernames per call. The implementation lives in the
// shared gametest package so the dev /api/dev/seed endpoint and these
// tests stay aligned. Optional gametest.Option values (WithCurrentRow,
// WithRankings, WithPlan) shape the board beyond the blank row-1 default.
func newTestGame(t *testing.T, q *dbgen.Queries, n int, opts ...gametest.Option) testGame {
	t.Helper()
	require.GreaterOrEqual(t, n, 2)
	require.LessOrEqual(t, n, 5)
	usernames := make([]string, n)
	for i := range usernames {
		usernames[i] = fmt.Sprintf("p%d-%s", i+1, randSuffix())
	}
	seeded, err := gametest.SeedMainEvent(context.Background(), q, usernames, opts...)
	require.NoError(t, err)
	return testGame{Game: seeded.Game, Players: seeded.Players}
}

// randSuffix returns a short random string for distinct join codes /
// cookie tokens across subtests within the same process lifetime.
func randSuffix() string {
	s, err := db.NewCookieToken()
	if err != nil {
		return "xxxxx"
	}
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// createPlanOnRow inserts a plan row directly with sensible defaults. Used
// by tests that need a target plan to demand against without driving the
// full preparation flow for an unrelated plan type.
func createPlanOnRow(
	t *testing.T,
	q *dbgen.Queries,
	game *dbgen.Game,
	preparer *dbgen.Player,
	planType model.PlanType,
	category model.RankingCategory,
	row int16,
) dbgen.Plan {
	t.Helper()
	p, err := q.CreatePlan(context.Background(), dbgen.CreatePlanParams{
		GameID:        game.ID,
		PlanType:      planType,
		Category:      category,
		PreparerID:    preparer.ID,
		RowNumber:     &row,
		RowOrder:      0,
		PreparedAtRow: game.CurrentRow,
	})
	require.NoError(t, err)
	return p
}
