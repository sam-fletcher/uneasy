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

// BackupCheckInterval is how often the check actually reads the database.
const BackupCheckInterval = 24 * time.Hour

// BackupWatch notices that the weekly backup has stopped reporting in. The
// backup workflow writes its finish time to the database (migration 059); this
// reads it back. It exists because GitHub disables scheduled workflows after
// about 60 days without repository activity, and a disabled workflow can't say
// so itself.
//
// It has no timer of its own. The notify ticker already reads the database
// every 30 minutes, so Tick is called from there and does real work only once a
// day. Because it piggybacks, it adds no database wake-ups at all: Neon bills
// awake time, and a separate daily timer would have added one more.
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

// Tick runs the check if a day has passed since the last one. Call it from a
// single goroutine (the notify ticker).
func (b *BackupWatch) Tick(ctx context.Context) {
	now := b.Now()
	if !b.lastChecked.IsZero() && now.Sub(b.lastChecked) < BackupCheckInterval {
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
