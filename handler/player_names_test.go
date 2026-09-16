package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	dbgen "uneasy/db/gen"
)

// A seeded name must be served without touching the database. The nil
// *Queries makes that structural: any fall-through to GetPlayerByID panics.
func TestPlayerPlainName_UsesSeededNamesWithoutDB(t *testing.T) {
	ctx := withKnownPlayers(context.Background(),
		dbgen.Player{ID: 7, DisplayName: "Mara"},
		dbgen.Player{ID: 9, DisplayName: "Teodor"},
	)
	assert.Equal(t, "Mara", playerPlainName(ctx, nil, 7))
	assert.Equal(t, "Teodor", playerPlainName(ctx, nil, 9))
	assert.Equal(t, "@@7|Mara@@", playerDisplayName(ctx, nil, 7), "marked form reads the cache too")
}

func TestWithKnownPlayers_LayersAndIsolates(t *testing.T) {
	base := withKnownPlayers(context.Background(), dbgen.Player{ID: 1, DisplayName: "One"})
	layered := withKnownPlayers(base, dbgen.Player{ID: 2, DisplayName: "Two"})

	name, ok := knownPlayerName(layered, 1)
	assert.True(t, ok)
	assert.Equal(t, "One", name, "a later seeding keeps the earlier names")

	_, ok = knownPlayerName(base, 2)
	assert.False(t, ok, "seeding a child context must not leak into the parent")

	_, ok = knownPlayerName(context.Background(), 1)
	assert.False(t, ok)

	assert.Equal(t, base, withKnownPlayers(base), "seeding nothing returns the same context")
}
