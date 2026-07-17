//go:build integration

// handler/accounts_tables_integration_test.go — coverage for the enriched
// GET /api/accounts/me/tables response the profile page's table cards render:
// phase, full roster in join order, wait state, and account-level presence.

package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

func TestListMyTablesEnrichment(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)

	tg := newTestGame(t, q, 3)
	viewer := tg.Players[0]

	// Mark players[1]'s account online by giving it a live hub client — the
	// same registration path a real WebSocket connection takes.
	m := hub.NewManager()
	h := m.GetOrCreate(tg.Game.ID)
	c := hub.NewClient(h, nil, tg.Players[1], slog.Default())
	require.True(t, h.Register(c))
	require.Eventually(t, func() bool { return m.IsAccountOnline(tg.Players[1].AccountID) },
		2*time.Second, 5*time.Millisecond, "registered client never showed online")

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/me/tables", nil)
	req = req.WithContext(appMiddleware.AccountContext(req.Context(), &appMiddleware.Account{ID: viewer.AccountID}))
	w := httptest.NewRecorder()
	ListMyTables(store, m)(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Tables []struct {
			GameID   int64  `json:"game_id"`
			JoinCode string `json:"join_code"`
			Phase    string `json:"phase"`
			PlayerID int64  `json:"player_id"`
			Players  []struct {
				ID          int64   `json:"id"`
				DisplayName string  `json:"display_name"`
				TokenColor  *string `json:"token_color"`
				SeatOrder   *int16  `json:"seat_order"`
				Online      bool    `json:"online"`
			} `json:"players"`
			WaitingOn []int64 `json:"waiting_on_player_ids"`
		} `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Tables, 1)
	tbl := resp.Tables[0]
	require.Equal(t, tg.Game.ID, tbl.GameID)
	require.Equal(t, "main_event", tbl.Phase)
	require.Equal(t, viewer.ID, tbl.PlayerID, "player_id must be the viewer's own seat")
	require.NotNil(t, tbl.WaitingOn, "waiting_on_player_ids must be an array, not null")

	// Roster comes back in join order — facilitator (players[0]) first.
	require.Len(t, tbl.Players, 3)
	for i, p := range tg.Players {
		require.Equal(t, p.ID, tbl.Players[i].ID)
		require.Equal(t, p.DisplayName, tbl.Players[i].DisplayName)
	}

	// Only the account with the live hub client reads as online.
	require.False(t, tbl.Players[0].Online)
	require.True(t, tbl.Players[1].Online)
	require.False(t, tbl.Players[2].Online)
}
