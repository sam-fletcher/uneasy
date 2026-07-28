//go:build integration

package handler

// Coverage for requireFacilitator (phases.go), which gates the only two
// facilitator-only routes left in the app: POST /tables/{id}/start-prologue and
// POST /tables/{id}/endgame. Both gates were previously asserted nowhere — see
// adr/FACILITATOR_POWERS_AUDIT.md, which found the flag's remaining uses and
// recommended keeping exactly these two.
//
// Each route gets both halves: a non-facilitator is refused with 403 AND the
// facilitator succeeds, so a guard that rejected everyone would still fail.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// sessionCookieFor opens a session for a player's account and returns the
// cookie the middleware authenticates with.
func sessionCookieFor(t *testing.T, q *dbgen.Queries, accountID int64) *http.Cookie {
	t.Helper()
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: accountID,
	})
	require.NoError(t, err)
	return &http.Cookie{Name: "player_token", Value: tok}
}

// postAs issues a POST to path on r, authenticated as the given cookie.
func postAs(r chi.Router, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest("POST", path, nil)
	} else {
		req = httptest.NewRequest("POST", path, strings.NewReader(body))
	}
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// newLobbyGame seats n players in a fresh lobby-phase game (games.phase
// defaults to 'lobby', migration 002). The first player is the facilitator,
// mirroring how JoinTable/CreateTable seat a real table. newTestGame can't be
// used here — it seeds main_event, which start-prologue rejects outright.
func newLobbyGame(t *testing.T, q *dbgen.Queries, n int) (dbgen.Game, []dbgen.Player) {
	t.Helper()
	ctx := context.Background()

	game, err := q.CreateGame(ctx, "lobby-"+randSuffix())
	require.NoError(t, err)
	require.Equal(t, model.PhaseLobby, game.Phase, "a fresh game starts in the lobby")

	players := make([]dbgen.Player, 0, n)
	for i := range n {
		acct, aErr := q.CreateAccount(ctx, dbgen.CreateAccountParams{
			Username: "lobby-" + strconv.Itoa(i) + "-" + randSuffix(), PasswordHash: "x",
		})
		require.NoError(t, aErr)
		p, pErr := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   "P" + strconv.Itoa(i+1),
			AccountID:     acct.ID,
			IsFacilitator: i == 0,
		})
		require.NoError(t, pErr)
		players = append(players, p)
	}
	return game, players
}

// ── StartPrologue ────────────────────────────────────────────────────────────

func TestStartPrologue_RejectsNonFacilitator(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	game, players := newLobbyGame(t, q, 2)

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/start-prologue", StartPrologue(db.NewStore(pool), hub.NewManager()))

	path := "/api/tables/" + strconv.FormatInt(game.ID, 10) + "/start-prologue"
	rec := postAs(r, path, "", sessionCookieFor(t, q, players[1].AccountID))

	require.Equal(t, http.StatusForbidden, rec.Code,
		"a seated non-facilitator must not be able to start the prologue")
	assert.Contains(t, rec.Body.String(), "only the facilitator")

	after, err := q.GetGameByID(context.Background(), game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhaseLobby, after.Phase, "the refused call must not move the phase")
}

func TestStartPrologue_AllowsFacilitator(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	game, players := newLobbyGame(t, q, 2)

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/start-prologue", StartPrologue(db.NewStore(pool), hub.NewManager()))

	path := "/api/tables/" + strconv.FormatInt(game.ID, 10) + "/start-prologue"
	rec := postAs(r, path, "", sessionCookieFor(t, q, players[0].AccountID))

	require.Equalf(t, http.StatusOK, rec.Code, "start-prologue failed: %s", rec.Body.String())

	after, err := q.GetGameByID(context.Background(), game.ID)
	require.NoError(t, err)
	assert.Equal(t, model.PhasePrologue, after.Phase)
}

// ── SetEndgameMode ───────────────────────────────────────────────────────────

func TestSetEndgameMode_RejectsNonFacilitator(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2) // main_event; Players[0] is the facilitator

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/endgame", SetEndgameMode(db.NewStore(pool), hub.NewManager()))

	path := "/api/tables/" + strconv.FormatInt(tg.Game.ID, 10) + "/endgame"
	body := `{"mode":"` + EndingModeExplosiveFinale + `"}`
	rec := postAs(r, path, body, sessionCookieFor(t, q, tg.Players[1].AccountID))

	require.Equal(t, http.StatusForbidden, rec.Code,
		"only the facilitator may choose how the table's game ends")
	assert.Contains(t, rec.Body.String(), "only the facilitator")

	after, err := q.GetGameByID(context.Background(), tg.Game.ID)
	require.NoError(t, err)
	assert.Nil(t, after.EndingMode, "the refused call must not set an ending mode")
}

func TestSetEndgameMode_AllowsFacilitator(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/endgame", SetEndgameMode(db.NewStore(pool), hub.NewManager()))

	path := "/api/tables/" + strconv.FormatInt(tg.Game.ID, 10) + "/endgame"
	body := `{"mode":"` + EndingModeExplosiveFinale + `"}`
	rec := postAs(r, path, body, sessionCookieFor(t, q, tg.Players[0].AccountID))

	require.Equalf(t, http.StatusOK, rec.Code, "endgame failed: %s", rec.Body.String())

	after, err := q.GetGameByID(context.Background(), tg.Game.ID)
	require.NoError(t, err)
	require.NotNil(t, after.EndingMode)
	assert.Equal(t, EndingModeExplosiveFinale, *after.EndingMode)
}
