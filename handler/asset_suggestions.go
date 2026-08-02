package handler

// asset_suggestions.go — type-keyed inspiration for player-authored asset text.
//
// Two "blank canvas" surfaces want the same nudge: a few example strings keyed
// by asset type, with anything already in play filtered out (for creative
// diversity — the same example is never offered twice across the game).
//
//   kind=name        → asset-name examples (game.AssetExamples)
//   kind=marginalia  → marginalia examples (game.MarginaliaExamples)
//
// Both dedupe against what the game's fiction already contains. The filter is a
// plain normalized string match against persisted content — NOT a record of
// which suggestions were tapped. So a player who takes an example and edits a
// word leaves it on offer, one who submits it verbatim burns it, and one who
// independently types the same string burns it too. Destroyed assets and torn
// marginalia stay burnt: they were in the fiction, and re-introducing them is
// the degenerate move this filter exists to prevent.
//
// The whole unused pool comes back in one response, shuffled — see
// pickUnusedSuggestions. An empty array is a legitimate answer for a game that
// has worked through a pool; the client hides the example row.

import (
	"math/rand/v2"
	"net/http"
	"strings"

	"uneasy/db"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// normForDedup lowercases and trims so "Impenetrable" and " impenetrable "
// count as the same example.
func normForDedup(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// pickUnusedSuggestions returns every pool entry not present in used (compared
// case-insensitively, trimmed), shuffled for variety.
//
// The full pool ships in one response rather than a page of three. A pool is
// ~1KB on the wire and the DB round trip dominates the request either way, so
// batching costs nothing — and it buys the client enough examples that walking
// them never blocks on the network mid-reroll. The shuffle is what makes each
// picker's walk feel fresh; order is stable for the life of one response.
func pickUnusedSuggestions(pool []string, used map[string]struct{}) []string {
	available := make([]string, 0, len(pool))
	for _, s := range pool {
		if _, taken := used[normForDedup(s)]; taken {
			continue
		}
		available = append(available, s)
	}
	rand.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })
	return available
}

// validAssetType reports whether s is one of the four asset types.
func validAssetType(s string) bool {
	switch model.AssetType(s) {
	case model.AssetPeer, model.AssetHolding, model.AssetArtifact, model.AssetResource:
		return true
	default:
		return false
	}
}

// GetAssetSuggestions handles
// GET /api/tables/{id}/asset-suggestions?asset_type=X&kind=name|marginalia.
func GetAssetSuggestions(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		assetType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("asset_type")))
		if !validAssetType(assetType) {
			respondErr(w, http.StatusBadRequest, "asset_type must be one of peer, holding, artifact, resource")
			return
		}
		kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))

		var pool []string
		used := map[string]struct{}{}
		ctx := r.Context()

		switch kind {
		case "name":
			pool = gamepkg.AssetExamples[assetType]
			// Names of destroyed assets included — see ListAssetNamesByGame.
			if names, err := s.Q.ListAssetNamesByGame(ctx, gameID); err == nil {
				for _, n := range names {
					used[normForDedup(n)] = struct{}{}
				}
			}
		case "marginalia":
			pool = gamepkg.MarginaliaExamples[assetType]
			if texts, err := s.Q.ListMarginaliaTextByGame(ctx, gameID); err == nil {
				for _, t := range texts {
					used[normForDedup(t)] = struct{}{}
				}
			}
		default:
			respondErr(w, http.StatusBadRequest, "kind must be 'name' or 'marginalia'")
			return
		}

		respond(w, http.StatusOK, map[string]any{
			"suggestions": pickUnusedSuggestions(pool, used),
			"asset_type":  assetType,
		})
	}
}
