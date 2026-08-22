//go:build integration

// handler/accounts_tables_integration_test.go — coverage for the enriched
// GET /api/accounts/me/tables response the profile page's table cards render:
// phase, full roster in join order, wait state, account-level presence, and
// the unread count behind the card's "N new" chip, and the two reminder flags
// behind its bell. Plus POST /api/tables/join's lobby-phase gate, which decides
// whether a table appears on a card at all.

package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// TestJoinTablePhaseGate pins ruling R1 of adr/LOBBY_AND_CHECKLIST_PLAN.md: the
// table is sealed the moment the facilitator starts the prologue. Before the
// gate, anyone holding a code could walk into a running game and be seated with
// no rankings row, no prologue history and no plan tokens — and the lobby's
// facilitator copy ("once you start, the table is sealed") would be a lie.
//
// The two clauses that could go wrong in opposite directions both have a case
// here: over-gating (a lobby join refused) and under-gating (a seated player
// locked out of their own running table, which the short-circuit above the gate
// is there to prevent).
func TestJoinTablePhaseGate(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := t.Context()

	// join posts a join-code request as the given account, the way the profile
	// page's join form does.
	join := func(acct dbgen.Account, code string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/tables/join",
			strings.NewReader(`{"join_code":"`+code+`"}`))
		req = req.WithContext(appMiddleware.AccountContext(req.Context(),
			&appMiddleware.Account{ID: acct.ID, Username: acct.Username}))
		w := httptest.NewRecorder()
		JoinTable(store, hub.NewManager())(w, req)
		return w
	}

	// newcomer is an account with no seat anywhere — the attacker/stranger in
	// finding 11, and the honest late arrival in the lobby case.
	newcomer := func() dbgen.Account {
		t.Helper()
		acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: "newcomer-" + randSuffix(), PasswordHash: "x",
		})
		require.NoError(t, err)
		return acct
	}

	// A game in the given phase, seeded as a real two-player main event and
	// then moved — the gate reads nothing but games.phase.
	gameInPhase := func(phase model.GamePhase) testGame {
		t.Helper()
		tg := newTestGame(t, q, 2)
		require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
			ID: tg.Game.ID, Phase: phase,
		}))
		return tg
	}

	// Lobby: joining works. Guards against over-gating — this is the whole
	// point of a join code.
	t.Run("lobby accepts a newcomer", func(t *testing.T) {
		tg := gameInPhase(model.PhaseLobby)
		acct := newcomer()

		rec := join(acct, tg.Game.JoinCode)
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		seated, err := q.GetPlayerByAccountAndGame(ctx, dbgen.GetPlayerByAccountAndGameParams{
			AccountID: acct.ID, GameID: tg.Game.ID,
		})
		require.NoError(t, err, "a 201 must have actually seated the account")
		require.EqualValues(t, 3, *seated.SeatOrder)
	})

	// Every phase past the lobby: 409, and nothing written.
	for _, phase := range []model.GamePhase{
		model.PhasePrologue, model.PhaseMainEvent, model.PhaseShakeUp, model.PhaseEnded,
	} {
		t.Run(string(phase)+" refuses a newcomer", func(t *testing.T) {
			tg := gameInPhase(phase)
			acct := newcomer()

			rec := join(acct, tg.Game.JoinCode)
			require.Equal(t, http.StatusConflict, rec.Code,
				"a %s game must not accept a new player", phase)
			require.Contains(t, rec.Body.String(), "already started",
				"the message reaches the profile page's ErrorText verbatim")

			_, err := q.GetPlayerByAccountAndGame(ctx, dbgen.GetPlayerByAccountAndGameParams{
				AccountID: acct.ID, GameID: tg.Game.ID,
			})
			require.Error(t, err, "the refused join must not have seated anyone")

			players, err := q.GetPlayersByGame(ctx, tg.Game.ID)
			require.NoError(t, err)
			require.Len(t, players, 2, "the roster must be untouched")
		})
	}

	// The short-circuit above the gate: a player already at a running table can
	// still hit the endpoint (the profile page's join form is the same form for
	// everyone) and gets their own seat back, not a lockout.
	t.Run("a seated player re-joins their running table", func(t *testing.T) {
		tg := gameInPhase(model.PhaseMainEvent)
		seated := tg.Players[1]

		acct, err := q.GetAccountByID(ctx, seated.AccountID)
		require.NoError(t, err)

		rec := join(acct, tg.Game.JoinCode)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var resp struct {
			Player struct {
				ID int64 `json:"id"`
			} `json:"player"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Equal(t, seated.ID, resp.Player.ID, "must hand back their existing seat")

		players, err := q.GetPlayersByGame(ctx, tg.Game.ID)
		require.NoError(t, err)
		require.Len(t, players, 2, "a re-join must not create a second seat")
	})
}

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
			WaitingOn   []int64 `json:"waiting_on_player_ids"`
			UnreadCount int64   `json:"unread_count"`
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

	require.Zero(t, tbl.UnreadCount, "a game with no posts owes the viewer nothing to read")
}

// TestListMyTablesUnreadCount pins each clause of the unread rule that the
// profile card's "N new" chip counts on. The rule is defined twice by
// necessity — CountUnreadPostsByAccount (SQL) and isUnreadPost (chatFeed.ts) —
// so this is the test that keeps the server half honest; its frontend
// counterpart lives in chatFeed.test.ts.
func TestListMyTablesUnreadCount(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := t.Context()

	tg := newTestGame(t, q, 3)
	viewer, other := tg.Players[0], tg.Players[1]

	post := func(author *int64, severity int32, code string) dbgen.ScenePost {
		t.Helper()
		if author != nil {
			p, err := q.CreatePlayerMessage(ctx, dbgen.CreatePlayerMessageParams{
				GameID: tg.Game.ID, AuthorID: author, Body: "hello",
			})
			require.NoError(t, err)
			return p
		}
		p, err := q.CreateSystemPost(ctx, dbgen.CreateSystemPostParams{
			GameID: tg.Game.ID, Body: "a thing happened",
			Severity: severity, SystemCode: &code,
		})
		require.NoError(t, err)
		return p
	}

	unreadFor := func(player dbgen.Player) int64 {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/accounts/me/tables", nil)
		req = req.WithContext(appMiddleware.AccountContext(req.Context(),
			&appMiddleware.Account{ID: player.AccountID}))
		w := httptest.NewRecorder()
		ListMyTables(store, hub.NewManager())(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Tables []struct {
				UnreadCount int64 `json:"unread_count"`
			} `json:"tables"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Tables, 1)
		return resp.Tables[0].UnreadCount
	}

	// Someone else's message counts.
	post(&other.ID, 0, "")
	require.EqualValues(t, 1, unreadFor(viewer))

	// The viewer's own message never counts against them.
	post(&viewer.ID, 0, "")
	require.EqualValues(t, 1, unreadFor(viewer))

	// Bookkeeping-tier system posts are below the "hide bookkeeping" bar and
	// are excluded — the bug the shared rule exists to prevent is a badge
	// inflated by posts the feed won't even show.
	post(nil, model.SeverityMinor, "asset.refreshed")
	post(nil, model.SeverityTrace, "marginalia.edited")
	require.EqualValues(t, 1, unreadFor(viewer))

	// Default-and-up system posts do count.
	newest := post(nil, model.SeverityImportant, "plan.resolved.make")
	require.EqualValues(t, 2, unreadFor(viewer))

	// The other player sees their own message excluded but the viewer's
	// counted — the count is per-player, not per-game.
	require.EqualValues(t, 2, unreadFor(other))

	// Advancing the read marker clears it.
	_, err := q.UpdateReadMarker(ctx, dbgen.UpdateReadMarkerParams{
		PlayerID: viewer.ID, GameID: tg.Game.ID, RequestedID: newest.ID,
	})
	require.NoError(t, err)
	require.Zero(t, unreadFor(viewer))
}

// TestListMyTablesAcrossTables is the test the batching needs: the handler now
// fetches every roster in one ListPlayersByGames and every unread count in one
// CountUnreadPostsByAccount, then redistributes both by key. The single-table
// tests above cannot fail on a mis-keyed grouping — with one table, any
// grouping is the right one. This one gives each table a different roster size
// and a different unread count, so a card wearing another table's numbers is
// unmissable.
func TestListMyTablesAcrossTables(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := t.Context()

	// Three tables with deliberately different roster sizes. The viewer already
	// sits at the first; seat them at the other two so one account spans all
	// three, which is the shape the profile page actually loads.
	tgA := newTestGame(t, q, 2)
	tgB := newTestGame(t, q, 3)
	tgC := newTestGame(t, q, 4)
	viewer := tgA.Players[0]

	joinAs := func(tg testGame) dbgen.Player {
		t.Helper()
		p, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        tg.Game.ID,
			DisplayName:   viewer.DisplayName,
			AccountID:     viewer.AccountID,
			IsFacilitator: false,
		})
		require.NoError(t, err)
		return p
	}
	seatB := joinAs(tgB)
	seatC := joinAs(tgC)

	// Distinct unread counts per table, all authored by somebody else so every
	// post counts: A=1, B=3, C=0.
	say := func(tg testGame, author int64, n int) {
		t.Helper()
		for range n {
			_, err := q.CreatePlayerMessage(ctx, dbgen.CreatePlayerMessageParams{
				GameID: tg.Game.ID, AuthorID: &author, Body: "hello",
			})
			require.NoError(t, err)
		}
	}
	say(tgA, tgA.Players[1].ID, 1)
	say(tgB, tgB.Players[1].ID, 3)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts/me/tables", nil)
	req = req.WithContext(appMiddleware.AccountContext(req.Context(),
		&appMiddleware.Account{ID: viewer.AccountID}))
	w := httptest.NewRecorder()
	ListMyTables(store, hub.NewManager())(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Tables []struct {
			GameID   int64 `json:"game_id"`
			PlayerID int64 `json:"player_id"`
			Players  []struct {
				ID int64 `json:"id"`
			} `json:"players"`
			WaitingOn   []int64 `json:"waiting_on_player_ids"`
			UnreadCount int64   `json:"unread_count"`
		} `json:"tables"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Tables, 3)

	byGame := make(map[int64]int)
	for i, tbl := range resp.Tables {
		byGame[tbl.GameID] = i
	}
	for _, id := range []int64{tgA.Game.ID, tgB.Game.ID, tgC.Game.ID} {
		require.Containsf(t, byGame, id, "game %d missing from the table list", id)
	}

	check := func(tg testGame, seat dbgen.Player, wantRoster int, wantUnread int64) {
		t.Helper()
		tbl := resp.Tables[byGame[tg.Game.ID]]
		require.Equalf(t, seat.ID, tbl.PlayerID, "game %d: wrong seat for the viewer", tg.Game.ID)
		require.Lenf(t, tbl.Players, wantRoster, "game %d: roster came from another table", tg.Game.ID)
		require.EqualValuesf(t, wantUnread, tbl.UnreadCount,
			"game %d: unread count came from another table", tg.Game.ID)
		// Every id on the card belongs to this game — the check a roster
		// grouped by the wrong key fails even when the size happens to match.
		roster, err := q.GetPlayersByGame(ctx, tg.Game.ID)
		require.NoError(t, err)
		want := make(map[int64]bool, len(roster))
		for _, p := range roster {
			want[p.ID] = true
		}
		for _, p := range tbl.Players {
			require.Truef(t, want[p.ID], "game %d: player %d is not at this table", tg.Game.ID, p.ID)
		}
		require.NotNilf(t, tbl.WaitingOn, "game %d: waiting_on_player_ids must be an array", tg.Game.ID)
	}
	check(tgA, viewer, 2, 1)
	check(tgB, seatB, 4, 3) // 3 seeded + the viewer
	check(tgC, seatC, 5, 0) // 4 seeded + the viewer, nothing posted
}

// TestListMyTablesReminderFlags: the profile card's bell needs both reasons a
// table it is waiting on might still be sending nothing — the player silenced
// this wait, or the give-up horizon ended it. They are independent, so the card
// gets both rather than one collapsed verdict.
func TestListMyTablesReminderFlags(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()

	tg := newTestGame(t, q, 2)
	viewer := tg.Players[0] // scene_setting focus player, per seedBase
	require.NoError(t, reconcileWaitees(ctx, q, tg.Game.ID))

	read := func() (muted, exhausted bool) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/accounts/me/tables", nil)
		req = req.WithContext(appMiddleware.AccountContext(req.Context(), &appMiddleware.Account{ID: viewer.AccountID}))
		w := httptest.NewRecorder()
		ListMyTables(store, hub.NewManager())(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Tables []struct {
				ReminderMuted     bool `json:"reminder_muted"`
				ReminderExhausted bool `json:"reminder_exhausted"`
			} `json:"tables"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Tables, 1)
		return resp.Tables[0].ReminderMuted, resp.Tables[0].ReminderExhausted
	}

	muted, exhausted := read()
	assert.False(t, muted)
	assert.False(t, exhausted, "a wait that just started is neither silenced nor given up on")

	_, err := q.SetPendingNotificationMuted(ctx, dbgen.SetPendingNotificationMutedParams{
		PlayerID: viewer.ID, Muted: true,
	})
	require.NoError(t, err)
	muted, _ = read()
	assert.True(t, muted)

	_, execErr := pool.Exec(ctx,
		`UPDATE pending_notifications SET first_waiting_at = now() - make_interval(days => $2)
		 WHERE player_id = $1`, viewer.ID, reminderGiveUpDays+1)
	require.NoError(t, execErr)
	muted, exhausted = read()
	assert.True(t, muted)
	assert.True(t, exhausted, "both can hold at once — muting a wait that already gave up is allowed")
}
