package ops

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBackups struct {
	last  time.Time
	ok    bool
	err   error
	reads int
}

func (f *fakeBackups) Fetch(context.Context) (time.Time, bool, error) {
	f.reads++
	return f.last, f.ok, f.err
}

func newWatch(f *fakeBackups) (*BackupWatch, *fakeClock, *fakeWebhook) {
	c, w := newFakeClock(), &fakeWebhook{}
	return &BackupWatch{Fetch: f.Fetch, Now: c.Now, Post: w.Post, Log: slog.New(slog.DiscardHandler)}, c, w
}

func TestBackupWatch_NoRowStaysQuiet(t *testing.T) {
	f := &fakeBackups{ok: false}
	b, c, w := newWatch(f)
	b.Tick(context.Background())
	c.Advance(30 * 24 * time.Hour)
	b.Tick(context.Background())
	assert.Empty(t, w.Messages(), "no backup has ever recorded itself, so there is nothing to call overdue")
}

func TestBackupWatch_FreshBackupIsQuiet(t *testing.T) {
	c0 := newFakeClock().Now()
	f := &fakeBackups{ok: true, last: c0.Add(-7 * 24 * time.Hour)}
	b, _, w := newWatch(f)
	b.Tick(context.Background())
	assert.Empty(t, w.Messages())
}

func TestBackupWatch_StaleBackupPosts(t *testing.T) {
	c0 := newFakeClock().Now()
	f := &fakeBackups{ok: true, last: c0.Add(-9 * 24 * time.Hour)}
	b, _, w := newWatch(f)
	b.Tick(context.Background())
	msgs := w.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "backup overdue")
	assert.Contains(t, msgs[0], "9 days ago")
	assert.Contains(t, msgs[0], "14 Nov 2026")
}

func TestBackupWatch_ExactlyEightDaysIsStillFine(t *testing.T) {
	c0 := newFakeClock().Now()
	f := &fakeBackups{ok: true, last: c0.Add(-BackupMaxAge)}
	b, _, w := newWatch(f)
	b.Tick(context.Background())
	assert.Empty(t, w.Messages())
}

// The notify ticker calls Tick every 30 minutes; only one call a day may read
// the database or post.
func TestBackupWatch_ChecksOncePerDay(t *testing.T) {
	c0 := newFakeClock().Now()
	f := &fakeBackups{ok: true, last: c0.Add(-20 * 24 * time.Hour)}
	b, c, w := newWatch(f)
	for range 47 { // 23.5 hours of 30-minute ticks
		b.Tick(context.Background())
		c.Advance(30 * time.Minute)
	}
	assert.Equal(t, 1, f.reads)
	assert.Len(t, w.Messages(), 1)
	c.Advance(time.Hour) // past the day
	b.Tick(context.Background())
	assert.Equal(t, 2, f.reads)
	assert.Len(t, w.Messages(), 2, "still overdue tomorrow: one reminder a day")
}

func TestBackupWatch_ReadErrorRetriesNextTick(t *testing.T) {
	c0 := newFakeClock().Now()
	f := &fakeBackups{err: errors.New("db down")}
	b, c, w := newWatch(f)
	b.Tick(context.Background())
	assert.Empty(t, w.Messages(), "a read error is the database alert's job")
	f.err, f.ok, f.last = nil, true, c0.Add(-30*24*time.Hour)
	c.Advance(30 * time.Minute)
	b.Tick(context.Background())
	assert.Len(t, w.Messages(), 1, "the failed read didn't use up the day")
}

func TestBackupWatch_RecoversOnceBackupReports(t *testing.T) {
	c0 := newFakeClock().Now()
	f := &fakeBackups{ok: true, last: c0.Add(-12 * 24 * time.Hour)}
	b, c, w := newWatch(f)
	b.Tick(context.Background())
	require.Len(t, w.Messages(), 1)
	c.Advance(25 * time.Hour)
	f.last = c.Now().Add(-time.Hour) // the workflow ran
	b.Tick(context.Background())
	assert.Len(t, w.Messages(), 1)
}
