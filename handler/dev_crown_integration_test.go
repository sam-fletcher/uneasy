//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
)

func postCrown(t *testing.T, store *db.Store, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	blob, err := json.Marshal(body)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	DevCrown(store, hub.NewManager()).ServeHTTP(rec,
		httptest.NewRequest(http.MethodPost, "/api/dev/crown", bytes.NewReader(blob)))
	return rec
}

// TestDevCrown_SeatsAMonarch proves the shortcut produces the same state a
// Prologue title claim does: the throne is established and currentMonarch
// resolves to the crowned player.
func TestDevCrown_SeatsAMonarch(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	tg := newTestGame(t, q, 3)
	ctx := context.Background()

	_, _, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.False(t, ok, "no monarch before the crown")

	rec := postCrown(t, store, map[string]any{"game_id": tg.Game.ID, "player_id": tg.Players[2].ID})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	_, ownerID, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, tg.Players[2].ID, ownerID)
	game, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.True(t, game.ThroneEstablished)
}

func TestDevCrown_Refusals(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	a := newTestGame(t, q, 2)
	b := newTestGame(t, q, 2)

	assert.Equal(t, http.StatusBadRequest, postCrown(t, store, map[string]any{"game_id": a.Game.ID}).Code)
	assert.Equal(t, http.StatusNotFound,
		postCrown(t, store, map[string]any{"game_id": 999999999, "player_id": a.Players[0].ID}).Code)
	// A player from another game can't be crowned here.
	rec := postCrown(t, store, map[string]any{"game_id": a.Game.ID, "player_id": b.Players[0].ID})
	assert.Equal(t, http.StatusNotFound, rec.Code, strconv.Itoa(rec.Code))
}
