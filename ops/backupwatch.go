package ops

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// BackupMaxAge is how old the last recorded backup may be before the server
// complains. The workflow runs weekly (Mondays 06:00 UTC); 8 days leaves a
// day's slack for a late or retried run without hiding a genuinely missed week.
const BackupMaxAge = 8 * 24 * time.Hour

// BackupCheckHourUTC is the one hour of the day (12:00 to 12:59 UTC) in which
// the check runs. The 30-minute notify ticker lands in it once or twice.
//
// It is a fixed hour of the clock, not "24 hours since the last check", because
// the last-check memory lives in the process and a redeploy wipes it. Main
// deploys often, so a plain "once a day since boot" would post another overdue
// message after every redeploy while a backup is overdue. With a fixed hour, a
// restart outside it can't post at all; only a restart inside the hour can
// repeat the message, which is rare enough to accept.
const BackupCheckHourUTC = 12

// BackupWatch notices that the weekly backup has stopped reporting in. The
// backup workflow writes its finish time to the database (migration 059); this
// reads it back. It exists because GitHub disables scheduled workflows after
// about 60 days without repository activity, and a disabled workflow can't say
// so itself.
//
// It has no timer of its own. The notify ticker already reads the database
// every 30 minutes, so Tick is called from there and does real work only once a
// day, in the BackupCheckHourUTC hour. Because it piggybacks, it adds no
// database wake-ups at all: Neon bills awake time, and a separate daily timer
// would have added one more.
type BackupWatch struct {
	// Fetch returns when the last backup finished, and false when no backup has
	// ever recorded itself (a fresh database, or before the first run after the
	// migration). It is the only thing that touches the database.
	Fetch func(ctx context.Context) (last time.Time, ok bool, err error)
	Now   func() time.Time
	Post  func(msg string)
	Log   *slog.Logger

	lastChecked time.Time
}

// Tick runs the check on ticks that fall in the BackupCheckHourUTC hour, at most
// once per UTC day. Call it from a single goroutine (the notify ticker).
func (b *BackupWatch) Tick(ctx context.Context) {
	now := b.Now().UTC()
	if now.Hour() != BackupCheckHourUTC {
		return
	}
	if !b.lastChecked.IsZero() && sameUTCDay(b.lastChecked, now) {
		return
	}
	last, ok, err := b.Fetch(ctx)
	if err != nil {
		// Don't mark the day as checked: try again next tick. If the database is
		// really down, the database alert already covers it.
		b.Log.WarnContext(ctx, "backup check: read failed", "err", err)
		return
	}
	b.lastChecked = now
	if !ok {
		return // no backup recorded yet: stay quiet until the first one writes a row
	}
	age := now.Sub(last)
	if age <= BackupMaxAge {
		return
	}
	b.Post(fmt.Sprintf("🟠 **Database backup overdue.** The last successful weekly backup was %s (%d days ago). "+
		"GitHub switches scheduled workflows off after about 60 days without repo activity: check "+
		"Actions → db-backup, re-enable it if it says disabled, then run it once by hand (Run workflow).",
		last.UTC().Format("2 Jan 2006"), int(age.Hours()/24)))
}

func sameUTCDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}
