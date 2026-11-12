//go:build integration

// handler/laws_rumors_seen_integration_test.go — the per-player "new" markers
// behind the pale dots on Laws and Rumors (migration 058): the endpoint that
// moves them, the rule that an edit counts as new, the author who stays caught
// up, and the part that must never leak: who wrote or edited a rumor whose
// source is hidden.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

type lawsRumorsHarness struct {
	t       *testing.T
	pool    *pgxpool.Pool
	q       *dbgen.Queries
	manager *hub.Manager
	tg      testGame
	router  http.Handler
	tokens  []string
}

func newLawsRumorsHarness(t *testing.T) *lawsRumorsHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 3)
	store := db.NewStore(pool)
	manager := hub.NewManager()

	tokens := make([]string, len(tg.Players))
	for i, p := range tg.Players {
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{Token: tok, AccountID: p.AccountID})
		require.NoError(t, err)
		tokens[i] = tok
	}

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Get("/api/tables/{id}/state", GetGameState(store))
	r.Get("/api/tables/{id}/laws", ListLaws(store))
	r.Get("/api/tables/{id}/rumors", ListRumors(store))
	r.Put("/api/tables/{id}/seen/laws", MarkLawsSeen(store))
	r.Put("/api/tables/{id}/seen/rumors", MarkRumorsSeen(store))
	r.Patch("/api/laws/{lawId}", UpdateLaw(store, manager))
	r.Patch("/api/rumors/{rumorId}", UpdateRumor(store, manager))
	return &lawsRumorsHarness{t: t, pool: pool, q: q, manager: manager, tg: tg, router: r, tokens: tokens}
}

// do sends a request as player idx and returns the status and raw body.
func (h *lawsRumorsHarness) do(method string, idx int, path string, body any) (int, []byte) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.AddCookie(&http.Cookie{Name: appMiddleware.SessionCookie, Value: h.tokens[idx]})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func (h *lawsRumorsHarness) table(suffix string) string {
	return "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + suffix
}

// law makes a law as the given seat without going through any handler, so
// nobody's marker moves: it is exactly what "someone else wrote this" looks like.
func (h *lawsRumorsHarness) law(text string, signatory int) dbgen.Law {
	h.t.Helper()
	sig := h.tg.Players[signatory].ID
	l, err := h.q.CreateLaw(context.Background(), dbgen.CreateLawParams{
		GameID: h.tg.Game.ID, Text: text, SignatoryID: &sig,
	})
	require.NoError(h.t, err)
	return l
}

func (h *lawsRumorsHarness) rumor(text string, source *int64, originPlanID *int64) dbgen.Rumor {
	h.t.Helper()
	r, err := h.q.CreateRumor(context.Background(), dbgen.CreateRumorParams{
		GameID: h.tg.Game.ID, Text: text, SourcePlayerID: source, OriginPlanID: originPlanID,
	})
	require.NoError(h.t, err)
	return r
}

// seen reads a player's marker for a list through the list endpoint.
func (h *lawsRumorsHarness) seen(idx int, list string) int64 {
	h.t.Helper()
	code, body := h.do(http.MethodGet, idx, h.table("/"+list), nil)
	require.Equal(h.t, http.StatusOK, code, string(body))
	var out struct {
		SeenRev int64 `json:"seen_rev"`
	}
	require.NoError(h.t, json.Unmarshal(body, &out))
	return out.SeenRev
}

func (h *lawsRumorsHarness) markLaws(idx int, rev int64) (int, int64) {
	h.t.Helper()
	code, body := h.do(http.MethodPut, idx, h.table("/seen/laws"), map[string]any{"seen_rev": rev})
	var out struct {
		SeenRev int64 `json:"seen_rev"`
	}
	_ = json.Unmarshal(body, &out)
	return code, out.SeenRev
}

func (h *lawsRumorsHarness) markRumors(idx int, rev int64) (int, int64) {
	h.t.Helper()
	code, body := h.do(http.MethodPut, idx, h.table("/seen/rumors"), map[string]any{"seen_rev": rev})
	var out struct {
		SeenRev int64 `json:"seen_rev"`
	}
	_ = json.Unmarshal(body, &out)
	return code, out.SeenRev
}

// editLaw PATCHes a law as the given seat and returns the stored law.
func (h *lawsRumorsHarness) editLaw(idx int, id int64, text string) dbgen.Law {
	h.t.Helper()
	code, body := h.do(http.MethodPatch, idx, "/api/laws/"+strconv.FormatInt(id, 10), map[string]any{"text": text})
	require.Equal(h.t, http.StatusOK, code, string(body))
	var out struct {
		Law dbgen.Law `json:"law"`
	}
	require.NoError(h.t, json.Unmarshal(body, &out))
	return out.Law
}

func (h *lawsRumorsHarness) editRumor(idx int, id int64, text string) dbgen.Rumor {
	h.t.Helper()
	code, body := h.do(http.MethodPatch, idx, "/api/rumors/"+strconv.FormatInt(id, 10), map[string]any{"text": text})
	require.Equal(h.t, http.StatusOK, code, string(body))
	var out struct {
		Rumor dbgen.Rumor `json:"rumor"`
	}
	require.NoError(h.t, json.Unmarshal(body, &out))
	return out.Rumor
}

func TestLawsRumorsSeen_AnUnmarkedSeatSeesEverythingAsNew(t *testing.T) {
	h := newLawsRumorsHarness(t)
	l := h.law("No one may bear iron in the hall.", 0)
	r := h.rumor("The queen walks at night.", nil, nil)

	require.Zero(t, h.seen(1, "laws"))
	require.Zero(t, h.seen(1, "rumors"))
	require.Positive(t, l.Rev, "a law carries a revision to compare with the marker")
	require.Positive(t, r.Rev)
}

func TestMarkSeen_MonotonicClampedAndPerList(t *testing.T) {
	h := newLawsRumorsHarness(t)
	l1 := h.law("one", 0)
	l2 := h.law("two", 0)
	l3 := h.law("three", 0)
	require.Less(t, l1.Rev, l2.Rev)
	require.Less(t, l2.Rev, l3.Rev)

	code, stored := h.markLaws(1, l2.Rev)
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, l2.Rev, stored)

	_, stored = h.markLaws(1, l1.Rev)
	require.Equal(t, l2.Rev, stored, "a marker never moves back")

	_, stored = h.markLaws(1, l3.Rev+1000)
	require.Equal(t, l3.Rev, stored, "a marker clamps to the game's newest law")
	require.Equal(t, l3.Rev, h.seen(1, "laws"))

	code, _ = h.markLaws(1, -1)
	require.Equal(t, http.StatusBadRequest, code)

	// Laws and rumors are separate lists with separate markers.
	require.Zero(t, h.seen(1, "rumors"), "marking laws seen leaves rumors alone")
	r := h.rumor("hush", nil, nil)
	_, stored = h.markRumors(1, r.Rev)
	require.Equal(t, r.Rev, stored)
	require.Equal(t, l3.Rev, h.seen(1, "laws"))

	// A marker is each player's own.
	require.Zero(t, h.seen(2, "laws"))
	require.Zero(t, h.seen(0, "laws"))
}

func TestMarkSeen_AnEmptyListClampsToZero(t *testing.T) {
	h := newLawsRumorsHarness(t)
	_, stored := h.markLaws(1, 99)
	require.Zero(t, stored, "there is nothing to have seen")
}

func TestMarkSeen_RefusesANonMember(t *testing.T) {
	h := newLawsRumorsHarness(t)
	outsider := newTestGame(t, h.q, 2)
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = h.q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: outsider.Players[0].AccountID,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, h.table("/seen/laws"), bytes.NewReader([]byte(`{"seen_rev":1}`)))
	req.AddCookie(&http.Cookie{Name: appMiddleware.SessionCookie, Value: tok})
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	require.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, rec.Code)
}

// An edit draws a new revision, so it is new again to everyone who had seen the
// old wording: the owner's "edits count as new".
func TestEditCountsAsNew(t *testing.T) {
	t.Run("law", func(t *testing.T) {
		h := newLawsRumorsHarness(t)
		l := h.law("Tolls are abolished.", 0)
		_, _ = h.markLaws(1, l.Rev)
		require.Equal(t, l.Rev, h.seen(1, "laws"))

		edited := h.editLaw(0, l.ID, "Tolls are abolished, but not at the bridge.")
		require.Greater(t, edited.Rev, l.Rev, "an edit draws a fresh revision")
		require.Equal(t, l.Rev, h.seen(1, "laws"), "the reader's marker is where they left it")
		// So the entry is above the marker: new again.
		require.Greater(t, edited.Rev, h.seen(1, "laws"))
	})
	t.Run("rumor", func(t *testing.T) {
		h := newLawsRumorsHarness(t)
		src := h.tg.Players[0].ID
		r := h.rumor("The duke is ill.", &src, nil)
		_, _ = h.markRumors(1, r.Rev)

		edited := h.editRumor(0, r.ID, "The duke is dying.")
		require.Greater(t, edited.Rev, r.Rev)
		require.Greater(t, edited.Rev, h.seen(1, "rumors"))
	})
}

// You wrote it, so it is not news to you: chat never counts your own posts, and
// the same holds here. A marker is one number, so it can only follow your write
// when you were already caught up.
func TestAnAuthorStaysCaughtUp(t *testing.T) {
	t.Run("an edit by a caught-up author does not light their own dot", func(t *testing.T) {
		h := newLawsRumorsHarness(t)
		l := h.law("Mine.", 0)
		_, _ = h.markLaws(0, l.Rev)
		_, _ = h.markLaws(1, l.Rev)

		edited := h.editLaw(0, l.ID, "Mine, amended.")
		require.Equal(t, edited.Rev, h.seen(0, "laws"), "the editor is caught up with their own edit")
		require.Less(t, h.seen(1, "laws"), edited.Rev, "everyone else sees the edit as new")
	})

	t.Run("an author who is behind stays behind", func(t *testing.T) {
		h := newLawsRumorsHarness(t)
		theirs := h.law("Someone else's, unread by player 0.", 1)
		mine := h.law("Mine.", 0)
		require.Less(t, theirs.Rev, mine.Rev)

		edited := h.editLaw(0, mine.ID, "Mine, amended.")
		// Moving the marker to the edit would hide `theirs`, which player 0 never saw.
		require.Less(t, h.seen(0, "laws"), theirs.Rev)
		require.Greater(t, edited.Rev, theirs.Rev)
	})

	t.Run("a rumor's editor is caught up too, and only they", func(t *testing.T) {
		h := newLawsRumorsHarness(t)
		src := h.tg.Players[0].ID
		r := h.rumor("Mine.", &src, nil)
		_, _ = h.markRumors(0, r.Rev)
		_, _ = h.markRumors(2, r.Rev)

		edited := h.editRumor(0, r.ID, "Mine, amended.")
		require.Equal(t, edited.Rev, h.seen(0, "rumors"))
		require.Less(t, h.seen(2, "rumors"), edited.Rev)
	})

	t.Run("a law or rumor created through the prologue is not news to its writer", func(t *testing.T) {
		h := newLawsRumorsHarness(t)
		ctx := context.Background()
		require.NoError(t, addLawOrRumor(ctx, h.q, h.manager, h.tg.Game.ID, h.tg.Players[0].ID, "Write a law", "No dogs."))
		require.NoError(t, addLawOrRumor(ctx, h.q, h.manager, h.tg.Game.ID, h.tg.Players[0].ID, "Spread a rumor", "Dogs talk."))

		laws, err := h.q.ListLaws(ctx, h.tg.Game.ID)
		require.NoError(t, err)
		rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
		require.NoError(t, err)
		require.Len(t, laws, 1)
		require.Len(t, rumors, 1)

		require.Equal(t, laws[0].Rev, h.seen(0, "laws"))
		require.Equal(t, rumors[0].Rev, h.seen(0, "rumors"))
		require.Zero(t, h.seen(1, "laws"), "a bystander has seen nothing")
		require.Zero(t, h.seen(1, "rumors"))
	})
}

func TestGameState_CarriesTheViewersOwnMarkers(t *testing.T) {
	h := newLawsRumorsHarness(t)
	l := h.law("a", 0)
	r := h.rumor("b", nil, nil)
	_, _ = h.markLaws(1, l.Rev)
	_, _ = h.markRumors(2, r.Rev)

	read := func(idx int) (laws, rumors int64) {
		code, body := h.do(http.MethodGet, idx, h.table("/state"), nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var out struct {
			Seen struct {
				Laws   int64 `json:"laws"`
				Rumors int64 `json:"rumors"`
			} `json:"laws_rumors_seen"`
		}
		require.NoError(t, json.Unmarshal(body, &out))
		return out.Seen.Laws, out.Seen.Rumors
	}
	laws, rumors := read(1)
	require.Equal(t, l.Rev, laws)
	require.Zero(t, rumors)
	laws, rumors = read(2)
	require.Zero(t, laws)
	require.Equal(t, r.Rev, rumors)
	laws, rumors = read(0)
	require.Zero(t, laws)
	require.Zero(t, rumors)
}

// ── Hidden sources ───────────────────────────────────────────────────────────

// TestHiddenSourceRumor_NothingNamesTheWriterOrEditor is the trap the migration
// is built around. A rumor whose source is hidden (source_player_id NULL) is
// still written, and later edited, by someone: its origin plan's preparer. The
// table sees the edit (the text and the revision change) but must learn nothing
// that says who made it.
func TestHiddenSourceRumor_NothingNamesTheWriterOrEditor(t *testing.T) {
	h := newLawsRumorsHarness(t)
	ctx := context.Background()
	author := h.tg.Players[0]
	plan := createPlanOnRow(t, h.q, &h.tg.Game, &author, model.PlanSpreadRumors, model.CategoryEsteem, 1)
	rumor := h.rumor("Someone has been selling the crown jewels.", nil, &plan.ID)
	require.Nil(t, rumor.SourcePlayerID)

	// The author has caught up with the rumor, as they would after writing it.
	_, _ = h.markRumors(0, rumor.Rev)

	// What another seat sees before the edit.
	_, beforeList := h.do(http.MethodGet, 1, h.table("/rumors"), nil)
	_, beforeState := h.do(http.MethodGet, 1, h.table("/state"), nil)

	edited := h.editRumor(0, rumor.ID, "Someone has been selling the crown jewels, piece by piece.")
	require.Nil(t, edited.SourcePlayerID, "editing does not unhide the source")

	_, afterList := h.do(http.MethodGet, 1, h.table("/rumors"), nil)
	_, afterState := h.do(http.MethodGet, 1, h.table("/state"), nil)

	t.Run("an edit changes exactly the text and the revision on the wire", func(t *testing.T) {
		before := firstRumor(t, beforeList)
		after := firstRumor(t, afterList)
		var changed []string
		for k := range after {
			if string(before[k]) != string(after[k]) {
				changed = append(changed, k)
			}
		}
		require.ElementsMatch(t, []string{"text", "rev"}, changed,
			"nothing else about a rumor may move when its hidden author edits it")
		require.JSONEq(t, "null", string(after["source_player_id"]))
	})

	t.Run("the rumor row has no column that could name an editor", func(t *testing.T) {
		// A tripwire for the next migration. Rumors rows are served whole to every
		// player (ListRumors, GET /state, the rumor.* events), so any new column
		// reaches the table. If one is genuinely needed, it must not be a
		// player id, and this list is where to say so.
		require.ElementsMatch(t,
			[]string{"id", "game_id", "text", "target_asset_id", "origin_plan_id",
				"source_player_id", "is_active", "created_at", "display_order", "rev"},
			columnsOf(t, h, "rumors"))
		require.ElementsMatch(t,
			[]string{"id", "game_id", "text", "addendum", "origin_plan_id", "signatory_id",
				"created_at", "is_active", "display_order", "rev"},
			columnsOf(t, h, "laws"))
	})

	t.Run("markers are not on the players rows every seat receives", func(t *testing.T) {
		cols := columnsOf(t, h, "players")
		require.NotContains(t, cols, "laws_seen_rev")
		require.NotContains(t, cols, "rumors_seen_rev")

		var state struct {
			Players []map[string]json.RawMessage `json:"players"`
		}
		require.NoError(t, json.Unmarshal(afterState, &state))
		require.NotEmpty(t, state.Players)
		for _, p := range state.Players {
			require.NotContains(t, p, "laws_seen_rev")
			require.NotContains(t, p, "rumors_seen_rev")
		}
	})

	t.Run("another seat's state is its own marker, not the editor's", func(t *testing.T) {
		var before, after struct {
			Seen map[string]int64 `json:"laws_rumors_seen"`
		}
		require.NoError(t, json.Unmarshal(beforeState, &before))
		require.NoError(t, json.Unmarshal(afterState, &after))
		require.Equal(t, before.Seen, after.Seen,
			"the editor catching up moves their marker and no one else's")
		// And the editor's own really did move, so the check above is not vacuous.
		require.Equal(t, edited.Rev, h.seen(0, "rumors"))
		require.Zero(t, after.Seen["rumors"])
	})

	t.Run("the markers cascade away with the game", func(t *testing.T) {
		var n int
		require.NoError(t, h.pool.QueryRow(ctx,
			`SELECT count(*) FROM player_laws_rumors_seen WHERE player_id = $1`, author.ID).Scan(&n))
		require.Equal(t, 1, n)
		_, err := h.q.DeleteGame(ctx, h.tg.Game.ID)
		require.NoError(t, err)
		require.NoError(t, h.pool.QueryRow(ctx,
			`SELECT count(*) FROM player_laws_rumors_seen`).Scan(&n))
		require.Zero(t, n, "deleting a game must not strand marker rows")
	})
}

func firstRumor(t *testing.T, listBody []byte) map[string]json.RawMessage {
	t.Helper()
	var out struct {
		Rumors []map[string]json.RawMessage `json:"rumors"`
	}
	require.NoError(t, json.Unmarshal(listBody, &out))
	require.Len(t, out.Rumors, 1)
	return out.Rumors[0]
}

// columnsOf lists a table's columns, for the tripwires above.
func columnsOf(t *testing.T, h *lawsRumorsHarness, table string) []string {
	t.Helper()
	rows, err := h.pool.Query(context.Background(),
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1`, table)
	require.NoError(t, err)
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		cols = append(cols, c)
	}
	require.NoError(t, rows.Err())
	return cols
}
