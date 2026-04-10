package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

// parseGamePlayer extracts the game ID from the "{id}" URL param, looks up
// the authenticated player from context, and verifies they belong to that
// game. On failure it writes the appropriate error response and returns
// (0, nil, false). Callers should return immediately when ok is false.
func parseGamePlayer(w http.ResponseWriter, r *http.Request) (int64, *dbgen.Player, bool) {
	gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid table id")
		return 0, nil, false
	}
	player := appMiddleware.PlayerFromContext(r.Context())
	if player == nil || player.GameID != gameID {
		respondErr(w, http.StatusForbidden, "not a member of this table")
		return 0, nil, false
	}
	return gameID, player, true
}
