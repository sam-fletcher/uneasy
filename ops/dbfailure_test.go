package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock is a hand-wound clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)}
}
func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fakeWebhook records messages instead of posting to Discord.
type fakeWebhook struct {
	mu   sync.Mutex
	msgs []string
}

func (w *fakeWebhook) Post(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.msgs = append(w.msgs, msg)
}
func (w *fakeWebhook) Messages() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.msgs...)
}

var errDown = errors.New("connection refused")

func newMonitor() (*DBMonitor, *fakeClock, *fakeWebhook) {
	c, w := newFakeClock(), &fakeWebhook{}
	return NewDBMonitor(c.Now, w.Post), c, w
}

func TestDBMonitor_FirstFailurePostsOnce(t *testing.T) {
	m, _, w := newMonitor()
	m.Failure(errDown)
	m.Failure(errDown)
	m.Failure(errDown)
	msgs := w.Messages()
	require.Len(t, msgs, 1, "a burst of failures is one message")
	assert.Contains(t, msgs[0], "Database failing")
	assert.Contains(t, msgs[0], "connection refused")
}

func TestDBMonitor_RepeatsAtMostHourly(t *testing.T) {
	m, c, w := newMonitor()
	m.Failure(errDown)
	c.Advance(59 * time.Minute)
	m.Failure(errDown)
	assert.Len(t, w.Messages(), 1, "59 minutes in: still quiet")
	c.Advance(2 * time.Minute) // 61 minutes since the first alert
	m.Failure(errDown)
	msgs := w.Messages()
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[1], "still failing")
	c.Advance(30 * time.Minute)
	m.Failure(errDown)
	assert.Len(t, w.Messages(), 2, "the hour restarts from the last message")
}

func TestDBMonitor_RecoveryPostsOnceOnFirstSuccess(t *testing.T) {
	m, c, w := newMonitor()
	m.Failure(errDown)
	c.Advance(5 * time.Minute)
	m.Success()
	m.Success()
	msgs := w.Messages()
	require.Len(t, msgs, 2)
	assert.Contains(t, msgs[1], "recovered")
	assert.Contains(t, msgs[1], "5m0s")
}

func TestDBMonitor_SuccessWhileHealthyIsSilent(t *testing.T) {
	m, _, w := newMonitor()
	m.Success()
	m.Success()
	assert.Empty(t, w.Messages())
}

func TestDBMonitor_NewOutageAfterRecoveryPostsAgainAfterTheHour(t *testing.T) {
	m, c, w := newMonitor()
	m.Failure(errDown)
	m.Success()
	c.Advance(2 * time.Hour)
	m.Failure(errDown)
	msgs := w.Messages()
	require.Len(t, msgs, 3)
	assert.Contains(t, msgs[2], "Database failing", "a fresh outage reads as new, not 'still failing'")
}

// Flapping must not turn into a message per flap, and must not announce a
// recovery from an outage the owner was never told about.
func TestDBMonitor_FlappingDoesNotSpam(t *testing.T) {
	m, c, w := newMonitor()
	m.Failure(errDown) // posts
	m.Success()        // posts "recovered"
	require.Len(t, w.Messages(), 2)
	for range 5 {
		c.Advance(time.Minute)
		m.Failure(errDown) // inside the hour: suppressed
		m.Success()        // owner's last message was "recovered": silent
	}
	assert.Len(t, w.Messages(), 2)
}

func TestDBMonitor_RecoveryAfterSuppressedFailureStaysQuietUntilTold(t *testing.T) {
	m, c, w := newMonitor()
	m.Failure(errDown)
	m.Success()
	c.Advance(time.Minute)
	m.Failure(errDown) // suppressed
	m.Success()        // silent: no matching failure message
	c.Advance(time.Hour)
	m.Failure(errDown) // an hour on: posts
	m.Success()        // now the owner was told, so recovery posts
	msgs := w.Messages()
	require.Len(t, msgs, 4)
	assert.Contains(t, msgs[2], "Database failing")
	assert.Contains(t, msgs[3], "recovered")
}

func TestDBMonitor_ErrorTextIsTrimmed(t *testing.T) {
	m, _, w := newMonitor()
	m.Failure(errors.New(strings.Repeat("x", 5000) + "`"))
	require.Len(t, w.Messages(), 1)
	assert.Less(t, len(w.Messages()[0]), 600)
}

func TestDBMonitor_ConcurrentUse(t *testing.T) {
	m, _, w := newMonitor()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			if i%2 == 0 {
				m.Failure(errDown)
			} else {
				m.Success()
			}
		})
	}
	wg.Wait()
	assert.LessOrEqual(t, len(w.Messages()), 2) // one failure, at most one recovery
}

func TestIsDBFailure(t *testing.T) {
	pg := func(code string) error { return &pgconn.PgError{Code: code} }
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"no rows", pgx.ErrNoRows, false},
		{"unique violation", pg("23505"), false},
		{"syntax error", pg("42601"), false},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"wrapped canceled", fmt.Errorf("x: %w", context.Canceled), false},
		{"query canceled by client", pg("57014"), false},
		{"admin shutdown", pg("57P01"), true},
		{"cannot connect now", pg("57P03"), true},
		{"too many connections", pg("53300"), true},
		{"disk full", pg("53100"), true},
		{"connection failure", pg("08006"), true},
		{"connect error", &pgconn.ConnectError{}, true},
		{"EOF", io.EOF, true},
		{"unexpected EOF", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), true},
		{"net error", &net.OpError{Op: "read", Err: errors.New("reset by peer")}, true},
		{"random app error", errors.New("scan: wrong type"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, IsDBFailure(c.err))
		})
	}
}

func TestIsConnectFailure(t *testing.T) {
	assert.False(t, IsConnectFailure(nil))
	assert.False(t, IsConnectFailure(context.Canceled))
	assert.True(
		t,
		IsConnectFailure(&pgconn.PgError{Code: "28P01"}),
		"even a login refusal (quota, bad password) is an outage",
	)
	assert.True(t, IsConnectFailure(errDown))
}

type recordingTracer struct{ starts, ends int }

func (r *recordingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	r.starts++
	return ctx
}
func (r *recordingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) { r.ends++ }

// The tracer is the glue between pgx and the monitor: check each kind of
// outcome reaches the right monitor method.
func TestDBTracer_RoutesOutcomes(t *testing.T) {
	m, _, w := newMonitor()
	inner := &recordingTracer{}
	tr := DBTracer{Monitor: m, Inner: inner}
	ctx := context.Background()

	end := func(err error) { tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: err}) }

	tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
	end(pgx.ErrNoRows)
	end(&pgconn.PgError{Code: "23505"})
	assert.Empty(t, w.Messages(), "ordinary outcomes from a working database are silent")

	tr.TraceConnectEnd(ctx, pgx.TraceConnectEndData{Err: errDown})
	require.Len(t, w.Messages(), 1, "a failed connection attempt is an outage")

	end(context.Canceled)
	assert.Len(t, w.Messages(), 1, "a cancelled request is neither failure nor recovery")

	end(pgx.ErrNoRows) // "no rows" proves the database answered
	require.Len(t, w.Messages(), 2)
	assert.Contains(t, w.Messages()[1], "recovered")

	end(io.ErrUnexpectedEOF)
	assert.Equal(t, 1, inner.starts)
	assert.Equal(t, 5, inner.ends, "dev counter still sees every query")
}
