package hub

// Lifecycle tests for the hub's die-when-empty behavior. Clients here are
// constructed with a nil websocket.Conn and never Run() — the tests drive
// Register/Unregister/Broadcast directly and read from client send channels,
// which is exactly the surface run() owns.

import (
	"log/slog"
	"testing"
	"time"

	dbgen "uneasy/db/gen"
)

func testClient(h *Hub, id int64) *Client {
	return NewClient(h, nil, dbgen.Player{ID: id, DisplayName: "p"}, slog.Default())
}

// waitFor polls cond until it holds or the deadline passes. The hub's state
// changes happen on its own goroutine, so assertions after Register /
// Unregister / Broadcast need to wait for the loop to catch up.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestHubDiesWhenLastClientLeaves(t *testing.T) {
	m := NewManager()
	h := m.GetOrCreate(7)
	c := testClient(h, 1)
	if !h.Register(c) {
		t.Fatal("register on a fresh hub must succeed")
	}

	h.Unregister(c)

	waitFor(t, func() bool {
		_, ok := m.Get(7)
		return !ok
	}, "hub was not removed from the manager after its last client left")

	// The dead hub must refuse further registrations rather than block.
	if h.Register(testClient(h, 2)) {
		t.Fatal("register on a dead hub must return false")
	}
	// And a late Unregister (force-dropped client unwinding) must not hang.
	h.Unregister(c)
}

func TestHubSurvivesWhileClientsRemain(t *testing.T) {
	m := NewManager()
	h := m.GetOrCreate(7)
	c1, c2 := testClient(h, 1), testClient(h, 2)
	h.Register(c1)
	h.Register(c2)
	h.Unregister(c1)

	// c2 is still connected: the hub must stay registered and reachable.
	waitFor(t, func() bool { return len(drain(c2.send)) > 0 },
		"remaining client never saw the presence update")
	if _, ok := m.Get(7); !ok {
		t.Fatal("hub died while it still had a client")
	}
}

func TestGetOrCreateReplacesDeadHub(t *testing.T) {
	m := NewManager()
	h1 := m.GetOrCreate(7)
	c1 := testClient(h1, 1)
	h1.Register(c1)
	h1.Unregister(c1)
	waitFor(t, func() bool {
		_, ok := m.Get(7)
		return !ok
	}, "hub was not removed")

	// The WebSocket handler's retry loop: a fresh GetOrCreate must yield a
	// live replacement that accepts registrations and delivers broadcasts.
	h2 := m.GetOrCreate(7)
	if h2 == h1 {
		t.Fatal("GetOrCreate returned the dead hub")
	}
	c2 := testClient(h2, 2)
	if !h2.Register(c2) {
		t.Fatal("register on the replacement hub failed")
	}
	h2.Broadcast([]byte(`{"type":"x"}`))
	waitFor(t, func() bool {
		for _, msg := range drain(c2.send) {
			if string(msg) == `{"type":"x"}` {
				return true
			}
		}
		return false
	}, "broadcast never reached the client on the replacement hub")
}

func TestSlowClientDropCanEmptyTheHub(t *testing.T) {
	m := NewManager()
	h := m.GetOrCreate(7)
	c := testClient(h, 1)
	h.Register(c)

	// Never drain c.send: after messageBufferSize undelivered messages the
	// broadcast loop force-drops the client, emptying the hub — which must
	// then die, not linger with zero clients.
	for range messageBufferSize + 8 {
		h.Broadcast([]byte("m"))
		time.Sleep(time.Microsecond) // let run() drain its own buffer
	}

	waitFor(t, func() bool {
		_, ok := m.Get(7)
		return !ok
	}, "hub did not die after force-dropping its only client")
}

// drain empties and returns everything currently buffered on ch without
// blocking.
func drain(ch chan []byte) [][]byte {
	var out [][]byte
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, msg)
		default:
			return out
		}
	}
}
