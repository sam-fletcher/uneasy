//go:build integration

package ops

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
)

// The exact statement .github/workflows/db-backup.yml runs as its last step.
// Keep the two in step: this is the test that proves the workflow's SQL works
// against the real schema.
const backupHeartbeatSQL = `INSERT INTO ops_backup_status (id, last_success_at) VALUES (TRUE, now())
ON CONFLICT (id) DO UPDATE SET last_success_at = now()`

func openDB(t *testing.T, tr DBTracer) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	require.NoError(t, db.RunMigrations(url))
	cfg, err := pgxpool.ParseConfig(url)
	require.NoError(t, err)
	cfg.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(context.Background(), "DELETE FROM ops_backup_status")
	require.NoError(t, err)
	return pool
}

func TestBackupStatus_RoundTrip(t *testing.T) {
	pool := openDB(t, DBTracer{Monitor: NewDBMonitor(newFakeClock().Now, func(string) {})})
	ctx := context.Background()
	q := dbgen.New(pool)

	_, err := q.GetBackupLastSuccess(ctx)
	assert.ErrorIs(t, err, pgx.ErrNoRows, "no row until the first backup reports")

	_, err = pool.Exec(ctx, backupHeartbeatSQL)
	require.NoError(t, err)
	first, err := q.GetBackupLastSuccess(ctx)
	require.NoError(t, err)
	assert.True(t, first.Valid)

	_, err = pool.Exec(ctx, backupHeartbeatSQL) // second run updates, never adds a row
	require.NoError(t, err)
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM ops_backup_status").Scan(&n))
	assert.Equal(t, 1, n)

	_, err = pool.Exec(ctx, "INSERT INTO ops_backup_status (id, last_success_at) VALUES (FALSE, now())")
	assert.Error(t, err, "the single-row constraint rejects a second row")
}

// Ordinary "no rows" results from a healthy database must never alert, and a
// real query is what ends an outage.
func TestDBTracer_RealDatabaseNoRowsIsHealthy(t *testing.T) {
	m, _, w := newMonitor()
	pool := openDB(t, DBTracer{Monitor: m})
	ctx := context.Background()
	_, err := dbgen.New(pool).GetBackupLastSuccess(ctx)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	assert.Empty(t, w.Messages())

	m.Failure(errDown)
	_, err = dbgen.New(pool).GetBackupLastSuccess(ctx)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	msgs := w.Messages()
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[1], "recovered")
}
