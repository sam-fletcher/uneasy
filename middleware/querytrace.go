package middleware

// querytrace.go — per-request database statement accounting for dev.
//
// Production talks to a Postgres in another region, so a request's latency is
// mostly its *serial* chain of round trips (~25ms each) rather than query
// execution. This file gives dev builds one log line per API request —
//
//	db trace method=POST path=/api/tables/{id}/prologue/choose status=200 queries=23 trips=23 db_ms=4.1
//
// — so an N+1 or a duplicated lookup is visible in `docker compose logs`
// instead of resurfacing as "that button feels slow" in prod. It is wired in
// only when the server runs in dev (UNEASY_DEV=1 or DEV_MODE=true): the pool
// gets no tracer in production and the middleware is not mounted, so
// production pays nothing.
//
// Mechanics: QueryTraceLog stashes a counter on the request context; pgx
// hands that same context to QueryTracer for every Query/QueryRow/Exec
// (BEGIN/COMMIT included), which bumps the counter when one is present and
// is a no-op otherwise (background goroutines, startup). Concurrent
// statements from a fan-out all count towards queries; trips is the number
// of round trips the request's serial chain took, counting a statement that
// starts while others are still in flight as part of the same trip. Under
// the production cost model (~25ms per trip, with concurrent statements
// costing one) trips is the number to rank by; queries is the DB's work.
// db_ms is the sum of statement durations, not the wall clock.

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
)

// queryStats accumulates one request's statement count, round trips and DB
// time. inFlight is the number of statements currently running: a statement
// that starts when it is zero opens a new trip.
type queryStats struct {
	n        atomic.Int64
	trips    atomic.Int64
	inFlight atomic.Int64
	ns       atomic.Int64
}

type (
	queryStatsKey struct{}
	queryStartKey struct{}
)

// QueryTracer is a pgx.QueryTracer that charges each statement to the
// request whose context it runs under. Set it on the pool config
// (cfg.ConnConfig.Tracer) in dev only.
type QueryTracer struct{}

// TraceQueryStart implements pgx.QueryTracer.
func (QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	stats, ok := ctx.Value(queryStatsKey{}).(*queryStats)
	if !ok {
		return ctx
	}
	stats.n.Add(1)
	if stats.inFlight.Add(1) == 1 {
		stats.trips.Add(1)
	}
	return context.WithValue(ctx, queryStartKey{}, time.Now())
}

// TraceQueryEnd implements pgx.QueryTracer.
func (QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	stats, ok := ctx.Value(queryStatsKey{}).(*queryStats)
	if !ok {
		return
	}
	stats.inFlight.Add(-1)
	if start, ok := ctx.Value(queryStartKey{}).(time.Time); ok {
		stats.ns.Add(int64(time.Since(start)))
	}
}

// QueryTraceLog returns middleware that attaches a statement counter to the
// request context and, once the handler returns, logs the request's method,
// chi route pattern, status, statement count, round trips and summed DB time
// at debug level. Mount it above EnsureSession so the auth lookups are counted too.
func QueryTraceLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			stats := &queryStats{}
			ctx := context.WithValue(r.Context(), queryStatsKey{}, stats)
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r.WithContext(ctx))

			// The pattern ("/api/tables/{id}/state") rather than the URL, so
			// lines for the same endpoint rank together.
			path := r.URL.Path
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				if p := rctx.RoutePattern(); p != "" {
					path = p
				}
			}
			logger.DebugContext(r.Context(), "db trace",
				"method", r.Method,
				"path", path,
				"status", ww.Status(),
				"queries", stats.n.Load(),
				"trips", stats.trips.Load(),
				"db_ms", float64(stats.ns.Load())/float64(time.Millisecond),
			)
		})
	}
}
