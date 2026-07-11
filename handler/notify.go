package handler

// notify.go — Discord webhook notifier for feedback_submissions rows
// (adr/FEEDBACK_AND_RESET_PLAN.md, decision 3). Callers fire this in a
// goroutine after their DB commit: a failed or slow webhook must never add
// latency to the caller's response or affect the row already committed —
// the DB row is the durable record, this is best-effort notification only.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"uneasy/model"
)

// discordWebhookURL is set once at startup by SetDiscordWebhookURL, mirroring
// the secureCookies pattern in config.go. Left empty in dev (and any deploy
// that hasn't set DISCORD_WEBHOOK_URL yet) — NotifyDiscord falls back to a
// structured stdout log line instead of posting.
var discordWebhookURL string

// SetDiscordWebhookURL configures discordWebhookURL. Call once from main.go
// before serving requests.
func SetDiscordWebhookURL(url string) {
	discordWebhookURL = url
}

// discordMessageCap is Discord's hard limit on a webhook's "content" field —
// longer payloads are rejected outright, so truncate rather than risk losing
// the notification entirely. The full, untruncated text always lives in the
// feedback_submissions row.
const discordMessageCap = 2000

// discordPostTimeout bounds the webhook POST. No retries: by the time this
// runs the row is already committed, so a failed ping loses nothing.
const discordPostTimeout = 5 * time.Second

// NotifyDiscord posts a best-effort notification for a just-committed
// feedback_submissions row. Call it in a goroutine, never inline on the
// request path.
//
// username is the account's username for kind=feedback, or the as-typed
// username for kind=reset_request. gameLabel, route, and contact may be
// empty (no game context / no route / no reply channel given).
func NotifyDiscord(kind model.FeedbackKind, username, gameLabel, route, body, contact string) {
	ctx, cancel := context.WithTimeout(context.Background(), discordPostTimeout)
	defer cancel()
	logger := slog.Default()

	msg := formatDiscordMessage(kind, username, gameLabel, route, body, contact)

	if discordWebhookURL == "" {
		logger.InfoContext(ctx, "feedback submission (DISCORD_WEBHOOK_URL unset)", "kind", kind, "message", msg)
		return
	}

	payload, err := json.Marshal(map[string]string{"content": msg})
	if err != nil {
		logger.ErrorContext(ctx, "discord notify: encode payload", "err", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordWebhookURL, bytes.NewReader(payload))
	if err != nil {
		logger.ErrorContext(ctx, "discord notify: build request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.WarnContext(ctx, "discord notify: post failed", "err", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		logger.WarnContext(ctx, "discord notify: non-2xx response", "status", resp.StatusCode)
	}
}

// formatDiscordMessage builds the webhook content, truncated to
// discordMessageCap runes. Split out from NotifyDiscord so tests can assert
// on the formatting/truncation without a network round trip.
func formatDiscordMessage(kind model.FeedbackKind, username, gameLabel, route, body, contact string) string {
	header := fmt.Sprintf("**%s** from %s", kind, username)
	if gameLabel != "" {
		header += " (" + gameLabel + ")"
	}
	if route != "" {
		header += " @ " + route
	}
	msg := header + "\n" + body
	if contact != "" {
		msg += "\ncontact: " + contact
	}
	return truncateRunes(msg, discordMessageCap)
}

// truncateRunes shortens s to at most maxLen runes, appending an ellipsis in
// place of the last rune when truncated. Rune-based (not byte-based) so a
// multi-byte character is never split.
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
