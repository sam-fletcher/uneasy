package handler

// player_activity.go — "last here 2 days ago" and "reminders off" for the
// Retinue header.
//
// Two halves, both small: TouchActivity records that a player has the table on
// screen (written from deliberate foreground events only — see migration 055
// for why neither hub presence nor the chat read marker can stand in for it),
// and buildPlayerActivity turns the roster's raw settings into the verdict the
// header renders.

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// playerActivityThrottle is how stale last_active_at may get before
// TouchPlayerActivity writes it back.
//
// Same reasoning as middleware's sessionTouchInterval, and the same billing
// concern: the client pings on every tab-visible event, which on a phone is
// frequent, while the header renders coarse buckets where an hour of slack is
// invisible ("last here 3h ago"). The write is a single conditional UPDATE, so
// a throttled call costs one no-op statement and no extra round trip to decide
// it. The smallest bucket the client renders is deliberately wider than this
// interval, so the display can never be more precise than the data.
const playerActivityThrottle = time.Hour

// TouchActivity handles POST /api/tables/{id}/activity.
//
// Called when the table page mounts and when its tab becomes visible again.
// Deliberately its own endpoint rather than a bump inside LoadPlayer: that
// middleware also runs for WebSocket reconnects from background tabs, which is
// exactly the stale-tab signal this column exists to avoid being.
//
// Always 204, including when the throttle skipped the write — the client is
// fire-and-forget and has nothing to do with the answer.
func TouchActivity(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		err := s.Q.TouchPlayerActivity(r.Context(), dbgen.TouchPlayerActivityParams{
			PlayerID:        player.ID,
			ThrottleMinutes: int32(playerActivityThrottle / time.Minute),
		})
		if err != nil {
			respondInternalErr(w, r, "could not record activity", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// timePtr unwraps a nullable Postgres timestamp into the *time.Time the JSON
// shape wants — pgtype.Timestamptz marshals as an object-ish value the
// hand-written frontend types don't model, and every field here is genuinely
// optional.
func timePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// buildPlayerActivity converts ListPlayerActivityByGame rows into the served
// shape. Split out from its caller so the verdict logic is unit-testable
// without a database (mirrors formatDiscordMessage / buildPushPayload).
func buildPlayerActivity(rows []dbgen.ListPlayerActivityByGameRow) []model.PlayerActivity {
	out := make([]model.PlayerActivity, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.PlayerActivity{
			PlayerID:      row.PlayerID,
			LastActiveAt:  timePtr(row.LastActiveAt),
			Reminder:      reminderState(row),
			ReminderDueAt: reminderDueAt(row),
		})
	}
	return out
}

// reminderState collapses the four inputs into one verdict. Order matters,
// and each step is a reason the one below it doesn't get to answer:
//
//   - A NULL cadence makes everything else moot — UpsertPendingNotification's
//     join skips those accounts entirely, so a cadence-off player can never
//     hold a pending row anyway.
//   - No device outranks the timer for the same reason it exists: a player
//     with a perfectly good timer and nothing to send to is the silent
//     failure, and saying "reminder due in 6h" would be the exact false
//     promise this type is documented not to make.
//   - Exhausted outranks scheduled because both are read off the same row.
//     A wait past the give-up horizon keeps its row (and a due_at now in the
//     past) but is no longer selected for sending, so trusting due_at here
//     would announce a ping that is never coming.
func reminderState(row dbgen.ListPlayerActivityByGameRow) model.ReminderState {
	switch {
	case row.NotifyCadenceHours == nil:
		return model.ReminderOff
	case !row.HasPushDevice:
		return model.ReminderNoDevice
	case row.ReminderExhausted:
		return model.ReminderExhausted
	case row.ReminderDueAt.Valid:
		return model.ReminderScheduled
	default:
		return model.ReminderReady
	}
}

// reminderDueAt is non-nil only in the ReminderScheduled case, so the client
// never has to decide whether a due time it can see is meaningful. A player
// with a pending row but no device has a real timer — it just has nothing to
// send to (ListDueNotificationsWithSubscriptions still returns them so the row
// can be re-bumped), and surfacing that as "a reminder is coming" would be a
// promise we can't keep.
func reminderDueAt(row dbgen.ListPlayerActivityByGameRow) *time.Time {
	if reminderState(row) != model.ReminderScheduled {
		return nil
	}
	return timePtr(row.ReminderDueAt)
}
