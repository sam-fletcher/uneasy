//go:build integration

// handler/title_claim_integration_test.go — ADR-007 Phase B: title claims stamp
// the canonical title id and trip the throne_established gate. Covers all three
// Prologue title-claim surfaces (main-character claim, non-monarch claim, the
// ≤3-player extra-peer claim) plus the title-immutability guarantee on the
// marginalia text-update path.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// seedProloguePlayer creates a one-player game parked in the prologue with a
// main-character peer, ready for a recordPrologueChoice title claim.
func seedProloguePlayer(t *testing.T, q *dbgen.Queries, label string) (dbgen.Game, dbgen.Player, dbgen.Asset) {
	t.Helper()
	ctx := context.Background()
	game, err := q.CreateGame(ctx, label)
	require.NoError(t, err)
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: game.ID, Phase: model.PhasePrologue,
	}))
	acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: label + "-" + game.JoinCode, PasswordHash: "x",
	})
	require.NoError(t, err)
	player, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
		GameID: game.ID, DisplayName: "Claimant", AccountID: acct.ID,
	})
	require.NoError(t, err)
	mc, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: game.ID, OwnerID: player.ID, CreatorID: player.ID,
		AssetType: model.AssetPeer, Name: "The Heir Apparent", IsMainCharacter: true,
	})
	require.NoError(t, err)
	return game, player, mc
}

// titleOnAsset returns the stamped title id of the (single) title-bearing
// marginalia on an asset, or "" if none.
func titleOnAsset(t *testing.T, q *dbgen.Queries, assetID int64) string {
	t.Helper()
	margs, err := q.ListMarginaliaByAsset(context.Background(), assetID)
	require.NoError(t, err)
	for _, m := range margs {
		if m.Title != nil {
			return *m.Title
		}
	}
	return ""
}

// TestPrologueMonarchClaim_StampsTitleAndTripsGate: claiming The Monarch in the
// Prologue stamps title=monarch on the MC marginalia, trips throne_established,
// and makes currentMonarch resolve to the claimer.
func TestPrologueMonarchClaim_StampsTitleAndTripsGate(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	game, player, mc := seedProloguePlayer(t, q, "pmonarch")

	body := &chooseRequestBody{
		SheetType:       gamepkg.PrologueSheetTitles,
		ChoiceName:      "The Monarch",
		AssetText:       "The Iron Crown",
		AssetMarginalia: "Forged in the old style",
		MarginaliumText: "Sovereign of the Vale",
		CardAssets: []CardAssetText{
			{Suit: "C", Value: "K", Text: "Household Guard"},
			{Suit: "D", Value: "K", Text: "Crown Jewels"},
		},
	}
	choice := gamepkg.FindPrologueChoice(body.SheetType, body.ChoiceName)
	require.NotNil(t, choice)
	_, err := recordPrologueChoice(ctx, q, manager, game.ID, player.ID, body, choice)
	require.NoError(t, err)

	assert.Equal(t, gamepkg.TitleMonarch, titleOnAsset(t, q, mc.ID), "title stamped on MC marginalia")

	g, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.True(t, g.ThroneEstablished, "monarch claim trips the throne gate")

	gotAsset, gotOwner, ok, err := currentMonarch(ctx, q, game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, mc.ID, gotAsset)
	assert.Equal(t, player.ID, gotOwner)
}

// TestPrologueNonMonarchClaim_StampsButNoGate: a non-monarch title (Spymaster)
// is stamped, but the throne gate stays closed — and even with no gate,
// currentMonarch returns none (Spymaster is outside the line of succession).
func TestPrologueNonMonarchClaim_StampsButNoGate(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	manager := hub.NewManager()

	game, player, mc := seedProloguePlayer(t, q, "pspy")

	body := &chooseRequestBody{
		SheetType:       gamepkg.PrologueSheetTitles,
		ChoiceName:      "The Spymaster",
		AssetText:       "The Cipher Ring",
		AssetMarginalia: "Never removed, even to bathe",
		MarginaliumText: "Eyes in every room",
		CardAssets: []CardAssetText{
			{Suit: "D", Value: "A", Text: "Informant Network"},
			{Suit: "D", Value: "J", Text: "Coded Ledgers"},
		},
	}
	choice := gamepkg.FindPrologueChoice(body.SheetType, body.ChoiceName)
	require.NotNil(t, choice)
	_, err := recordPrologueChoice(ctx, q, manager, game.ID, player.ID, body, choice)
	require.NoError(t, err)

	assert.Equal(t, gamepkg.TitleSpymaster, titleOnAsset(t, q, mc.ID))

	g, err := q.GetGameByID(ctx, game.ID)
	require.NoError(t, err)
	assert.False(t, g.ThroneEstablished, "a non-monarch title must not establish the throne")

	_, _, ok, err := currentMonarch(ctx, q, game.ID)
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestUpdateMarginaliaText_PreservesTitle locks the immutability guarantee: the
// text-update path only touches text, so a title-bearing marginalia keeps its
// title — there is no way to relabel an ordinary note into a title (or vice
// versa) through editing.
func TestUpdateMarginaliaText_PreservesTitle(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()

	game, player, mc := seedProloguePlayer(t, q, "pedit")
	_ = player

	titleID := gamepkg.TitleConsort
	m, err := q.CreateTitleMarginalia(ctx, dbgen.CreateTitleMarginaliaParams{
		AssetID: mc.ID, Position: 1, Text: "Royal spouse", Title: &titleID,
	})
	require.NoError(t, err)

	require.NoError(t, q.UpdateMarginaliaText(ctx, dbgen.UpdateMarginaliaTextParams{
		ID: m.ID, Text: "Royal spouse, but ambitious",
	}))

	updated, err := q.GetMarginaliaByID(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "Royal spouse, but ambitious", updated.Text, "text edit applied")
	require.NotNil(t, updated.Title, "title survives a text edit")
	assert.Equal(t, gamepkg.TitleConsort, *updated.Title)
	_ = game
}

// TestExtraPeerMonarchClaim_StampsAndTripsGate: the ≤3-player extra-peer claim
// of The Monarch stamps the title on the peer's marginalia, trips the gate, and
// makes currentMonarch resolve to the extra peer (a non-main-character asset).
func TestExtraPeerMonarchClaim_StampsAndTripsGate(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	ctx := context.Background()
	store := db.NewStore(pool)
	manager := hub.NewManager()

	tg := newTestGame(t, q, 3)
	manager.GetOrCreate(tg.Game.ID)
	require.NoError(t, q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: tg.Game.ID, Phase: model.PhasePrologue,
	}))
	step := gamepkg.PrologueStepExtraPeers
	require.NoError(t, q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: tg.Game.ID, PrologueRankingStep: &step,
	}))

	router := chi.NewRouter()
	router.Use(appMiddleware.EnsureSession(q))
	router.Post("/tables/{id}/prologue/extra-peer", CreateExtraPeer(store, manager))

	actor := tg.Players[0]
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = q.CreateSession(ctx, dbgen.CreateSessionParams{Token: tok, AccountID: actor.AccountID})
	require.NoError(t, err)

	raw, err := json.Marshal(map[string]any{"title_name": "The Monarch", "peer_text": "Lord Vex"})
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/tables/"+strconv.FormatInt(tg.Game.ID, 10)+"/prologue/extra-peer", bytes.NewReader(raw))
	req.AddCookie(&http.Cookie{Name: "player_token", Value: tok})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equalf(t, http.StatusOK, rec.Code, "extra-peer failed: %s", rec.Body.String())

	var resp struct {
		Asset dbgen.Asset `json:"asset"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, gamepkg.TitleMonarch, titleOnAsset(t, q, resp.Asset.ID), "title stamped on extra peer")

	g, err := q.GetGameByID(ctx, tg.Game.ID)
	require.NoError(t, err)
	assert.True(t, g.ThroneEstablished, "extra-peer monarch claim trips the throne gate")

	gotAsset, gotOwner, ok, err := currentMonarch(ctx, q, tg.Game.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, resp.Asset.ID, gotAsset, "extra peer is the monarch asset")
	assert.Equal(t, actor.ID, gotOwner)
}
