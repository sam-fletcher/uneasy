package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// RepeatInterval is the longest the monitor stays quiet while the database
// keeps failing: one reminder an hour, no more.
const RepeatInterval = time.Hour

// maxErrChars trims the error text put in the message so one pathological
// error can't eat the whole Discord message.
const maxErrChars = 300

// DBMonitor turns a stream of "this database request failed" and "this one
// worked" signals into a few Discord messages:
//
//   - the first failure posts one message;
//   - while failures continue it posts at most one more per RepeatInterval;
//   - the first success after a failure posts one "recovered" message, but only
//     if the owner's last message was the failure (see notified below).
//
// Flapping (fail, work, fail, work) can't spam: failure messages are limited to
// one per RepeatInterval in total, not one per outage, and a recovery that
// follows a suppressed failure stays quiet so the owner never reads "recovered"
// for an outage they weren't told about.
//
// It is safe for concurrent use: every request goroutine feeds it.
type DBMonitor struct {
	now  func() time.Time
	post func(msg string)

	// failing is the fast path. Success is reported on every query that works,
	// so it must cost one atomic load when nothing is wrong.
	failing atomic.Bool

	mu        sync.Mutex
	since     time.Time // when the current outage's first failure was seen
	count     int       // failures seen this outage
	alerts    int       // failure messages sent this outage
	lastAlert time.Time // when a failure message last went out, across outages
	notified  bool      // the owner's latest message is a failure, not yet a recovery
}

// NewDBMonitor builds a monitor. The monitor calls post from request
// goroutines, so post must return at once (production passes a function that
// starts a goroutine; see handler.PostOpsAlert). A slow or failing Discord must
// never hold up a request.
func NewDBMonitor(now func() time.Time, post func(msg string)) *DBMonitor {
	return &DBMonitor{now: now, post: post}
}

// Failure records a failed database request. err is shown (trimmed) in the
// message so the owner can tell "connection refused" from "quota exceeded".
func (m *DBMonitor) Failure(err error) {
	if msg := m.recordFailure(err); msg != "" {
		m.post(msg)
	}
}

// recordFailure updates the state and returns the message to send, or "".
func (m *DBMonitor) recordFailure(err error) string {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.failing.Load() {
		m.since = now
		m.count = 0
		m.alerts = 0
		m.failing.Store(true)
	}
	m.count++
	if !m.lastAlert.IsZero() && now.Sub(m.lastAlert) < RepeatInterval {
		return ""
	}
	m.lastAlert = now
	m.notified = true
	var msg string
	if m.alerts == 0 {
		msg = fmt.Sprintf("🔴 **Database failing.** Requests to the database started erroring at %s. Latest error: `%s`",
			m.since.UTC().Format("15:04 UTC"), trimErr(err))
	} else {
		msg = fmt.Sprintf("🔴 **Database still failing** after %s (%d failed requests so far). Latest error: `%s`",
			roundDuration(now.Sub(m.since)), m.count, trimErr(err))
	}
	m.alerts++
	return msg
}

// Success records a database request that worked. Cheap when nothing is wrong.
func (m *DBMonitor) Success() {
	if !m.failing.Load() {
		return
	}
	if msg := m.recordSuccess(); msg != "" {
		m.post(msg)
	}
}

func (m *DBMonitor) recordSuccess() string {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.failing.Load() {
		return ""
	}
	m.failing.Store(false)
	if !m.notified {
		return ""
	}
	m.notified = false
	return fmt.Sprintf("🟢 **Database recovered.** Requests are working again after %s.",
		roundDuration(now.Sub(m.since)))
}

func trimErr(err error) string {
	if err == nil {
		return "unknown"
	}
	s := strings.ReplaceAll(err.Error(), "`", "'")
	r := []rune(s)
	if len(r) > maxErrChars {
		return string(r[:maxErrChars-1]) + "…"
	}
	return s
}

func roundDuration(d time.Duration) string {
	if d < time.Minute {
		return "under a minute"
	}
	return d.Round(time.Minute).String()
}

// IsDBFailure reports whether err from a *query* means the database itself is
// in trouble, as opposed to a normal outcome of a working database (no rows, a
// unique-constraint violation, a bad cast, a cancelled request). Getting this
// wrong in the loose direction would page the owner for ordinary 404s and 409s.
//
// Connection errors are handled separately by IsConnectFailure: any error while
// opening a connection counts, because a server that is up and answers a login
// with "quota exceeded" is still an outage for the app.
func IsDBFailure(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[*pgconn.ConnectError](err); ok {
		return true
	}
	// The caller went away or its deadline passed. That says nothing about the
	// database (a slow request is not a down database).
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return isServerTroubleCode(pgErr.Code)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

// IsConnectFailure reports whether err from opening a connection counts as an
// outage: everything except the caller giving up.
func IsConnectFailure(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled)
}

// isServerTroubleCode picks the SQLSTATE classes that mean "the server is
// unwell" rather than "your query was wrong": 08 connection exception, 53
// insufficient resources (too many connections, disk full, out of memory), 57
// operator intervention (shutdown, "cannot connect now") except 57014, which is
// the caller cancelling its own query, and 58 system error.
func isServerTroubleCode(code string) bool {
	if code == "57014" {
		return false
	}
	switch {
	case strings.HasPrefix(code, "08"),
		strings.HasPrefix(code, "53"),
		strings.HasPrefix(code, "57"),
		strings.HasPrefix(code, "58"):
		return true
	}
	return false
}
