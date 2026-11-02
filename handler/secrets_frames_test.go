//go:build integration

package handler

import (
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	dbgen "uneasy/db/gen"
	"uneasy/hub"

	"github.com/stretchr/testify/require"
)

// captureFrames registers a connection-less client on the game's hub and
// returns a func giving every frame broadcast to it so far — what an open
// browser tab would have received.
func captureFrames(t *testing.T, m *hub.Manager, gameID int64, player dbgen.Player) func() []string {
	t.Helper()
	h := m.GetOrCreate(gameID)
	c := hub.NewClient(h, nil, player, slog.Default())
	require.True(t, h.Register(c))
	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case f := <-c.Outbox():
				mu.Lock()
				got = append(got, string(f))
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()
	return func() []string {
		// Broadcasts are delivered by the hub goroutine; give it a moment.
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// assertSecretCreatedFrame fails unless a secret.created frame for the asset
// reached the captured client, and none of its frames carry the secret text.
func assertSecretCreatedFrame(t *testing.T, frames func() []string, assetID int64) {
	t.Helper()
	found := false
	for _, f := range frames() {
		if strings.Contains(f, `"secret.created"`) && strings.Contains(f, `"asset_id":`+itoa(assetID)) {
			found = true
		}
	}
	require.True(t, found, "secret.created for asset %d was not broadcast", assetID)
}
