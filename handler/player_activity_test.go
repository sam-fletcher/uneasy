package handler

// player_activity_test.go — DB-free coverage for the reminder verdict.
// The endpoint, the throttle, and the query behind it need a database and
// live in player_activity_integration_test.go (same split as
// push_notifications_test.go vs its integration file).

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// The one cadence value these tests need. A *int16 rather than a helper
// returning one: `go fix` rewrites such a helper into `new(h)` and stamps it
// with a //go:fix directive that golangci-lint then rejects.
var cadence24 = func() *int16 { h := int16(24); return &h }()

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func TestReminderState_VerdictPerInputCombination(t *testing.T) {
	due := time.Now().Add(6 * time.Hour)

	cases := []struct {
		name string
		row  dbgen.ListPlayerActivityByGameRow
		want model.ReminderState
	}{
		{
			name: "no cadence is off, whatever else is true",
			row: dbgen.ListPlayerActivityByGameRow{
				NotifyCadenceHours: nil, HasPushDevice: true,
			},
			want: model.ReminderOff,
		},
		{
			name: "cadence but no subscribed device is the silent failure",
			row: dbgen.ListPlayerActivityByGameRow{
				NotifyCadenceHours: cadence24, HasPushDevice: false,
			},
			want: model.ReminderNoDevice,
		},
		{
			name: "cadence, device, and a pending row is scheduled",
			row: dbgen.ListPlayerActivityByGameRow{
				NotifyCadenceHours: cadence24, HasPushDevice: true, ReminderDueAt: ts(due),
			},
			want: model.ReminderScheduled,
		},
		{
			name: "cadence and device but nothing pending is ready",
			row: dbgen.ListPlayerActivityByGameRow{
				NotifyCadenceHours: cadence24, HasPushDevice: true,
			},
			want: model.ReminderReady,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, reminderState(tc.row))
		})
	}
}

// A player with a pending timer but no device really does have a row — the
// tick still lists them so it can re-bump — but nothing can be sent to them.
// Reporting "a reminder is due" there would promise a ping that cannot
// happen, which is the exact confusion the no_device state exists to clear up.
func TestReminderDueAt_SuppressedWhenNothingCanBeSent(t *testing.T) {
	row := dbgen.ListPlayerActivityByGameRow{
		NotifyCadenceHours: cadence24,
		HasPushDevice:      false,
		ReminderDueAt:      ts(time.Now().Add(time.Hour)),
	}
	assert.Equal(t, model.ReminderNoDevice, reminderState(row))
	assert.Nil(t, reminderDueAt(row))
}

func TestBuildPlayerActivity_MapsRowsAndNullsThrough(t *testing.T) {
	seen := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Second)
	due := time.Now().Add(90 * time.Minute).UTC().Truncate(time.Second)

	out := buildPlayerActivity([]dbgen.ListPlayerActivityByGameRow{
		{
			PlayerID:           7,
			LastActiveAt:       ts(seen),
			NotifyCadenceHours: cadence24,
			HasPushDevice:      true,
			ReminderDueAt:      ts(due),
		},
		{
			// Never opened the table, notifications off — both nulls must
			// survive as nulls so the client can say "not arrived yet"
			// instead of rendering the zero time.
			PlayerID:           8,
			NotifyCadenceHours: nil,
		},
	})

	if assert.Len(t, out, 2) {
		assert.Equal(t, int64(7), out[0].PlayerID)
		if assert.NotNil(t, out[0].LastActiveAt) {
			assert.Equal(t, seen, out[0].LastActiveAt.UTC())
		}
		assert.Equal(t, model.ReminderScheduled, out[0].Reminder)
		if assert.NotNil(t, out[0].ReminderDueAt) {
			assert.Equal(t, due, out[0].ReminderDueAt.UTC())
		}

		assert.Equal(t, int64(8), out[1].PlayerID)
		assert.Nil(t, out[1].LastActiveAt)
		assert.Equal(t, model.ReminderOff, out[1].Reminder)
		assert.Nil(t, out[1].ReminderDueAt)
	}
}
