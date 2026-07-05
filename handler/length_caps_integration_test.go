//go:build integration

// handler/length_caps_integration_test.go — Session 3 spot-checks for the
// textField length-cap sweep (adr/PUBLIC_LAUNCH_PLAN.md Session 3, item 1).
//
// The helper itself is unit-tested in helpers_test.go; these drive one real
// endpoint per cap tier through the actual chi routes to prove textField is
// actually wired in, rather than re-testing every capped field.

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
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

// lengthCapHarness wires just the routes these spot-checks need.
type lengthCapHarness struct {
	t      *testing.T
	tg     testGame
	router http.Handler
	tokens []string
}

func newLengthCapHarness(t *testing.T) *lengthCapHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	store := db.NewStore(pool)
	manager := hub.NewManager()

	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: tg.Players[0].AccountID,
	})
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(appMiddleware.EnsureSession(q))
	r.Post("/api/tables/{id}/assets", CreateAsset(store, manager))
	r.Post("/api/tables/{id}/posts", CreatePlayerPost(store, manager))
	r.Route("/api/assets/{assetId}", func(rr chi.Router) {
		rr.Post("/marginalia", AddMarginalia(store, manager))
		rr.Post("/secrets", WriteSecret(store, manager))
	})

	return &lengthCapHarness{t: t, tg: tg, router: r, tokens: []string{tok}}
}

func (h *lengthCapHarness) do(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[0]})
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

func (h *lengthCapHarness) tablePath(suffix string) string {
	return "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + suffix
}

// TestCreateAsset_RejectsOverlongName spot-checks the maxAssetNameLen (120)
// tier via the asset-creation route.
func TestCreateAsset_RejectsOverlongName(t *testing.T) {
	h := newLengthCapHarness(t)

	code, out := h.do(http.MethodPost, h.tablePath("/assets"), map[string]any{
		"asset_type": "peer",
		"name":       strings.Repeat("a", maxAssetNameLen+1),
		"marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusBadRequest, code, "overlong name rejected: %v", out)
	require.Contains(t, out["error"], "name")

	code, out = h.do(http.MethodPost, h.tablePath("/assets"), map[string]any{
		"asset_type": "peer",
		"name":       strings.Repeat("a", maxAssetNameLen),
		"marginalia": []string{"a trait"},
	})
	require.Equalf(t, http.StatusCreated, code, "name at exactly the cap accepted: %v", out)
}

// TestAddMarginalia_RejectsOverlongText spot-checks the maxMarginaliaLen
// (300) tier via the marginalia route.
func TestAddMarginalia_RejectsOverlongText(t *testing.T) {
	h := newLengthCapHarness(t)

	_, assetOut := h.do(http.MethodPost, h.tablePath("/assets"), map[string]any{
		"asset_type": "peer", "name": "Ally", "marginalia": []string{"a trait"},
	})
	assetID := int64(assetOut["asset"].(map[string]any)["id"].(float64))

	code, out := h.do(http.MethodPost, assetPath(assetID, "/marginalia"), map[string]any{
		"text": strings.Repeat("b", maxMarginaliaLen+1),
	})
	require.Equalf(t, http.StatusBadRequest, code, "overlong marginalia rejected: %v", out)
	require.Contains(t, out["error"], "text")
}

// TestWriteSecret_RejectsOverlongText spot-checks the maxNarrativeLen (1000)
// tier via the secrets route.
func TestWriteSecret_RejectsOverlongText(t *testing.T) {
	h := newLengthCapHarness(t)

	_, assetOut := h.do(http.MethodPost, h.tablePath("/assets"), map[string]any{
		"asset_type": "peer", "name": "Ally", "marginalia": []string{"a trait"},
	})
	assetID := int64(assetOut["asset"].(map[string]any)["id"].(float64))

	code, out := h.do(http.MethodPost, assetPath(assetID, "/secrets"), map[string]any{
		"text": strings.Repeat("c", maxNarrativeLen+1),
	})
	require.Equalf(t, http.StatusBadRequest, code, "overlong secret rejected: %v", out)
	require.Contains(t, out["error"], "text")
}

// TestCreatePlayerPost_RejectsOverlongBody spot-checks the maxLongTextLen
// (5000) tier via the chat-post route.
func TestCreatePlayerPost_RejectsOverlongBody(t *testing.T) {
	h := newLengthCapHarness(t)

	code, out := h.do(http.MethodPost, h.tablePath("/posts"), map[string]any{
		"body": strings.Repeat("d", maxLongTextLen+1),
	})
	require.Equalf(t, http.StatusBadRequest, code, "overlong chat post rejected: %v", out)
	require.Contains(t, out["error"], "body")
}
