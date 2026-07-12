package handler

// push_subscriptions_test.go — endpoint validation for CreatePushSubscription
// and DeletePushSubscription. These checks all run before the handler
// touches the store, so a nil-backed store is safe here, mirroring
// accounts_length_test.go's approach for CreateAccount/UpdateMe.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"uneasy/db"
	appMiddleware "uneasy/middleware"
)

func withAccount(req *http.Request) *http.Request {
	return req.WithContext(
		appMiddleware.AccountContext(req.Context(), &appMiddleware.Account{ID: 1, Username: "someone"}),
	)
}

func TestCreatePushSubscription_RejectsLoggedOut(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"p","auth":"a"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreatePushSubscription_RejectsInvalidJSON(t *testing.T) {
	store := db.NewStore(nil)
	req := withAccount(httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader("{not json")))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreatePushSubscription_RejectsNonHTTPSEndpoint(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"endpoint":"http://push.example/x","keys":{"p256dh":"p","auth":"a"}}`
	req := withAccount(httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader(body)))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "https")
}

func TestCreatePushSubscription_RejectsOverlongEndpoint(t *testing.T) {
	store := db.NewStore(nil)
	overlong := "https://push.example/" + strings.Repeat("a", maxPushEndpointLen)
	body := `{"endpoint":"` + overlong + `","keys":{"p256dh":"p","auth":"a"}}`
	req := withAccount(httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader(body)))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "endpoint")
}

func TestCreatePushSubscription_RejectsOverlongKey(t *testing.T) {
	store := db.NewStore(nil)
	overlong := strings.Repeat("a", maxPushKeyLen+1)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"` + overlong + `","auth":"a"}}`
	req := withAccount(httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader(body)))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "p256dh")
}

func TestCreatePushSubscription_RequiresAuth(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"p","auth":""}}`
	req := withAccount(httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader(body)))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "auth")
}

func TestCreatePushSubscription_RequiresP256dh(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"","auth":"a"}}`
	req := withAccount(httptest.NewRequest(http.MethodPost, "/api/push-subscriptions", strings.NewReader(body)))
	w := httptest.NewRecorder()

	CreatePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "p256dh")
}

func TestDeletePushSubscription_RejectsLoggedOut(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"endpoint":"https://push.example/x"}`
	req := httptest.NewRequest(http.MethodDelete, "/api/push-subscriptions", strings.NewReader(body))
	w := httptest.NewRecorder()

	DeletePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestDeletePushSubscription_RequiresEndpoint(t *testing.T) {
	store := db.NewStore(nil)
	req := withAccount(httptest.NewRequest(http.MethodDelete, "/api/push-subscriptions", strings.NewReader(`{}`)))
	w := httptest.NewRecorder()

	DeletePushSubscription(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestIsHTTPSURL(t *testing.T) {
	assert.True(t, isHTTPSURL("https://push.example/x"))
	assert.False(t, isHTTPSURL("http://push.example/x"))
	assert.False(t, isHTTPSURL("https://"))
	assert.False(t, isHTTPSURL(""))
}
