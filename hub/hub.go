// Package hub implements the per-table WebSocket hub.
//
// Architecture overview:
//
//	Manager (singleton)
//	  └─ Hub (one goroutine per active game table)
//	       └─ Client (one goroutine pair per connected browser tab)
//
// The Hub's Run() goroutine is the sole owner of the clients map — no mutex
// needed on that map. All mutations go through channels. This is the canonical
// Go concurrency pattern for fan-out over a dynamic set of goroutines.
package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/coder/websocket"

	"uneasy/model"
)

// ── Manager ───────────────────────────────────────────────────────────────────

// Manager creates and tracks one Hub per active game table.
// It is safe for concurrent use.
type Manager struct {
	mu   sync.RWMutex
	hubs map[int64]*Hub
}

// NewManager returns a ready Manager.
func NewManager() *Manager {
	return &Manager{hubs: make(map[int64]*Hub)}
}

// GetOrCreate returns the hub for tableID, creating and starting it if needed.
func (m *Manager) GetOrCreate(tableID int64) *Hub {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.hubs[tableID]; ok {
		return h
	}
	h := newHub(tableID)
	m.hubs[tableID] = h
	go h.run()
	return h
}

// Get returns the hub for tableID if it exists.
func (m *Manager) Get(tableID int64) (*Hub, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.hubs[tableID]
	return h, ok
}

// ── Hub ───────────────────────────────────────────────────────────────────────

// Hub maintains the set of active WebSocket clients for one game table.
// All fields are owned by the run() goroutine; never access them directly.
type Hub struct {
	tableID    int64
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
}

func newHub(tableID int64) *Hub {
	return &Hub{
		tableID:    tableID,
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 256),
	}
}

// run is the hub's event loop. It must be called in its own goroutine.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
			h.pushPresence()

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.pushPresence()
			}

		case msg := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					// Client's send buffer is full — drop it.
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// Broadcast enqueues a JSON message for delivery to all connected clients.
func (h *Hub) Broadcast(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
		// Hub's own buffer full — very unusual; drop rather than block.
	}
}

// BroadcastEvent is a convenience wrapper that marshals a WSMessage and
// calls Broadcast. Marshalling errors are silently dropped (they indicate a
// bug in the caller, not a runtime error worth propagating).
func (h *Hub) BroadcastEvent(eventType string, payload any) {
	msg, err := json.Marshal(model.WSMessage{Type: eventType, Payload: payload})
	if err != nil {
		return
	}
	h.Broadcast(msg)
}

// Register adds a client to the hub. Call before starting the client's pumps.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// Unregister removes a client from the hub. Called by the client's read pump
// on disconnect.
func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

// pushPresence sends a fresh presence snapshot to every connected client.
// Must only be called from within run().
func (h *Hub) pushPresence() {
	members := make([]model.PresenceMember, 0, len(h.clients))
	for c := range h.clients {
		members = append(members, model.PresenceMember{
			ID:          c.player.ID,
			DisplayName: c.player.DisplayName,
			Online:      true,
		})
	}
	msg, err := json.Marshal(model.WSMessage{
		Type:    model.EventPresenceSnapshot,
		Payload: model.PresenceSnapshotPayload{Members: members},
	})
	if err != nil {
		return
	}
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
		}
	}
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client is one connected browser tab. It has two goroutines: readPump (reads
// commands from the browser) and writePump (writes events to the browser).
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	player model.Player
	send   chan []byte
	log    *slog.Logger
}

// NewClient constructs a Client. Call hub.Register(c) before calling c.Run().
func NewClient(h *Hub, conn *websocket.Conn, player model.Player, logger *slog.Logger) *Client {
	return &Client{
		hub:    h,
		conn:   conn,
		player: player,
		send:   make(chan []byte, 256),
		log:    logger.With("player_id", player.ID),
	}
}

// Run starts the write pump in a goroutine and blocks on the read pump until
// the connection closes. This is the correct call pattern from an HTTP handler:
//
//	client := hub.NewClient(h, conn, player)
//	h.Register(client)
//	client.Run(r.Context()) // blocks until disconnected
func (c *Client) Run(ctx context.Context) {
	go c.writePump(ctx)
	c.readPump(ctx)
}

// readPump reads client → server commands until the connection closes.
func (c *Client) readPump(ctx context.Context) {
	defer c.hub.Unregister(c)

	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		c.handleCommand(data)
	}
}

// writePump drains c.send and writes messages to the WebSocket.
func (c *Client) writePump(ctx context.Context) {
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Hub closed the channel — shut down cleanly.
				if err := c.conn.Close(websocket.StatusNormalClosure, ""); err != nil {
					c.log.Error("writePump: close connection", "error", err)
				}
				return
			}
			if err := c.conn.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		case <-ctx.Done():
			if err := c.conn.Close(websocket.StatusNormalClosure, ""); err != nil {
				c.log.Error("writePump: close connection", "error", err)
			}
			return
		}
	}
}

// handleCommand processes a message sent from the client (e.g. typing.start).
func (c *Client) handleCommand(data []byte) {
	var cmd struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &cmd); err != nil {
		return
	}

	switch cmd.Type {
	case model.CmdTypingStart:
		c.hub.BroadcastEvent(model.EventTypingUpdate, model.TypingUpdatePayload{
			PlayerID:    c.player.ID,
			DisplayName: c.player.DisplayName,
			Typing:      true,
		})
	case model.CmdTypingStop:
		c.hub.BroadcastEvent(model.EventTypingUpdate, model.TypingUpdatePayload{
			PlayerID:    c.player.ID,
			DisplayName: c.player.DisplayName,
			Typing:      false,
		})
	}
}
