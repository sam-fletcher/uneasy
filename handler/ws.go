package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"uneasy/hub"
	appMiddleware "uneasy/middleware"
)

// WebSocket handles GET /api/tables/{id}/ws.
//
// Upgrades to a WebSocket connection for the given table. The caller must:
//   - Have a valid player_token cookie
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
func WebSocket(pool *pgxpool.Pool, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid table id", http.StatusBadRequest)
			return
		}

		player := appMiddleware.PlayerFromContext(r.Context())
		if player == nil || player.GameID != gameID {
			http.Error(w, "not a member of this table", http.StatusForbidden)
			return
		}

		// Upgrade the HTTP connection to WebSocket.
		// In dev, OriginPatterns is permissive; lock this down for production.
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			// Accept writes its own error response.
			return
		}

		h := manager.GetOrCreate(gameID)
		client := hub.NewClient(h, conn, *player, slog.Default())
		h.Register(client)

		// Run blocks until the connection closes (reads pump + write pump).
		client.Run(r.Context())
	}
}
