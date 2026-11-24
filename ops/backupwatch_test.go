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
	c.Advance(2 * time.Hour) // 10:00 -> 12:00 UTC, the hour the check runs
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
	f := &fakeBackups{ok: true}
	b, c, w := newWatch(f)
	f.last = c.Now().Add(-7 * 24 * time.Hour)
	b.Tick(context.Background())
	assert.Empty(t, w.Messages())
}

func TestBackupWatch_StaleBackupPosts(t *testing.T) {
	f := &fakeBackups{ok: true}
	b, c, w := newWatch(f)
	f.last = c.Now().Add(-9 * 24 * time.Hour)
	b.Tick(context.Background())
	msgs := w.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "backup overdue")
	assert.Contains(t, msgs[0], "9 days ago")
	assert.Contains(t, msgs[0], "14 Nov 2026")
}

func TestBackupWatch_ExactlyEightDaysIsStillFine(t *testing.T) {
	f := &fakeBackups{ok: true}
	b, c, w := newWatch(f)
	f.last = c.Now().Add(-BackupMaxAge)
	b.Tick(context.Background())
	assert.Empty(t, w.Messages())
}

// The notify ticker calls Tick every 30 minutes; only the ticks inside the
// 12:00 UTC hour may read the database or post, and only once per day.
func TestBackupWatch_ChecksOncePerDay(t *testing.T) {
	f := &fakeBackups{ok: true}
	b, c, w := newWatch(f)
	f.last = c.Now().Add(-20 * 24 * time.Hour)
	for range 48 { // 24 hours of 30-minute ticks, starting at 12:00
		b.Tick(context.Background())
		c.Advance(30 * time.Minute)
	}
	assert.Equal(t, 1, f.reads, "12:00 and 12:30 are both in the hour; only the first reads")
	assert.Len(t, w.Messages(), 1)
	b.Tick(context.Background()) // next day, 12:00
	assert.Equal(t, 2, f.reads)
	assert.Len(t, w.Messages(), 2, "still overdue tomorrow: one reminder a day")
}

// A redeploy wipes the in-memory guard. Outside the check hour a fresh process
// must stay silent, however overdue the backup is, so frequent deploys can't
// repeat the message.
func TestBackupWatch_RebootOutsideTheHourDoesNotPost(t *testing.T) {
	f := &fakeBackups{ok: true}
	b, c, w := newWatch(f)
	f.last = c.Now().Add(-20 * 24 * time.Hour)
	b.Tick(context.Background()) // 12:00: posts
	require.Len(t, w.Messages(), 1)

	for _, h := range []int{1, 5, 11, 22} { // 13:00, 17:00, 23:00, 10:00 next day
		c.Advance(time.Duration(h) * time.Hour)
		reboot := &BackupWatch{Fetch: f.Fetch, Now: c.Now, Post: w.Post, Log: slog.New(slog.DiscardHandler)}
		reboot.Tick(context.Background())
		reboot.Tick(context.Background())
	}
	assert.Len(t, w.Messages(), 1, "fresh processes outside 12:00-12:59 UTC never read or post")
	assert.Equal(t, 1, f.reads)
}

func TestBackupWatch_ReadErrorRetriesNextTick(t *testing.T) {
	f := &fakeBackups{err: errors.New("db down")}
	b, c, w := newWatch(f)
	b.Tick(context.Background())
	assert.Empty(t, w.Messages(), "a read error is the database alert's job")
	f.err, f.ok, f.last = nil, true, c.Now().Add(-30*24*time.Hour)
	c.Advance(30 * time.Minute) // 12:30, still in the hour
	b.Tick(context.Background())
	assert.Len(t, w.Messages(), 1, "the failed read didn't use up the day")
}

func TestBackupWatch_RecoversOnceBackupReports(t *testing.T) {
	f := &fakeBackups{ok: true}
	b, c, w := newWatch(f)
	f.last = c.Now().Add(-12 * 24 * time.Hour)
	b.Tick(context.Background())
	require.Len(t, w.Messages(), 1)
	c.Advance(24 * time.Hour)
	f.last = c.Now().Add(-time.Hour) // the workflow ran
	b.Tick(context.Background())
	assert.Len(t, w.Messages(), 1)
}
