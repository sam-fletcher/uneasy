package handler

// asset_suggestions.go — type-keyed inspiration for player-authored asset text.
//
// Two "blank canvas" surfaces want the same nudge: a few example strings keyed
// by asset type, with anything already in play filtered out (for creative
// diversity — the same example is never offered twice across the game).
//
//   kind=name        → asset-name examples (game.PrologueExamples)
//   kind=marginalia  → marginalia examples (game.MarginaliaExamples)
//
// Both dedupe against what already exists in the game (asset names / marginalia
// text). Fewer than `suggestionCount` (possibly zero) come back when the unused
// pool is small; the client renders blanks for the remainder.

import (
	"math/rand/v2"
	"net/http"
	"strings"

	"uneasy/db"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// suggestionCount is how many example strings a suggestion endpoint returns.
const suggestionCount = 3

// normForDedup lowercases and trims so "Impenetrable" and " impenetrable "
// count as the same example.
func normForDedup(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// pickUnusedSuggestions returns up to n pool entries not present in used
// (compared case-insensitively, trimmed), shuffled for variety.
func pickUnusedSuggestions(pool []string, used map[string]struct{}, n int) []string {
	available := make([]string, 0, len(pool))
	for _, s := range pool {
		if _, taken := used[normForDedup(s)]; taken {
			continue
		}
		available = append(available, s)
	}
	rand.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] })
	if len(available) > n {
		available = available[:n]
	}
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
			pool = gamepkg.PrologueExamples[assetType]
			if assets, err := s.Q.ListAssetsByGame(ctx, gameID); err == nil {
				for _, a := range assets {
					used[normForDedup(a.Name)] = struct{}{}
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
			"suggestions": pickUnusedSuggestions(pool, used, suggestionCount),
			"asset_type":  assetType,
		})
	}
}
