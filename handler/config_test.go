package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
)

// DeleteSession is used here (rather than CreateSession/openSession) because
// it skips the DB entirely when there's no player_token cookie on the
// request — letting this stay a fast unit test with a nil-backed store,
// like accounts_password_test.go does for the same reason.
func TestSecureCookiesFlagAppliesToLogoutCookie(t *testing.T) {
	t.Cleanup(func() { SetSecureCookies(false) }) // restore the dev default for other tests

	store := db.NewStore(nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions", nil)

	SetSecureCookies(false)
	w := httptest.NewRecorder()
	DeleteSession(store)(w, req)
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.False(t, cookies[0].Secure, "Secure must be off when SetSecureCookies(false)")

	SetSecureCookies(true)
	w2 := httptest.NewRecorder()
	DeleteSession(store)(w2, req)
	cookies2 := w2.Result().Cookies()
	require.Len(t, cookies2, 1)
	assert.True(t, cookies2[0].Secure, "Secure must be on when SetSecureCookies(true)")
}
