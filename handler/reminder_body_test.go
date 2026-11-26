package handler

// reminder_body_test.go — the wording of a "waiting on you" push, by table
// size. Pure: no database, so the names come straight from reminderSeat.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// seats builds a roster with ids 1..n in the order given, so a test can pick
// any seat as the recipient.
func seats(names ...string) []reminderSeat {
	out := make([]reminderSeat, 0, len(names))
	for i, name := range names {
		out = append(out, reminderSeat{id: int64(i + 1), name: name})
	}
	return out
}

func TestReminderBody_TwoPlayers_NamesTheOnlyTablemate(t *testing.T) {
	table := seats("otto", "wren")

	assert.Equal(t, "The game is waiting on you at the table with wren", reminderBody(1, table))
	assert.Equal(t, "The game is waiting on you at the table with otto", reminderBody(2, table))
}

func TestReminderBody_ThreePlayers_NamesBothTablemates(t *testing.T) {
	table := seats("otto", "wren", "mae")

	// The recipient is excluded by id, wherever they sit in the roster.
	assert.Equal(t, "The game is waiting on you at the table with wren and mae", reminderBody(1, table))
	assert.Equal(t, "The game is waiting on you at the table with otto and mae", reminderBody(2, table))
	assert.Equal(t, "The game is waiting on you at the table with otto and wren", reminderBody(3, table))
}

func TestReminderBody_FivePlayers_NamesTwoThenCountsTheRest(t *testing.T) {
	table := seats("otto", "wren", "mae", "ivo", "sol")

	assert.Equal(t,
		"The game is waiting on you at the table with otto, wren and 2 others",
		reminderBody(3, table), "recipient in the middle: the other four, two named")
	assert.Equal(t,
		"The game is waiting on you at the table with wren, mae and 2 others",
		reminderBody(1, table), "recipient first: the names shift, the count does not")
}

func TestReminderBody_FourPlayers_SingularOther(t *testing.T) {
	table := seats("otto", "wren", "mae", "ivo")

	// Three tablemates: two named, one left over. "1 other", not "1 others".
	assert.Equal(t,
		"The game is waiting on you at the table with wren, mae and 1 other",
		reminderBody(1, table))
}

func TestReminderBody_NoTablemates_KeepsThePlainCopy(t *testing.T) {
	// A roster that is only the recipient, or none at all (the roster read
	// failed and the caller passed nil), gets the wording that predates names.
	assert.Equal(t, "The game is waiting on you!", reminderBody(1, seats("otto")))
	assert.Equal(t, "The game is waiting on you!", reminderBody(1, nil))
}
