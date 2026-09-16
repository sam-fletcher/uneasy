package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	appMiddleware "uneasy/middleware"
)

// fakeClock lets the tests step past warmupMinInterval without sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// blockingPing is a ping the test releases by hand, so "in flight" is a
// state the test controls rather than a race it has to win.
type blockingPing struct {
	calls   atomic.Int32
	release chan struct{}
	done    chan struct{}
}

func newBlockingPing() *blockingPing {
	return &blockingPing{release: make(chan struct{}), done: make(chan struct{}, 16)}
}

func (p *blockingPing) ping(context.Context) error {
	p.calls.Add(1)
	<-p.release
	p.done <- struct{}{}
	return nil
}

// waitIdle blocks until the warmer's goroutine has cleared inFlight, which
// happens strictly after ping returns.
func waitIdle(t *testing.T, w *dbWarmer) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		idle := !w.inFlight
		w.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("warmer never went idle")
}

func TestDBWarmerCoalescesAndRateLimits(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	ping := newBlockingPing()
	w := newDBWarmer(slog.New(slog.DiscardHandler), ping.ping)
	w.now = clock.now

	// First call starts a ping; while it is in flight, further calls coalesce.
	if !w.Warm() {
		t.Fatal("first Warm() should start a ping")
	}
	for i := range 5 {
		if w.Warm() {
			t.Fatalf("Warm() #%d started a second ping while one was in flight", i+2)
		}
	}
	close(ping.release)
	<-ping.done
	waitIdle(t, w)
	if got := ping.calls.Load(); got != 1 {
		t.Fatalf("ping calls = %d, want 1 (coalescing failed)", got)
	}

	// Just finished: still inside the interval, so nothing fires.
	clock.advance(warmupMinInterval - time.Second)
	if w.Warm() {
		t.Fatal("Warm() fired inside warmupMinInterval of the last ping")
	}
	// Past the interval: fires again.
	clock.advance(2 * time.Second)
	if !w.Warm() {
		t.Fatal("Warm() did not fire after warmupMinInterval elapsed")
	}
	<-ping.done
	waitIdle(t, w)
	if got := ping.calls.Load(); got != 2 {
		t.Fatalf("ping calls = %d, want 2", got)
	}
}

// A failed ping must still stamp the interval — otherwise an unreachable
// database turns every page load into a fresh retry.
func TestDBWarmerRateLimitsAfterFailure(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	var calls atomic.Int32
	w := newDBWarmer(slog.New(slog.DiscardHandler), func(context.Context) error {
		calls.Add(1)
		return errors.New("boom")
	})
	w.now = clock.now

	if !w.Warm() {
		t.Fatal("first Warm() should start a ping")
	}
	waitIdle(t, w)
	if w.Warm() {
		t.Fatal("Warm() retried immediately after a failed ping")
	}
	clock.advance(warmupMinInterval + time.Second)
	if !w.Warm() {
		t.Fatal("Warm() did not fire after the interval following a failure")
	}
	waitIdle(t, w)
	if got := calls.Load(); got != 2 {
		t.Fatalf("ping calls = %d, want 2", got)
	}
}

// The frontend handler must fire the hook only for index.html AND only when
// the session cookie is present: anonymous traffic (crawlers, uptime pings on
// "/") must not wake the database, and asset requests ride behind an index
// that already fired it.
func TestFrontendWarmsOnlyForCookiedIndex(t *testing.T) {
	var calls atomic.Int32
	r := chi.NewRouter()
	if err := setupFrontend(r, false, "", func() { calls.Add(1) }); err != nil {
		t.Fatalf("setupFrontend: %v", err)
	}
	asset := "/" + anImmutableAsset(t)

	tests := []struct {
		name   string
		url    string
		cookie bool
		want   int32
	}{
		{"index with cookie fires", "/", true, 1},
		{"index without cookie does not", "/", false, 0},
		{"client route (SPA fallback) with cookie fires", "/table/42", true, 1},
		{"client route without cookie does not", "/table/42", false, 0},
		{"hashed asset with cookie does not", asset, true, 0},
		{"service worker with cookie does not", "/service-worker.js", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls.Store(0)
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			if tt.cookie {
				req.AddCookie(&http.Cookie{Name: appMiddleware.SessionCookie, Value: "tok"})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if got := calls.Load(); got != tt.want {
				t.Errorf("warm calls = %d, want %d", got, tt.want)
			}
		})
	}
}

// A nil hook must be accepted (tests and any future DB-less mode).
func TestFrontendNilWarmHook(t *testing.T) {
	r := chi.NewRouter()
	if err := setupFrontend(r, false, "", nil); err != nil {
		t.Fatalf("setupFrontend: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: appMiddleware.SessionCookie, Value: "tok"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
