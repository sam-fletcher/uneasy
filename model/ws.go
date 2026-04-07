// WebSocket message types shared between the hub and the handler.
package model

// ── Event types (server → client) ────────────────────────────────────────────

const (
	// EventPostCreated is broadcast whenever a new post is created.
	EventPostCreated = "post.created"

	// EventPresenceSnapshot is sent to all clients when anyone connects or
	// disconnects, giving a full view of who is currently online.
	EventPresenceSnapshot = "presence.snapshot"

	// EventTypingUpdate is broadcast when a client starts or stops typing.
	EventTypingUpdate = "typing.update"
)

// ── Command types (client → server) ──────────────────────────────────────────

const (
	CmdTypingStart = "typing.start"
	CmdTypingStop  = "typing.stop"
)

// ── Message envelope ─────────────────────────────────────────────────────────

// WSMessage is the JSON envelope for every WebSocket message in both
// directions: {"type": "...", "payload": {...}}.
type WSMessage struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// ── Payload types ─────────────────────────────────────────────────────────────

// PostCreatedPayload is the payload for EventPostCreated.
type PostCreatedPayload struct {
	Post Post `json:"post"`
}

// PresenceMember is one entry in a presence snapshot.
type PresenceMember struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Online      bool   `json:"online"`
}

// PresenceSnapshotPayload is the payload for EventPresenceSnapshot.
type PresenceSnapshotPayload struct {
	Members []PresenceMember `json:"members"`
}

// TypingUpdatePayload is the payload for EventTypingUpdate.
type TypingUpdatePayload struct {
	PlayerID    int64  `json:"player_id"`
	DisplayName string `json:"display_name"`
	Typing      bool   `json:"typing"`
}
