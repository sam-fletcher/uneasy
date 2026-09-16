package main

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// dbWarmer wakes a scale-to-zero database (Neon suspends after ~5 idle
// minutes and takes ~1s to resume) while the browser is still busy
// downloading the JS bundle, so the page's first API call finds a warm
// connection instead of paying the cold start itself. Play-by-post means
// most visits are cold, so that second was on the critical path of nearly
// every session.
//
// It is fired from the index.html path in setupFrontend, and ONLY when the
// request carries the session cookie: crawlers and uptime pingers hit "/"
// around the clock, and letting them wake the database would recreate the
// CU-budget blowout described at notifyTickInterval. /healthz stays DB-free
// for the same reason.
//
// Warm never blocks the caller — the ping runs in its own goroutine — and it
// coalesces: at most one ping in flight, and none at all within
// warmupMinInterval of the last one finishing, so a burst of tabs or a
// reload storm costs one round trip, not one per request.
type dbWarmer struct {
	ping   func(context.Context) error
	logger *slog.Logger
	now    func() time.Time

	mu       sync.Mutex
	inFlight bool
	last     time.Time
}

const (
	// warmupMinInterval is how long after a ping finishes before another may
	// start. Once the pool holds a live connection, a further ping buys
	// nothing; 30s comfortably covers a page load plus its first API calls.
	warmupMinInterval = 30 * time.Second
	// warmupTimeout bounds a single ping. A cold Neon resume is ~1s; anything
	// past a few seconds is an outage the real API calls will report on
	// their own, so give up rather than pile goroutines onto it.
	warmupTimeout = 5 * time.Second
)

// newDBWarmer wraps ping (typically pgxpool.Pool.Ping) in a coalescing,
// rate-limited, fire-and-forget warmer.
func newDBWarmer(logger *slog.Logger, ping func(context.Context) error) *dbWarmer {
	return &dbWarmer{ping: ping, logger: logger, now: time.Now}
}

// Warm starts a background ping unless one is already in flight or one
// finished within warmupMinInterval. It reports whether a ping was started,
// which matters only to tests.
func (w *dbWarmer) Warm() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inFlight {
		return false
	}
	if !w.last.IsZero() && w.now().Sub(w.last) < warmupMinInterval {
		return false
	}
	w.inFlight = true
	go w.run()
	return true
}

func (w *dbWarmer) run() {
	ctx, cancel := context.WithTimeout(context.Background(), warmupTimeout)
	defer cancel()
	start := w.now()
	err := w.ping(ctx)
	elapsed := w.now().Sub(start)

	w.mu.Lock()
	w.inFlight = false
	// Stamped on failure too: if the database is unreachable, every page load
	// retrying it would be exactly the pile-up the interval exists to stop.
	w.last = w.now()
	w.mu.Unlock()

	if err != nil {
		w.logger.Warn("db warm-up ping failed", "error", err, "ms", elapsed.Milliseconds())
		return
	}
	// Info, not Debug: this is the one place the cold-start cost is visible
	// in production logs (a warm pool answers in ~25ms, a resume in ~1000ms).
	w.logger.Info("db warm-up ping", "ms", elapsed.Milliseconds())
}
