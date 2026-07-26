package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

// Length caps (runes, after trimming) for free-text fields. Counts (e.g.
// maxMarginalia) are capped separately; these bound the SIZE of each entry
// against a hostile client sending an oversized body.
//
// The first three are PRODUCT limits, each read off the narrowest surface
// that has to render the value in full — not "generous enough that no honest
// player ever hits them". The rest are anti-abuse bounds.
const (
	// The player name has to fit the table header's pill strip, shared
	// between up to five players. Mirrored in
	// frontend/src/lib/textLimits.ts (USERNAME), which explains the 20.
	maxUsernameLen = 20
	maxEmailLen    = 254
	// maxAssetNameLen bounds a player-authored name: assets/peers, titles,
	// festivity resources, decree resources. Like maxUsernameLen this is a
	// PRODUCT limit read off a surface, not an anti-abuse bound. Mirrored in
	// frontend/src/lib/textLimits.ts (NAME), which explains the 50.
	maxAssetNameLen = 50
	// maxMarginaliaLen bounds a single marginalia entry, a scene time-note,
	// and prologue card text. Also a product limit; see textLimits.ts
	// (MARGINALIA) for the 160.
	maxMarginaliaLen = 160
	// maxToneTopicLen bounds a custom tone topic. A topic is allowed to grow
	// its tile rather than clip, so this is a bound on how tall it may get:
	// in the 114px tone-grid column, 65 runes wraps to 5 lines (~95px, about
	// double the 44px base tile). The 24 built-in topics all fit the natural
	// 2-line tile — the longest, "Distressing medical practices", is 29.
	// Mirrored in textLimits.ts (TONE_TOPIC).
	maxToneTopicLen = 65
	// maxNarrativeLen bounds secrets, scene/record summaries, prep notes, and
	// other plan-resolution free text (questions, answers, declared truths,
	// war terms).
	maxNarrativeLen = 1000
	// maxLongTextLen bounds chat posts, laws, rumors, and chronicle scenes.
	maxLongTextLen = 5000
)

// textField trims value and, if it exceeds maxLen runes, writes a 400 naming
// the field and the limit and returns ok=false. Otherwise returns the trimmed
// value. Does not enforce non-emptiness — callers that require a non-empty
// field already check that separately.
func textField(w http.ResponseWriter, name, value string, maxLen int) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) > maxLen {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("%s must be at most %d characters", name, maxLen))
		return "", false
	}
	return trimmed, true
}

// textFieldSlice applies textField to every marginalia entry in values,
// stopping (and having already written the 400) at the first one that's too
// long.
func textFieldSlice(w http.ResponseWriter, values []string, maxLen int) ([]string, bool) {
	out := make([]string, len(values))
	for i, v := range values {
		trimmed, ok := textField(w, "marginalia", v, maxLen)
		if !ok {
			return nil, false
		}
		out[i] = trimmed
	}
	return out, true
}

// parseGamePlayer extracts the game ID from the "{id}" URL param and loads
// the calling account's player row at that game. Writes the appropriate
// error response and returns ok=false on failure.
func parseGamePlayer(w http.ResponseWriter, r *http.Request, q *dbgen.Queries) (int64, *dbgen.Player, bool) {
	gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid table id")
		return 0, nil, false
	}
	player, ok := requirePlayerInGame(w, r, q, gameID)
	if !ok {
		return 0, nil, false
	}
	return gameID, player, true
}

// requirePlayerInGame loads the calling account's player row at gameID,
// or writes 401/403 and returns ok=false. Use when the gameID has already
// been resolved from a sub-resource (asset, plan, roll, ...).
func requirePlayerInGame(w http.ResponseWriter, r *http.Request, q *dbgen.Queries, gameID int64) (*dbgen.Player, bool) {
	account := appMiddleware.AccountFromContext(r.Context())
	if account == nil {
		respondErr(w, http.StatusUnauthorized, "log in first")
		return nil, false
	}
	player := appMiddleware.LoadPlayer(r.Context(), q, account.ID, gameID)
	if player == nil {
		respondErr(w, http.StatusForbidden, "not a member of this table")
		return nil, false
	}
	return player, true
}
