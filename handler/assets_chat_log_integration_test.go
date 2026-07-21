//go:build integration

// handler/assets_chat_log_integration_test.go — end-to-end coverage for the
// asset & marginalia action-log posts (asset.created, asset.renamed,
// marginalia.added/edited/torn, asset.taken, asset.main_character).
//
// These drive the real chi routes so the EmitX helpers fire through the same
// path production uses, then read the unified chat feed back to assert the
// post body and severity. The goal the logs serve — a feed from which game
// state can be reconstructed — is checked here at the message level.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// assetHarness wires the real asset/marginalia routes with one seeded session
// per player so post/put/del authenticate as players[i].
type assetHarness struct {
	t       *testing.T
	pool    *pgxpool.Pool
	q       *dbgen.Queries
	manager *hub.Manager
	tg      testGame
	router  http.Handler
	tokens  []string
}

func newAssetHarness(t *testing.T, n int) *assetHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, n)
	store := db.NewStore(pool)
	manager := hub.NewManager()

	tokens := make([]string, n)
	for i, p := range tg.Players {
		tok, err := db.NewCookieToken()
		require.NoError(t, err)
		_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
			Token: tok, AccountID: p.AccountID,
		})
		require.NoError(t, err)
		tokens[i] = tok
	}

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/assets", CreateAsset(store, manager))
	r.Post("/api/tables/{id}/replace-main-character", ReplaceMainCharacterWithNewPeer(store, manager))
	r.Route("/api/assets/{assetId}", func(rr chi.Router) {
		rr.Put("/", UpdateAsset(store, manager))
		rr.Post("/marginalia", AddMarginalia(store, manager))
		rr.Put("/marginalia/{pos}", UpdateMarginalia(store, manager))
		rr.Delete("/marginalia/{pos}", TearMarginalia(store, manager))
		rr.Post("/take", TakeAsset(store, manager))
		rr.Post("/secrets", WriteSecret(store, manager))
	})

	return &assetHarness{
		t: t, pool: pool, q: q, manager: manager,
		tg: tg, router: r, tokens: tokens,
	}
}

func (h *assetHarness) do(method string, playerIdx int, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[playerIdx]})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func (h *assetHarness) tablePath() string {
	return "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/assets"
}

func assetPath(assetID int64, suffix string) string {
	return "/api/assets/" + strconv.FormatInt(assetID, 10) + suffix
}

// postBySystemCode returns the single chat post with the given system_code,
// failing if there isn't exactly one.
func (h *assetHarness) postBySystemCode(code string) dbgen.ScenePost {
	h.t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(h.t, err)
	var matches []dbgen.ScenePost
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == code {
			matches = append(matches, p)
		}
	}
	require.Lenf(h.t, matches, 1, "expected exactly one %q post, got %d", code, len(matches))
	return matches[0]
}

func assetIDFromBody(t *testing.T, body map[string]any) int64 {
	t.Helper()
	asset, ok := body["asset"].(map[string]any)
	require.True(t, ok, "response had no asset: %v", body)
	return int64(asset["id"].(float64))
}

// TestCreateAsset_RequiresExactlyOneMarginalia proves the generic asset-
// creation route enforces the same name-and-one-marginalia rule as every
// other asset-creation route (Phase 8 tightening).
func TestCreateAsset_RequiresExactlyOneMarginalia(t *testing.T) {
	h := newAssetHarness(t, 2)

	code, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Nameless",
	})
	require.Equalf(t, http.StatusBadRequest, code, "missing marginalia rejected: %v", body)

	code, body = h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Nameless", "marginalia": []string{},
	})
	require.Equalf(t, http.StatusBadRequest, code, "empty marginalia rejected: %v", body)

	code, body = h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Nameless", "marginalia": []string{"", "   "},
	})
	require.Equalf(t, http.StatusBadRequest, code, "all-blank marginalia rejected: %v", body)

	code, body = h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Twice-Noted", "marginalia": []string{"first", "second"},
	})
	require.Equalf(t, http.StatusBadRequest, code, "two marginalia rejected: %v", body)

	code, body = h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Well-Formed", "marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusCreated, code, "exactly one marginalia accepted: %v", body)
	id := assetIDFromBody(t, body)

	margs, err := h.q.ListMarginaliaByAsset(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, margs, 1, "asset is created with exactly one marginalia")
	require.Equal(t, int16(1), margs[0].Position)
	require.Equal(t, "a trait", margs[0].Text)
}

// TestChatLog_AssetCreatedWithMarginalia: a single Default post folds the
// asset and its initial marginalia into one event.
func TestChatLog_AssetCreatedWithMarginalia(t *testing.T) {
	h := newAssetHarness(t, 2)
	owner := h.tg.Players[0].DisplayName

	code, _ := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer",
		"name":       "Sir Reginald",
		"marginalia": []string{"loyal to a fault"},
	})
	require.Equal(t, http.StatusCreated, code)

	p := h.postBySystemCode("asset.created")
	require.Equal(t, model.SeverityDefault, p.Severity)
	require.Contains(t, p.Body, owner)
	require.Contains(t, p.Body, "Sir Reginald")
	require.Contains(t, p.Body, "loyal to a fault")
}

// TestChatLog_AssetRenamed: renaming emits a Trace post naming old and new.
func TestChatLog_AssetRenamed(t *testing.T) {
	h := newAssetHarness(t, 2)

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "holding", "name": "Old Keep", "marginalia": []string{"weathered stone"},
	})
	id := assetIDFromBody(t, body)

	code, _ := h.do("PUT", 0, assetPath(id, "/"), map[string]any{"name": "New Keep"})
	require.Equal(t, http.StatusOK, code)

	p := h.postBySystemCode("asset.renamed")
	require.Equal(t, model.SeverityTrace, p.Severity)
	require.Contains(t, p.Body, "Old Keep")
	require.Contains(t, p.Body, "New Keep")
}

// TestChatLog_MarginaliaAddedAndEdited: add is Default with text+asset; edit
// is Trace with the new text+asset.
func TestChatLog_MarginaliaAddedAndEdited(t *testing.T) {
	h := newAssetHarness(t, 2)

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Lady Vex", "marginalia": []string{"sharp-tongued"},
	})
	id := assetIDFromBody(t, body)

	// Creation already occupies position 1, so the added note lands at 2.
	code, _ := h.do("POST", 0, assetPath(id, "/marginalia"), map[string]any{"text": "owes a debt"})
	require.Equal(t, http.StatusCreated, code)
	added := h.postBySystemCode("marginalia.added")
	require.Equal(t, model.SeverityDefault, added.Severity)
	require.Contains(t, added.Body, "owes a debt")
	require.Contains(t, added.Body, "Lady Vex")

	code, _ = h.do("PUT", 0, assetPath(id, "/marginalia/2"), map[string]any{"text": "debt forgiven"})
	require.Equal(t, http.StatusOK, code)
	edited := h.postBySystemCode("marginalia.edited")
	require.Equal(t, model.SeverityTrace, edited.Severity)
	require.Contains(t, edited.Body, "debt forgiven")
	require.Contains(t, edited.Body, "Lady Vex")
}

// TestChatLog_MarginaliaTornByRival: a Default post names tearer, owner, text
// and asset.
func TestChatLog_MarginaliaTornByRival(t *testing.T) {
	h := newAssetHarness(t, 2)
	owner := h.tg.Players[0].DisplayName
	rival := h.tg.Players[1].DisplayName

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "The Spy", "marginalia": []string{"knows a secret"},
	})
	id := assetIDFromBody(t, body)

	code, _ := h.do("DELETE", 1, assetPath(id, "/marginalia/1"), nil)
	require.Equal(t, http.StatusOK, code)

	p := h.postBySystemCode("marginalia.torn")
	require.Equal(t, model.SeverityDefault, p.Severity)
	require.Contains(t, p.Body, rival)
	require.Contains(t, p.Body, owner)
	require.Contains(t, p.Body, "knows a secret")
	require.Contains(t, p.Body, "The Spy")
}

// TestChatLog_AssetTaken: ownership transfer emits a Default post naming taker,
// asset and previous owner.
func TestChatLog_AssetTaken(t *testing.T) {
	h := newAssetHarness(t, 2)
	owner := h.tg.Players[0].DisplayName
	taker := h.tg.Players[1].DisplayName

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "artifact", "name": "The Crown", "marginalia": []string{"gleaming"},
	})
	id := assetIDFromBody(t, body)

	code, _ := h.do("POST", 1, assetPath(id, "/take"), nil)
	require.Equal(t, http.StatusOK, code)

	p := h.postBySystemCode("asset.taken")
	require.Equal(t, model.SeverityDefault, p.Severity)
	require.Contains(t, p.Body, taker)
	require.Contains(t, p.Body, owner)
	require.Contains(t, p.Body, "The Crown")
}

// TestChatLog_MainCharacterPromoted: naming a main character emits a Default
// post.
func TestChatLog_MainCharacterPromoted(t *testing.T) {
	h := newAssetHarness(t, 2)
	owner := h.tg.Players[0].DisplayName

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Protagonist", "marginalia": []string{"determined"},
	})
	id := assetIDFromBody(t, body)

	code, _ := h.do("PUT", 0, assetPath(id, "/"), map[string]any{"is_main_character": true})
	require.Equal(t, http.StatusOK, code)

	p := h.postBySystemCode("asset.main_character")
	require.Equal(t, model.SeverityDefault, p.Severity)
	require.Contains(t, p.Body, owner)
	require.Contains(t, p.Body, "Protagonist")
	require.True(t, strings.Contains(p.Body, "main character"))
}

// TestReplaceMainCharacter_ConscriptLeveragesAllAssets covers the "no peers
// left" escape hatch: a player whose main character was taken, and who has no
// peer to promote, conscripts a brand new one — at the custom-rule cost of all
// their assets becoming leveraged.
func TestReplaceMainCharacter_ConscriptLeveragesAllAssets(t *testing.T) {
	h := newAssetHarness(t, 2)
	ctx := context.Background()
	loser := h.tg.Players[0]

	// Give the loser a non-peer asset, so we can prove the cost hits every
	// asset they own — not just the freshly conscripted peer.
	holding, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: loser.ID, CreatorID: loser.ID,
		AssetType: model.AssetHolding, Name: "Keep", IsMainCharacter: false,
	})
	require.NoError(t, err)

	// Player 1 takes the loser's only peer (their seeded main character),
	// leaving the loser with no peer to promote.
	mc, err := h.q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: h.tg.Game.ID, OwnerID: loser.ID,
	})
	require.NoError(t, err)
	code, _ := h.do("POST", 1, assetPath(mc.ID, "/take"), nil)
	require.Equal(t, http.StatusOK, code)

	// Conscript a new main character.
	code, body := h.do("POST", 0,
		"/api/tables/"+strconv.FormatInt(h.tg.Game.ID, 10)+"/replace-main-character",
		map[string]any{"name": "Heir", "marginalia": []string{"bold", "untested"}})
	require.Equalf(t, http.StatusCreated, code, "conscript: %v", body)

	// They have a main character again, and it is leveraged.
	newMC, err := h.q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: h.tg.Game.ID, OwnerID: loser.ID,
	})
	require.NoError(t, err)
	require.True(t, newMC.IsLeveraged, "the conscripted main character is leveraged")

	// The cost hit every asset they own.
	keep, err := h.q.GetAssetByID(ctx, holding.ID)
	require.NoError(t, err)
	require.True(t, keep.IsLeveraged, "all of the conscriptor's assets become leveraged")

	// And it logged the conscription.
	p := h.postBySystemCode("asset.main_character")
	require.Contains(t, p.Body, "conscripted")
}

// TestReplaceMainCharacter_MinimumMarginalia proves conscription requires a
// name and one marginalia (relaxed from two, per the asset-creation rule) and
// still rejects an empty marginalia list.
func TestReplaceMainCharacter_MinimumMarginalia(t *testing.T) {
	h := newAssetHarness(t, 2)
	ctx := context.Background()
	loser := h.tg.Players[0]

	// Take the loser's only peer (their seeded main character), leaving them
	// with no peer to promote — the conscript route applies.
	mc, err := h.q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: h.tg.Game.ID, OwnerID: loser.ID,
	})
	require.NoError(t, err)
	code, _ := h.do("POST", 1, assetPath(mc.ID, "/take"), nil)
	require.Equal(t, http.StatusOK, code)

	path := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/replace-main-character"

	// Zero marginalia is still rejected.
	code, body := h.do("POST", 0, path, map[string]any{"name": "Heir", "marginalia": []string{}})
	require.Equalf(t, http.StatusBadRequest, code, "zero marginalia rejected: %v", body)

	// Exactly one marginalia now succeeds (previously required two).
	code, body = h.do("POST", 0, path, map[string]any{"name": "Heir", "marginalia": []string{"bold"}})
	require.Equalf(t, http.StatusCreated, code, "one marginalia accepted: %v", body)
}

// TestReplaceMainCharacter_RejectedWhenPeerAvailable proves the conscript route
// is the no-peers-left escape hatch only: a player who still owns a peer must
// promote it (free) instead of conscripting a new one (which costs leverage).
func TestReplaceMainCharacter_RejectedWhenPeerAvailable(t *testing.T) {
	h := newAssetHarness(t, 2)
	ctx := context.Background()
	loser := h.tg.Players[0]

	// Give the loser a spare peer, then take their main character. They now
	// have no main character but DO have a peer to promote.
	_, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: h.tg.Game.ID, OwnerID: loser.ID, CreatorID: loser.ID,
		AssetType: model.AssetPeer, Name: "Spare", IsMainCharacter: false,
	})
	require.NoError(t, err)
	mc, err := h.q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: h.tg.Game.ID, OwnerID: loser.ID,
	})
	require.NoError(t, err)
	code, _ := h.do("POST", 1, assetPath(mc.ID, "/take"), nil)
	require.Equal(t, http.StatusOK, code)

	code, body := h.do("POST", 0,
		"/api/tables/"+strconv.FormatInt(h.tg.Game.ID, 10)+"/replace-main-character",
		map[string]any{"name": "Heir", "marginalia": []string{"bold", "untested"}})
	require.Equalf(t, http.StatusConflict, code, "should reject conscript while a peer exists: %v", body)
}

// ── Row anchors outside the main event ────────────────────────────────────────
//
// scene_posts.row_number is FK-checked against public_record_rows, which only
// holds rows 1–13 and only once the main event has begun. A game's current_row
// sits outside that range during the prologue (0) and during the Shake-Up (14,
// where advanceRowInner leaves it). Emitters used to be handed current_row
// blind, so in both phases the insert violated the FK and EmitSystemPost —
// best-effort by design — dropped the post with no trace. The asset and
// marginalia routes carry no phase guard, so this silently lost every
// bookkeeping line outside the main event.
//
// logRow is the fix: it yields nil outside rows 1–13, and an unanchored post
// still reaches the feed. These tests pin the two phases that regressed.
// Note SeedShakeUp leaves current_row at 13, which is why the seeded fixtures
// never reproduced the Shake-Up half — these set the row explicitly.

// setPhaseAndRow forces the game off a valid public-record row, standing in for
// the prologue (row 0) and the Shake-Up (row 14).
func (h *assetHarness) setPhaseAndRow(phase model.GamePhase, row int16) {
	h.t.Helper()
	ctx := context.Background()
	require.NoError(h.t, h.q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: h.tg.Game.ID, Phase: phase,
	}))
	require.NoError(h.t, h.q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: h.tg.Game.ID, CurrentRow: row,
	}))
}

// assertUnanchoredLog runs fn in the given phase/row and asserts the named post
// landed in the feed with no row anchor.
func (h *assetHarness) assertUnanchoredLog(phase model.GamePhase, row int16, code string, fn func()) {
	h.t.Helper()
	h.setPhaseAndRow(phase, row)
	fn()
	p := h.postBySystemCode(code)
	require.Nilf(h.t, p.RowNumber,
		"%s post in %s should carry no row anchor (row %d does not exist)", code, phase, row)
}

// TestChatLog_MarginaliaAddedInPrologue: current_row is 0 before the main event
// seeds rows 1–13, so the add must log unanchored rather than vanish.
func TestChatLog_MarginaliaAddedInPrologue(t *testing.T) {
	h := newAssetHarness(t, 2)

	// Create the asset while the row is still valid, so only the add under
	// test runs against row 0.
	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Lady Vex", "marginalia": []string{"sharp-tongued"},
	})
	id := assetIDFromBody(t, body)

	h.assertUnanchoredLog(model.PhasePrologue, 0, "marginalia.added", func() {
		code, _ := h.do("POST", 0, assetPath(id, "/marginalia"), map[string]any{"text": "owes a debt"})
		require.Equal(t, http.StatusCreated, code)
	})

	added := h.postBySystemCode("marginalia.added")
	require.Equal(t, model.SeverityDefault, added.Severity)
	require.Contains(t, added.Body, "owes a debt")
	require.Contains(t, added.Body, "Lady Vex")
}

// TestChatLog_MarginaliaTornInShakeUp: the Shake-Up leaves current_row at 14,
// one past the last public-record row. Tearing must still log.
func TestChatLog_MarginaliaTornInShakeUp(t *testing.T) {
	h := newAssetHarness(t, 2)

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "The Spy", "marginalia": []string{"knows a secret"},
	})
	id := assetIDFromBody(t, body)

	h.assertUnanchoredLog(model.PhaseShakeUp, publicRecordRowCount+1, "marginalia.torn", func() {
		code, _ := h.do("DELETE", 1, assetPath(id, "/marginalia/1"), nil)
		require.Equal(t, http.StatusOK, code)
	})

	torn := h.postBySystemCode("marginalia.torn")
	require.Contains(t, torn.Body, "knows a secret")
	require.Contains(t, torn.Body, "The Spy")
}

// TestChatLog_AssetRenamedInPrologue: renames are Trace, so they are hidden by
// the chat's default "hide bookkeeping" filter — but they must still be written,
// or unticking the filter reveals nothing.
func TestChatLog_AssetRenamedInPrologue(t *testing.T) {
	h := newAssetHarness(t, 2)

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Old Name", "marginalia": []string{"unremarkable"},
	})
	id := assetIDFromBody(t, body)

	h.assertUnanchoredLog(model.PhasePrologue, 0, "asset.renamed", func() {
		code, _ := h.do("PUT", 0, assetPath(id, "/"), map[string]any{"name": "New Name"})
		require.Equal(t, http.StatusOK, code)
	})

	renamed := h.postBySystemCode("asset.renamed")
	require.Equal(t, model.SeverityTrace, renamed.Severity)
	require.Contains(t, renamed.Body, "Old Name")
	require.Contains(t, renamed.Body, "New Name")
}

// TestChatLog_MainEventStillAnchors guards the other direction: inside the main
// event the anchor must survive, since the Public Record sidebar's jump-to-row
// gesture depends on it.
func TestChatLog_MainEventStillAnchors(t *testing.T) {
	h := newAssetHarness(t, 2)
	h.setPhaseAndRow(model.PhaseMainEvent, 4)

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "Anchored", "marginalia": []string{"present"},
	})
	require.NotZero(t, assetIDFromBody(t, body))

	created := h.postBySystemCode("asset.created")
	require.NotNil(t, created.RowNumber)
	require.Equal(t, int16(4), *created.RowNumber)
}

// TestChatLog_SecretWritten: writing a secret logs that it happened and names
// the asset — but the secret's TEXT must never appear, since the action log is
// readable by the whole table and carrying it would defeat the mechanic. Only
// existence is public (mirroring the open-eye counter on asset cards).
func TestChatLog_SecretWritten(t *testing.T) {
	h := newAssetHarness(t, 2)
	author := h.tg.Players[0].DisplayName

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "The Confessor", "marginalia": []string{"keeps counsel"},
	})
	id := assetIDFromBody(t, body)

	const secretText = "poisoned the old king"
	code, _ := h.do("POST", 0, assetPath(id, "/secrets"), map[string]any{"text": secretText})
	require.Equal(t, http.StatusCreated, code)

	p := h.postBySystemCode("secret.written")
	require.Equal(t, model.SeverityDefault, p.Severity)
	require.Contains(t, p.Body, author)
	require.Contains(t, p.Body, "The Confessor")
	require.NotContainsf(t, p.Body, secretText,
		"secret text must never reach the action log")

	// Belt and braces: no post in the whole feed may carry the text.
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	for _, post := range posts {
		require.NotContainsf(t, post.Body, secretText,
			"secret text leaked into post %d (%v)", post.ID, post.SystemCode)
	}
}

// TestChatLog_SecretWrittenInPrologue: secrets are written during the prologue,
// which is exactly where the row-anchor bug swallowed posts.
func TestChatLog_SecretWrittenInPrologue(t *testing.T) {
	h := newAssetHarness(t, 2)

	_, body := h.do("POST", 0, h.tablePath(), map[string]any{
		"asset_type": "peer", "name": "The Confessor", "marginalia": []string{"keeps counsel"},
	})
	id := assetIDFromBody(t, body)

	h.assertUnanchoredLog(model.PhasePrologue, 0, "secret.written", func() {
		code, _ := h.do("POST", 0, assetPath(id, "/secrets"), map[string]any{"text": "a buried thing"})
		require.Equal(t, http.StatusCreated, code)
	})
}
