package handler

import (
	"net/http"

	"uneasy/db"
)

// GetSeenMarkers handles GET /api/tables/{id}/seen.
//
// Returns the caller's three "new to me" markers and nothing else:
// {"laws": N, "rumors": N, "last_read_post_id": N}. The Laws and Rumors markers
// are the ones GET /state carries as laws_rumors_seen, and the chat marker is
// the one GET /posts carries; this is the same data without the board or the
// feed around it.
//
// Why it exists: the markers are per player, and moving one (reading Laws,
// reading chat) is private, so nothing is broadcast to the table. A player's
// other open tabs and devices therefore keep a dot the player has already
// cleared elsewhere, until they reload. A tab that comes back to the foreground
// calls this to catch up. The only other route to the Laws and Rumors markers is
// the whole /state payload, which is too much to fetch every time a phone wakes.
//
// Hidden information: these are the viewer's own markers, selected by the
// viewer's own player id, so another player's marker is unreachable from here
// (migration 058 explains why the Laws and Rumors ones live in their own table:
// a hidden-source rumor's writer could be named by a marker anyone could read).
func GetSeenMarkers(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		seen, err := s.Q.GetLawsRumorsSeen(r.Context(), player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not read seen markers", err)
			return
		}
		respond(w, http.StatusOK, map[string]any{
			"laws":              seen.LawsSeenRev,
			"rumors":            seen.RumorsSeenRev,
			"last_read_post_id": player.LastReadPostID,
		})
	}
}
