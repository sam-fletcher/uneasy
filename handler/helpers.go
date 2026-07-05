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

// Length caps (runes, after trimming) for free-text fields, generous enough
// that no honest player ever hits them. Counts (e.g. maxMarginalia) are
// capped separately; these bound the SIZE of each entry against a hostile
// client sending an oversized body.
const (
	maxUsernameLen = 40
	maxEmailLen    = 254
	// maxAssetNameLen bounds a player-authored name: assets/peers, titles,
	// festivity resources, decree resources.
	maxAssetNameLen = 120
	// maxMarginaliaLen bounds a single marginalia entry, a tone topic, and
	// prologue card text.
	maxMarginaliaLen = 300
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
