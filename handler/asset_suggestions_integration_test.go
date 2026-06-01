//go:build integration

// handler/asset_suggestions_integration_test.go — end-to-end coverage for the
// type-keyed suggestion endpoint, focusing on the game-wide dedup that gives
// players fresh, diverse prompts.

package handler

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
)

func suggestionsPath(gameID int64, assetType, kind string) string {
	return "/api/tables/" + strconv.FormatInt(gameID, 10) +
		"/asset-suggestions?asset_type=" + assetType + "&kind=" + kind
}

// respSuggestions pulls the suggestions array out of the JSON body.
func respSuggestions(body map[string]any) []string {
	raw, _ := body["suggestions"].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// TestAssetSuggestions_MarginaliaDedup occupies all but two entries of the peer
// marginalia pool (as real marginalia in the game) and proves the endpoint
// returns exactly the two unused ones — confirming the pool source and the
// game-wide, case-insensitive dedup.
func TestAssetSuggestions_MarginaliaDedup(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	pool := gamepkg.MarginaliaExamples["peer"]
	require.GreaterOrEqual(t, len(pool), 3, "pool must have room to leave 2 unused")
	occupy := pool[:len(pool)-2]
	wantRemaining := pool[len(pool)-2:]

	// Spread the occupied entries across peers (≤4 marginalia per asset).
	var peerID int64
	for i, text := range occupy {
		pos := int16(i%4) + 1
		if pos == 1 {
			peerID = h.seedPeer(0, "carrier "+strconv.Itoa(i))
		}
		_, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: peerID, Position: pos, Text: text,
		})
		require.NoError(t, err)
	}

	code, body := h.get(0, suggestionsPath(h.tg.Game.ID, "peer", "marginalia"))
	require.Equalf(t, 200, code, "asset-suggestions: %v", body)
	got := respSuggestions(body)
	assert.ElementsMatch(t, wantRemaining, got, "only the two unused pool entries should remain")
}

// TestAssetSuggestions_NameDedup is the same proof for kind=name: occupy all
// but two peer-NAME pool entries (as asset names) and expect the two unused.
func TestAssetSuggestions_NameDedup(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	pool := gamepkg.PrologueExamples["peer"]
	require.GreaterOrEqual(t, len(pool), 3)
	occupy := pool[:len(pool)-2]
	wantRemaining := pool[len(pool)-2:]

	for _, name := range occupy {
		_, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID: h.tg.Game.ID, OwnerID: h.tg.Players[0].ID,
			CreatorID: h.tg.Players[0].ID, AssetType: "peer", Name: name,
		})
		require.NoError(t, err)
	}

	code, body := h.get(0, suggestionsPath(h.tg.Game.ID, "peer", "name"))
	require.Equalf(t, 200, code, "asset-suggestions: %v", body)
	got := respSuggestions(body)
	assert.ElementsMatch(t, wantRemaining, got, "only the two unused names should remain")
}

// TestAssetSuggestions_BadParams covers the 400 paths.
func TestAssetSuggestions_BadParams(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	gid := h.tg.Game.ID

	code, _ := h.get(0, suggestionsPath(gid, "dragon", "name"))
	assert.Equal(t, 400, code, "invalid asset_type")

	code, _ = h.get(0, suggestionsPath(gid, "peer", "nonsense"))
	assert.Equal(t, 400, code, "invalid kind")
}
