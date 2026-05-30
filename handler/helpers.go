package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

// truncateLabel trims s and shortens it to at most maxRunes runes, appending
// an ellipsis when it had to cut. Used to derive short asset names / log
// bodies from free-text fields. Returns "" for blank input.
func truncateLabel(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return string(r[:maxRunes])
	}
	return string(r[:maxRunes-1]) + "…"
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
