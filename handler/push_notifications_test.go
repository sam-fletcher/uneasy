package handler

// push_notifications_test.go — DB-free coverage for the pure pieces of the
// notification tick: payload encoding and the raw send/prune HTTP mechanics
// (mirrors notify_test.go's split between formatDiscordMessage and
// NotifyDiscord). Waitee reconciliation and the full send+rebump flow need a
// database and live in push_notifications_integration_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
)

func TestBuildPushPayload_EncodesFixedCopy(t *testing.T) {
	raw, err := buildPushPayload("WOLF-1", 42)
	require.NoError(t, err)

	var payload pushPayload
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.Equal(t, pushTitle, payload.Title)
	assert.Equal(t, "Table WOLF-1 is waiting on you.", payload.Body)
	assert.Equal(t, "game-42", payload.Tag)
	assert.Equal(t, "/table/42", payload.URL)
}

// testSubscriptionKeys returns a structurally valid (but not tied to any
// real device) P-256 public key and auth secret — enough for webpush-go's
// ECDH step to succeed so sendPush actually reaches the relay.
func testSubscriptionKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	_, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	return pub, "8Q1Ssq2t9BdMh0R4Y2iSEA"
}

func TestSendPush_SendsToConfiguredEndpoint(t *testing.T) {
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	var gotContentEncoding string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentEncoding = r.Header.Get("Content-Encoding")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	payload, err := buildPushPayload("WOLF-1", 1)
	require.NoError(t, err)

	prune, err := sendPush(context.Background(), srv.URL, p256dh, auth, payload)
	require.NoError(t, err)
	assert.False(t, prune)
	assert.Equal(t, "aes128gcm", gotContentEncoding)
}

func TestSendPush_PrunesOn410Gone(t *testing.T) {
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	payload, err := buildPushPayload("WOLF-1", 1)
	require.NoError(t, err)

	prune, err := sendPush(context.Background(), srv.URL, p256dh, auth, payload)
	require.NoError(t, err)
	assert.True(t, prune)
}

func TestSendPush_PrunesOn404NotFound(t *testing.T) {
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	payload, err := buildPushPayload("WOLF-1", 1)
	require.NoError(t, err)

	prune, err := sendPush(context.Background(), srv.URL, p256dh, auth, payload)
	require.NoError(t, err)
	assert.True(t, prune)
}

func TestSendPush_DoesNotPruneOnServerError(t *testing.T) {
	t.Cleanup(func() { SetVAPIDKeys("", "", "") })
	priv, pub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	SetVAPIDKeys(pub, priv, "mailto:test@example.com")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p256dh, auth := testSubscriptionKeys(t)
	payload, err := buildPushPayload("WOLF-1", 1)
	require.NoError(t, err)

	prune, err := sendPush(context.Background(), srv.URL, p256dh, auth, payload)
	require.NoError(t, err)
	assert.False(t, prune)
}

// TestGroupDueNotifications_CollapsesMultipleSubscriptions exercises the
// grouping in isolation from the DB: two subscription rows for the same
// player collapse into one group with two subs, and a player with no
// subscriptions (NULL fields from the LEFT JOIN) still gets its own
// zero-sub group so its timer can still be re-bumped/cleared.
func TestGroupDueNotifications_CollapsesMultipleSubscriptions(t *testing.T) {
	sub1, sub2 := int64(1), int64(2)
	endpoint1, endpoint2 := "https://a", "https://b"
	p256dh, auth := "p", "a"
	cadence := int16(24)

	rows := []dbgen.ListDueNotificationsWithSubscriptionsRow{
		{PlayerID: 10, GameID: 1, JoinCode: "WOLF-1", NotifyCadenceHours: &cadence,
			SubscriptionID: &sub1, Endpoint: &endpoint1, P256dh: &p256dh, Auth: &auth},
		{PlayerID: 10, GameID: 1, JoinCode: "WOLF-1", NotifyCadenceHours: &cadence,
			SubscriptionID: &sub2, Endpoint: &endpoint2, P256dh: &p256dh, Auth: &auth},
		{PlayerID: 20, GameID: 1, JoinCode: "WOLF-1", NotifyCadenceHours: nil,
			SubscriptionID: nil, Endpoint: nil, P256dh: nil, Auth: nil},
	}

	groups := groupDueNotifications(rows)
	require.Len(t, groups, 2)

	assert.Equal(t, int64(10), groups[0].playerID)
	require.Len(t, groups[0].subs, 2)
	assert.Equal(t, endpoint1, groups[0].subs[0].endpoint)
	assert.Equal(t, endpoint2, groups[0].subs[1].endpoint)
	require.NotNil(t, groups[0].cadenceHours)
	assert.Equal(t, int16(24), *groups[0].cadenceHours)

	assert.Equal(t, int64(20), groups[1].playerID)
	assert.Empty(t, groups[1].subs)
	assert.Nil(t, groups[1].cadenceHours)
}
