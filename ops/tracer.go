package ops

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// DBTracer plugs a DBMonitor into the connection pool. It is set once, on the
// pool's config, so every query and every new connection from every handler
// passes through it: the one central place the app already goes through, with
// no per-handler code and no query of its own.
//
// It implements pgx's QueryTracer (every Query/QueryRow/Exec, which includes
// BEGIN and COMMIT) and ConnectTracer (every attempt to open a connection:
// that's where "connection refused", a suspended Neon, a quota error and a
// "too many connections" refusal show up, because pgx never reaches a query).
type DBTracer struct {
	Monitor *DBMonitor
	// Inner, if set, receives every query trace first: the dev build's
	// per-request statement counter, which shares the same hook.
	Inner pgx.QueryTracer
}

var (
	_ pgx.QueryTracer   = DBTracer{}
	_ pgx.ConnectTracer = DBTracer{}
)

// TraceQueryStart implements pgx.QueryTracer.
func (t DBTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if t.Inner != nil {
		return t.Inner.TraceQueryStart(ctx, conn, data)
	}
	return ctx
}

// TraceQueryEnd implements pgx.QueryTracer. A query that finished with a normal
// outcome (including "no rows" and constraint violations) proves the database
// is answering, so it counts as a success. A cancelled request says nothing
// either way.
func (t DBTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if t.Inner != nil {
		t.Inner.TraceQueryEnd(ctx, conn, data)
	}
	switch {
	case IsDBFailure(data.Err):
		t.Monitor.Failure(data.Err)
	case errors.Is(data.Err, context.Canceled), errors.Is(data.Err, context.DeadlineExceeded):
		// Neither a sign of life nor of trouble.
	default:
		t.Monitor.Success()
	}
}

// TraceConnectStart implements pgx.ConnectTracer.
func (t DBTracer) TraceConnectStart(ctx context.Context, _ pgx.TraceConnectStartData) context.Context {
	return ctx
}

// TraceConnectEnd implements pgx.ConnectTracer.
func (t DBTracer) TraceConnectEnd(_ context.Context, data pgx.TraceConnectEndData) {
	if IsConnectFailure(data.Err) {
		t.Monitor.Failure(data.Err)
	}
	// A successful connect is not counted as a success: the query that follows
	// is the real proof, and the first query after a reconnect reports it.
}
