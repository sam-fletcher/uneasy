//go:build integration

// handler/push_subscriptions_integration_test.go — the body checks for
// CreatePushSubscription and DeletePushSubscription. Both handlers answer a
// logged-in account's request before they check the body, so these run through
// a real session (serveAs) and need the database. The logged-out checks stay in
// push_subscriptions_test.go.

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
)

// pushStore opens the test database and returns its queries and store.
func pushStore(t *testing.T) (*dbgen.Queries, *db.Store) {
	t.Helper()
	pool := openTestDB(t)
	return dbgen.New(pool), db.NewStore(pool)
}

// loggedInPush sends one request to a push-subscription handler as a freshly
// created account, so the body checks run after the login gate, as in production.
func loggedInPush(t *testing.T, q *dbgen.Queries, h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	acct, err := q.CreateAccount(t.Context(), dbgen.CreateAccountParams{
		Username: "push-" + randSuffix(), PasswordHash: "x",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(method, "/api/push-subscriptions", strings.NewReader(body))
	return serveAs(t, q, acct.ID, h, req)
}

func TestCreatePushSubscription_RejectsInvalidJSON(t *testing.T) {
	q, store := pushStore(t)
	w := loggedInPush(t, q, CreatePushSubscription(store), http.MethodPost, "{not json")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreatePushSubscription_RejectsNonHTTPSEndpoint(t *testing.T) {
	q, store := pushStore(t)
	body := `{"endpoint":"http://push.example/x","keys":{"p256dh":"p","auth":"a"}}`
	w := loggedInPush(t, q, CreatePushSubscription(store), http.MethodPost, body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "https")
}

func TestCreatePushSubscription_RejectsOverlongEndpoint(t *testing.T) {
	q, store := pushStore(t)
	overlong := "https://push.example/" + strings.Repeat("a", maxPushEndpointLen)
	body := `{"endpoint":"` + overlong + `","keys":{"p256dh":"p","auth":"a"}}`
	w := loggedInPush(t, q, CreatePushSubscription(store), http.MethodPost, body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "endpoint")
}

func TestCreatePushSubscription_RejectsOverlongKey(t *testing.T) {
	q, store := pushStore(t)
	overlong := strings.Repeat("a", maxPushKeyLen+1)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"` + overlong + `","auth":"a"}}`
	w := loggedInPush(t, q, CreatePushSubscription(store), http.MethodPost, body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "p256dh")
}

func TestCreatePushSubscription_RequiresAuth(t *testing.T) {
	q, store := pushStore(t)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"p","auth":""}}`
	w := loggedInPush(t, q, CreatePushSubscription(store), http.MethodPost, body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "auth")
}

func TestCreatePushSubscription_RequiresP256dh(t *testing.T) {
	q, store := pushStore(t)
	body := `{"endpoint":"https://push.example/x","keys":{"p256dh":"","auth":"a"}}`
	w := loggedInPush(t, q, CreatePushSubscription(store), http.MethodPost, body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "p256dh")
}

func TestDeletePushSubscription_RequiresEndpoint(t *testing.T) {
	q, store := pushStore(t)
	w := loggedInPush(t, q, DeletePushSubscription(store), http.MethodDelete, `{}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
