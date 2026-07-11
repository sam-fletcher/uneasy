package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/model"
)

func TestFormatDiscordMessage_IncludesAllFields(t *testing.T) {
	msg := formatDiscordMessage(
		model.FeedbackKindFeedback,
		"alice",
		"game #3",
		"/tables/3/plans",
		"it broke",
		"alice@example.com",
	)
	assert.Contains(t, msg, "feedback")
	assert.Contains(t, msg, "alice")
	assert.Contains(t, msg, "game #3")
	assert.Contains(t, msg, "/tables/3/plans")
	assert.Contains(t, msg, "it broke")
	assert.Contains(t, msg, "contact: alice@example.com")
}

func TestFormatDiscordMessage_OmitsEmptyOptionalFields(t *testing.T) {
	msg := formatDiscordMessage(model.FeedbackKindResetRequest, "bob", "", "", "locked out", "")
	assert.NotContains(t, msg, "()")
	assert.NotContains(t, msg, " @ ")
	assert.NotContains(t, msg, "contact:")
}

func TestFormatDiscordMessage_TruncatesToDiscordCap(t *testing.T) {
	huge := strings.Repeat("a", discordMessageCap*2)
	msg := formatDiscordMessage(model.FeedbackKindFeedback, "alice", "", "", huge, "")
	assert.LessOrEqual(t, len([]rune(msg)), discordMessageCap)
	assert.True(t, strings.HasSuffix(msg, "…"))
}

func TestFormatDiscordMessage_MultiByteTruncationStaysValidUTF8(t *testing.T) {
	// Every rune below is multi-byte; a byte-based truncation would risk
	// slicing one in half and producing invalid UTF-8.
	huge := strings.Repeat("é", discordMessageCap*2)
	msg := formatDiscordMessage(model.FeedbackKindFeedback, "alice", "", "", huge, "")
	assert.True(t, isValidUTF8(msg))
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestNotifyDiscord_PostsFormattedContentToConfiguredURL(t *testing.T) {
	var gotBody map[string]string
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Cleanup(func() { SetDiscordWebhookURL("") })

	SetDiscordWebhookURL(srv.URL)
	NotifyDiscord(model.FeedbackKindFeedback, "alice", "game #3", "/route", "hello there", "alice@example.com")

	assert.Equal(t, "application/json", gotContentType)
	require.Contains(t, gotBody, "content")
	assert.Contains(t, gotBody["content"], "hello there")
	assert.Contains(t, gotBody["content"], "alice")
}

func TestNotifyDiscord_NoopWhenURLUnset(t *testing.T) {
	// Nothing to assert on the network side — this just proves the unset
	// (dev-default) path doesn't attempt a POST or panic.
	SetDiscordWebhookURL("")
	NotifyDiscord(model.FeedbackKindFeedback, "alice", "", "", "hello", "")
}
