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
	"time"

	"github.com/coder/websocket"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

const (
	messageBufferSize = 256 // Channel buffer depth for broadcast and client sends

	// pingInterval is how often the server sends a WebSocket ping. The browser
	// answers with an automatic pong; this keeps idle connections alive across
	// proxies (LBs, dev tooling) that would otherwise close them, and lets us
	// notice dead peers within a bounded window.
	pingInterval = 30 * time.Second
	// pingTimeout is how long we wait for the pong before declaring the
	// connection dead and tearing it down.
	pingTimeout = 10 * time.Second
)

// ── Manager ───────────────────────────────────────────────────────────────────

// Manager creates and tracks one Hub per active game table.
// It is safe for concurrent use.
type Manager struct {
	mu   sync.RWMutex
	hubs map[int64]*Hub
	// presence refcounts live WebSocket connections per account, across every
	// table (app-wide, not per-game): an account with two tabs open counts 2.
	// Maintained by each hub's run() goroutine on the register / unregister /
	// force-drop paths, read by HTTP handlers via IsAccountOnline.
	presence map[int64]int
}

// NewManager returns a ready Manager.
func NewManager() *Manager {
	return &Manager{hubs: make(map[int64]*Hub), presence: make(map[int64]int)}
}

// IsAccountOnline reports whether the account has at least one live WebSocket
// connection to any table. "Online" therefore means "has a table open right
// now" — a user reading their profile page (no WS) counts as offline.
func (m *Manager) IsAccountOnline(accountID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.presence[accountID] > 0
}

// presenceInc / presenceDec adjust the connection refcount for an account.
// Called only from hub run() goroutines, mirroring the exact points where a
// client enters or leaves a hub's clients map so the two can't drift.
func (m *Manager) presenceInc(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.presence[accountID]++
}

func (m *Manager) presenceDec(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.presence[accountID] <= 1 {
		delete(m.presence, accountID)
		return
	}
	m.presence[accountID]--
}

// GetOrCreate returns the hub for tableID, creating and starting it if needed.
//
// The returned hub may be concurrently shutting down (its last client just
// left). Callers registering a client must use the Register return value and
// retry with a fresh GetOrCreate on failure — see handler.WebSocket.
func (m *Manager) GetOrCreate(tableID int64) *Hub {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.hubs[tableID]; ok {
		return h
	}
	h := newHub(tableID, m)
	m.hubs[tableID] = h
	go h.run()
	return h
}

// remove deletes the tableID entry iff it still maps to h — a dying hub must
// not evict the fresh hub that may have already replaced it.
func (m *Manager) remove(tableID int64, h *Hub) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hubs[tableID] == h {
		delete(m.hubs, tableID)
	}
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
//
// A hub's run() goroutine exits when its last client leaves (see
// dieIfEmpty), removing itself from the Manager so idle tables cost
// nothing. `done` is closed at that point; Register and Unregister select
// against it so they can never block on a dead hub.
type Hub struct {
	tableID    int64
	manager    *Manager
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	done       chan struct{}
}

func newHub(tableID int64, m *Manager) *Hub {
	return &Hub{
		tableID:    tableID,
		manager:    m,
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, messageBufferSize),
		done:       make(chan struct{}),
	}
}

// run is the hub's event loop. It must be called in its own goroutine.
// It returns — ending the hub's life — once the last client is gone.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
			h.manager.presenceInc(client.player.AccountID)
			h.pushPresence()

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.manager.presenceDec(client.player.AccountID)
				if h.dieIfEmpty() {
					return
				}
				h.pushPresence()
			}

		case msg := <-h.broadcast:
			dropped := false
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					// Client's send buffer is full — drop it.
					close(client.send)
					delete(h.clients, client)
					h.manager.presenceDec(client.player.AccountID)
					dropped = true
				}
			}
			// Only a drop can empty the map here. A fresh hub whose first
			// client hasn't registered yet must not die on a stray
			// broadcast — its Register is already on the way.
			if dropped && h.dieIfEmpty() {
				return
			}
		}
	}
}

// dieIfEmpty ends the hub's life if no clients remain: it removes the hub
// from the Manager, then closes done to unblock any concurrent Register /
// Unregister. That order matters — once done is observable, a retrying
// GetOrCreate must already see the manager slot free (or replaced).
// Returns true if the caller (run) should return. Must only be called from
// within run().
func (h *Hub) dieIfEmpty() bool {
	if len(h.clients) > 0 {
		return false
	}
	h.manager.remove(h.tableID, h)
	close(h.done)
	return true
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

// Register adds a client to the hub. Call before starting the client's
// pumps. Returns false if the hub shut down before the client could be
// added — the caller should fetch a fresh hub via GetOrCreate and retry.
func (h *Hub) Register(c *Client) bool {
	select {
	case h.register <- c:
		return true
	case <-h.done:
		return false
	}
}

// Unregister removes a client from the hub. Called by the client's read pump
// on disconnect. A no-op if the hub already shut down (possible when the
// client was force-dropped for a full send buffer and its pumps unwound
// after the hub died).
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
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
	player dbgen.Player
	send   chan []byte
	log    *slog.Logger
}

// NewClient constructs a Client. Call hub.Register(c) before calling c.Run().
func NewClient(h *Hub, conn *websocket.Conn, player dbgen.Player, logger *slog.Logger) *Client {
	return &Client{
		hub:    h,
		conn:   conn,
		player: player,
		send:   make(chan []byte, messageBufferSize),
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

// writePump drains c.send and writes messages to the WebSocket. It also
// sends periodic pings so idle connections survive intermediary proxies and
// so dead peers are noticed within ~pingInterval+pingTimeout.
func (c *Client) writePump(ctx context.Context) {
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// Hub closed the channel — shut down cleanly.
				if err := c.conn.Close(websocket.StatusNormalClosure, ""); err != nil {
					c.log.ErrorContext(ctx, "writePump: close connection", "error", err)
				}
				return
			}
			if err := c.conn.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		case <-pingTicker.C:
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				// No pong in time (or ctx cancelled) — peer is gone. Closing
				// the connection unblocks readPump, which Unregisters us.
				_ = c.conn.Close(websocket.StatusGoingAway, "ping timeout")
				return
			}
		case <-ctx.Done():
			if err := c.conn.Close(websocket.StatusNormalClosure, ""); err != nil {
				c.log.ErrorContext(ctx, "writePump: close connection", "error", err)
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
	case model.CmdSceneSetupDraft:
		// Ephemeral fan-out of the focus player's in-flight scene-setup
		// selections. We don't validate (it's a UI hint, not a state
		// change) but we do stamp PlayerID from the authenticated client
		// so peers can trust the source.
		var msg struct {
			Payload model.SceneSetupDraftPayload `json:"payload"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		msg.Payload.PlayerID = c.player.ID
		c.hub.BroadcastEvent(model.EventSceneSetupDraft, msg.Payload)
	case model.CmdPreparePlanDraft:
		// Ephemeral fan-out of the focus player's currently-highlighted
		// plan card. Like the scene-setup draft this is a UI hint, not a
		// state change; PlayerID is stamped from the authenticated client.
		var msg struct {
			Payload model.PreparePlanDraftPayload `json:"payload"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		msg.Payload.PlayerID = c.player.ID
		c.hub.BroadcastEvent(model.EventPreparePlanDraft, msg.Payload)
	}
}
