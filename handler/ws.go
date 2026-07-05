package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"uneasy/db"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

// WebSocket handles GET /api/tables/{id}/ws.
//
// Upgrades to a WebSocket connection for the given table. The caller must:
//   - Have a valid session cookie
//   - Be a member of the table
//
// Once connected, the client receives:
//   - An immediate presence.snapshot showing all online members
//   - scene_post.created events as others post
//   - typing.update events as others type
//
// The client can send:
//   - {"type": "typing.start"} — throttle to once per 2–3 seconds
//   - {"type": "typing.stop"}
//
// originPatterns restricts which Origin headers may complete the WebSocket
// handshake (see coder/websocket's OriginPatterns). main.go passes the
// PUBLIC_ORIGIN host when set, or "*" in dev (localhost-only, so any origin
// is fine).
func WebSocket(s *db.Store, manager *hub.Manager, originPatterns []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid table id", http.StatusBadRequest)
			return
		}

		account := appMiddleware.AccountFromContext(r.Context())
		if account == nil {
			http.Error(w, "log in first", http.StatusUnauthorized)
			return
		}
		player := appMiddleware.LoadPlayer(r.Context(), s.Q, account.ID, gameID)
		if player == nil {
			http.Error(w, "not a member of this table", http.StatusForbidden)
			return
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: originPatterns,
		})
		if err != nil {
			return
		}

		h := manager.GetOrCreate(gameID)
		client := hub.NewClient(h, conn, *player, slog.Default())
		h.Register(client)

		client.Run(r.Context())
	}
}
