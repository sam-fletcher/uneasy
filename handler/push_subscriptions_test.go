package handler

// push_subscriptions_test.go — the logged-out checks for CreatePushSubscription
// and DeletePushSubscription, which run before the handler touches the store,
// so a nil-backed store is safe here. The body checks need a logged-in account
// and live in push_subscriptions_integration_test.go.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"uneasy/db"
)

func TestCreatePushSubscription_RejectsLoggedOut(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"p","auth":"a"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeletePushSubscription_RejectsLoggedOut(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"endpoint":"https://push.example/x"}`
	req := httptest.NewRequest(http.MethodDelete, "/api/push-subscriptions", strings.NewReader(body))
	w := httptest.NewRecorder()

	DeletePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestIsHTTPSURL(t *testing.T) {
	assert.True(t, isHTTPSURL("https://push.example/x"))
	assert.False(t, isHTTPSURL("http://push.example/x"))
	assert.False(t, isHTTPSURL("https://"))
	assert.False(t, isHTTPSURL(""))
}
