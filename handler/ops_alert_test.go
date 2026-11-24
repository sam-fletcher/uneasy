package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PostOpsAlert posts to whatever webhook is configured. Aim it at a local
// stand-in server, never a real Discord webhook.
func TestPostOpsAlert_PostsContentToWebhook(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var p map[string]string
		_ = json.Unmarshal(b, &p)
		got <- p["content"]
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	SetDiscordWebhookURL(srv.URL)
	t.Cleanup(func() { SetDiscordWebhookURL("") })

	PostOpsAlert("database failing")

	select {
	case msg := <-got:
		assert.Equal(t, "database failing", msg)
	case <-time.After(3 * time.Second):
		t.Fatal("webhook never received the alert")
	}
}

// A hung Discord must not block the caller: PostOpsAlert returns immediately.
func TestPostOpsAlert_ReturnsImmediatelyWhenWebhookHangs(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	SetDiscordWebhookURL(srv.URL)
	t.Cleanup(func() { SetDiscordWebhookURL("") })

	start := time.Now()
	PostOpsAlert("x")
	require.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestPostOpsAlert_TruncatesToDiscordLimit(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var p map[string]string
		_ = json.Unmarshal(b, &p)
		got <- p["content"]
	}))
	defer srv.Close()
	SetDiscordWebhookURL(srv.URL)
	t.Cleanup(func() { SetDiscordWebhookURL("") })

	PostOpsAlert(strings.Repeat("a", 5000))
	select {
	case msg := <-got:
		assert.Len(t, []rune(msg), discordMessageCap)
	case <-time.After(3 * time.Second):
		t.Fatal("no post")
	}
}
