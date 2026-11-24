package ops

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real pgx pool, a port nothing listens on: proves the tracer actually
// fires on "connection refused", which a hand-built TraceConnectEnd call can't.
func TestDBTracer_RealPoolConnectionRefused(t *testing.T) {
	m, _, w := newMonitor()
	cfg, err := pgxpool.ParseConfig("postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=2")
	require.NoError(t, err)
	cfg.ConnConfig.Tracer = DBTracer{Monitor: m}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.Error(t, pool.Ping(ctx))

	msgs := w.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "Database failing")
}
