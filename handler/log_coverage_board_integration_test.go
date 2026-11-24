//go:build integration

// handler/log_coverage_board_integration_test.go — the runtime half of the
// "every board change posts to the action log" guard.
//
// log_coverage_test.go is the static half: it sees that a function calls a
// board-changing helper and also reaches a log emitter, but it can't tell
// whether the post sits on a branch that runs. This one watches the board
// itself. Every mutating request an integration test sends through serveGuarded
// is bracketed by two snapshots of the game's board; if the board moved and no
// new system post (action-log row) was written for that game, the test fails
// and names what changed.
//
// # What a snapshot holds
//
//	assets      owner, is_destroyed, is_main_character, name
//	marginalia  position, text, title, is_torn (every asset's)
//	secrets     existence and is_revealed — never the text (hidden information)
//	rankings    which player holds which rank in each category (track positions)
//	laws        text, addendum, is_active
//	rumors      text, target asset, is_active
//	players     shake_up_tokens
//	plan_tokens which plan a player's token for a plan type sits on
//	banked_dice who holds a banked die, its source, whether it is spent
//	shake_up_spends  each Shake-Up spend (player, category, option, target)
//
// # What it deliberately leaves out
//
//   - player_cards: a player's dealt hand is private, and dealing it is setup,
//     not a story event.
//   - is_leveraged: a token that flips on every roll and refresh; the roll and
//     refresh routes narrate it themselves, and it isn't story.
//   - secret_visibility and secret text: who may read a secret is access, not
//     board; the text is gated, so the snapshot must never carry it.
//   - plans, dice rolls, wars, duels, scenes and games rows (current_row, phase):
//     they are the machinery, and each already posts through its own plan or
//     phase code. Board changes that machinery causes (a breakage, a transfer, a
//     rank swap) are caught here through the tables above.
//   - display_order and the rev counters on laws and rumors: reordering and
//     bookkeeping, not a change the table would be told about.
//
// # What counts as a log post
//
// A new scene_posts row with a system_code. A player's chat message is the
// player talking, not the game recording what happened, so it doesn't satisfy
// the guard. Posts in the prologue (current_row 0) and Shake-Up (14) are not
// dropped — logRow returns nil there, so the post lands unanchored — which means
// those phases need no special rule: a board change there must still post.
//
// # When a silent change is legitimate
//
// Add the route to silentBoardRoutes with the reason. The friction is intended,
// like logCoverageAllowed in log_coverage_test.go: the first person to hit the
// guard with a legitimate case has to say why it is one. A real gap that the
// owner hasn't ruled on yet goes in with a TODO(log-gap) reason, so the list of
// open gaps is greppable.

package handler

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// silentBoardRoute is one allow-list entry. kinds narrows it to the listed
// snapshot kinds (asset, marginalia, secret, ranking, law, rumor); empty means
// the route may change anything silently.
type silentBoardRoute struct {
	reason string
	kinds  []string
}

// silentBoardRoutes lists the requests that may change the board without a log
// post, keyed "METHOD /path" with numeric segments written {id} (see
// boardRouteKey). Every entry says why.
var silentBoardRoutes = map[string]silentBoardRoute{}

// boardRouteKey turns "POST /api/plans/12/fair-trade?x=1" into
// "POST /api/plans/{id}/fair-trade".
func boardRouteKey(method, path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if s != "" && strings.Trim(s, "0123456789") == "" {
			segs[i] = "{id}"
		}
	}
	return method + " " + strings.Join(segs, "/")
}

// boardRow is one entry in a snapshot: its identity and a fingerprint of the
// fields the guard cares about.
type boardRow struct {
	kind  string
	id    int64
	state string
}

// gameBoard is a game's board plus the number of system posts in its log.
type gameBoard struct {
	rows  map[string]boardRow // "kind#id"
	posts int64
}

type boardSnapshot map[int64]*gameBoard

const boardSnapshotSQL = `
SELECT id AS game_id, 'game' AS kind, id, '' AS state FROM games
UNION ALL
SELECT game_id, 'asset', id, format('owner=%s destroyed=%s main_character=%s name=%s', owner_id, is_destroyed, is_main_character, name) FROM assets
UNION ALL
SELECT a.game_id, 'marginalia', m.id,
       format('asset=%s position=%s torn=%s title=%s text=%s', m.asset_id, m.position, m.is_torn, coalesce(m.title, ''), m.text)
  FROM marginalia m JOIN assets a ON a.id = m.asset_id
UNION ALL
SELECT a.game_id, 'secret', s.id, format('asset=%s revealed=%s', s.asset_id, s.is_revealed)
  FROM secrets s JOIN assets a ON a.id = s.asset_id
UNION ALL
SELECT game_id, 'ranking', id, format('player=%s category=%s rank=%s', coalesce(player_id::text, 'none'), category, rank) FROM rankings
UNION ALL
SELECT game_id, 'law', id, format('text=%s addendum=%s active=%s', text, coalesce(addendum, ''), is_active) FROM laws
UNION ALL
SELECT game_id, 'rumor', id, format('text=%s target_asset=%s active=%s', text, coalesce(target_asset_id::text, 'none'), is_active) FROM rumors
UNION ALL
SELECT game_id, 'shake_up_tokens', id, format('player=%s tokens=%s', id, shake_up_tokens) FROM players
UNION ALL
SELECT game_id, 'plan_token', id, format('plan_type=%s player=%s plan=%s', plan_type, player_id, plan_id) FROM plan_tokens
UNION ALL
SELECT game_id, 'banked_die', id, format('player=%s source=%s used=%s', player_id, source, used_at IS NOT NULL) FROM banked_dice
UNION ALL
SELECT game_id, 'shake_up_spend', id,
       format('player=%s category=%s option=%s asset=%s target_player=%s', player_id, category, option_key,
              coalesce(target_asset_id::text, 'none'), coalesce(target_player_id::text, 'none'))
  FROM shake_up_spends
UNION ALL
SELECT game_id, 'post', 0, count(*)::text FROM scene_posts WHERE system_code IS NOT NULL GROUP BY game_id
`

func takeBoardSnapshot(ctx context.Context) (boardSnapshot, error) {
	rows, err := harnessPool.Query(ctx, boardSnapshotSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snap := boardSnapshot{}
	for rows.Next() {
		var (
			gameID int64
			r      boardRow
		)
		if err := rows.Scan(&gameID, &r.kind, &r.id, &r.state); err != nil {
			return nil, err
		}
		g := snap[gameID]
		if g == nil {
			g = &gameBoard{rows: map[string]boardRow{}}
			snap[gameID] = g
		}
		if r.kind == "post" {
			_, _ = fmt.Sscan(r.state, &g.posts)
			continue
		}
		g.rows[fmt.Sprintf("%s#%d", r.kind, r.id)] = r
	}
	return snap, rows.Err()
}

// boardChange is one difference between two snapshots of a game.
type boardChange struct {
	kind string
	text string
}

// diffBoards lists what moved between before and after in the games both
// snapshots contain (a game created or deleted by the request is skipped).
func diffBoards(before, after boardSnapshot) map[int64][]boardChange {
	out := map[int64][]boardChange{}
	for gameID, b := range before {
		a, ok := after[gameID]
		if !ok {
			continue
		}
		var changes []boardChange
		for key, br := range b.rows {
			ar, still := a.rows[key]
			switch {
			case !still:
				changes = append(changes, boardChange{br.kind, fmt.Sprintf("%s removed (was %q)", key, br.state)})
			case ar.state != br.state:
				changes = append(changes, boardChange{br.kind, fmt.Sprintf("%s changed: %q -> %q", key, br.state, ar.state)})
			}
		}
		for key, ar := range a.rows {
			if _, was := b.rows[key]; !was {
				changes = append(changes, boardChange{ar.kind, fmt.Sprintf("%s added (%q)", key, ar.state)})
			}
		}
		if len(changes) > 0 {
			sort.Slice(changes, func(i, j int) bool { return changes[i].text < changes[j].text })
			out[gameID] = changes
		}
	}
	return out
}

// guardedRequests counts the requests the guard has bracketed (diagnostics).
var guardedRequests atomic.Int64

// boardT is the slice of *testing.T the guard uses, so the guard's own test can
// watch it fail without failing itself.
type boardT interface {
	Helper()
	Errorf(format string, args ...any)
	FailNow()
}

// serveGuarded is router.ServeHTTP plus the board guard. Every harness that
// sends a request as a player routes through it, so a mutating request that
// changes the board and posts nothing fails the test that sent it.
func serveGuarded(t boardT, router http.Handler, rec *httptest.ResponseRecorder, req *http.Request) {
	t.Helper()
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		router.ServeHTTP(rec, req)
		return
	}
	// A harness reaches here only after openTestDB, which sets harnessPool. If it
	// is nil the guard would silently do nothing, so fail instead.
	if harnessPool == nil {
		t.Errorf("board guard: harnessPool is nil, so this request would go unchecked; open the database with openTestDB first")
		t.FailNow()
	}
	guardedRequests.Add(1)
	ctx := context.Background()
	before, err := takeBoardSnapshot(ctx)
	require.NoError(t, err, "board guard: snapshot before")
	router.ServeHTTP(rec, req)
	after, err := takeBoardSnapshot(ctx)
	require.NoError(t, err, "board guard: snapshot after")

	route := boardRouteKey(req.Method, req.URL.Path)
	for gameID, changes := range diffBoards(before, after) {
		if after[gameID].posts > before[gameID].posts {
			continue
		}
		var unexplained []string
		allow, listed := silentBoardRoutes[route]
		for _, c := range changes {
			if listed && (len(allow.kinds) == 0 || slices.Contains(allow.kinds, c.kind)) {
				continue
			}
			unexplained = append(unexplained, c.text)
		}
		if len(unexplained) == 0 {
			continue
		}
		t.Errorf("%s (HTTP %d) changed game %d's board and wrote no action-log post:\n  %s\n"+
			"Post it with an emitter (see log_coverage_test.go), or add the route to silentBoardRoutes with the reason.",
			route, rec.Code, gameID, strings.Join(unexplained, "\n  "))
	}
}

// recordingT collects what the guard reports instead of failing the test that
// is checking it.
type recordingT struct{ errs []string }

func (r *recordingT) Helper() {}
func (r *recordingT) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}
func (r *recordingT) FailNow() { panic("recordingT.FailNow") }

// TestBoardGuardCatchesASilentChange proves the guard bites: a route that
// destroys an asset and writes nothing fails; the same route with a post passes;
// and an allow-listed route is let through, but only for its listed kinds.
func TestBoardGuardCatchesASilentChange(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	peer := createBoardPeer(t, q, tg)

	serve := func(path string, h http.HandlerFunc) *recordingT {
		rt := &recordingT{}
		serveGuarded(rt, h, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, nil))
		return rt
	}

	silent := serve("/silent/1", func(http.ResponseWriter, *http.Request) {
		require.NoError(t, q.DestroyAsset(t.Context(), peer))
	})
	require.Len(t, silent.errs, 1, "a destroyed asset with no post must fail the guard")
	require.Contains(t, silent.errs[0], "POST /silent/{id}")
	require.Contains(t, silent.errs[0], "destroyed=t", "the change must be named")

	peer = createBoardPeer(t, q, tg)
	posted := serve("/posted/1", func(http.ResponseWriter, *http.Request) {
		require.NoError(t, q.DestroyAsset(t.Context(), peer))
		_, err := q.CreateSystemPost(t.Context(), dbgen.CreateSystemPostParams{
			GameID: tg.Game.ID, Body: "destroyed", Severity: 1, SystemCode: new("asset.destroyed"),
		})
		require.NoError(t, err)
	})
	require.Empty(t, posted.errs, "the same change with a log post must pass")

	peer = createBoardPeer(t, q, tg)
	silentBoardRoutes["POST /allowed/{id}"] = silentBoardRoute{reason: "test", kinds: []string{"marginalia"}}
	defer delete(silentBoardRoutes, "POST /allowed/{id}")
	wrongKind := serve("/allowed/1", func(http.ResponseWriter, *http.Request) {
		require.NoError(t, q.DestroyAsset(t.Context(), peer))
	})
	require.Len(t, wrongKind.errs, 1, "an allow-list entry narrowed to marginalia must not excuse an asset change")
	silentBoardRoutes["POST /allowed/{id}"] = silentBoardRoute{reason: "test"}
	peer = createBoardPeer(t, q, tg)
	allowed := serve("/allowed/1", func(http.ResponseWriter, *http.Request) {
		require.NoError(t, q.DestroyAsset(t.Context(), peer))
	})
	require.Empty(t, allowed.errs, "an allow-listed route may change the board silently")
}

func createBoardPeer(t *testing.T, q *dbgen.Queries, tg testGame) int64 {
	t.Helper()
	a, err := q.CreateAsset(t.Context(), dbgen.CreateAssetParams{
		GameID: tg.Game.ID, OwnerID: tg.Players[0].ID, CreatorID: tg.Players[0].ID,
		AssetType: model.AssetPeer, Name: "Board guard peer",
	})
	require.NoError(t, err)
	return a.ID
}

// TestMain fails a full integration run in which the guard never ran, so a
// change that stops the harnesses reaching serveGuarded can't leave every test
// green while the guard does nothing. A filtered run (-run) or one with no
// database is exempt: it may legitimately touch no guarded route.
func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 && harnessPool != nil && guardedRequests.Load() == 0 {
		if f := flag.Lookup("test.run"); f == nil || f.Value.String() == "" {
			fmt.Fprintln(os.Stderr, "board guard: a full integration run guarded no requests; "+
				"serveGuarded is no longer reached from the harnesses")
			code = 1
		}
	}
	os.Exit(code)
}
