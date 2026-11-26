//go:build integration

package handler

// reminder_body_integration_test.go — the roster read behind the reminder
// wording. reminder_body_test.go covers the words; this checks that the
// tablemates really come from the game's seats, with the recipient left out.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	dbgen "uneasy/db/gen"
)

// TestReminderBodyFor_NamesTheSeatedTablemates seeds a five-player table and
// asks for the reminder to the middle seat. The other four are named by
// display name in seat order, two of them, with the rest counted.
func TestReminderBodyFor_NamesTheSeatedTablemates(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	tg := newTestGame(t, q, 5)
	recipient := tg.Players[2]

	got := reminderBodyFor(ctx, q, slog.Default(), recipient.ID, tg.Game.ID)

	want := "The game is waiting on you at the table with " +
		tg.Players[0].DisplayName + ", " + tg.Players[1].DisplayName + " and 2 others"
	assert.Equal(t, want, got)
}
