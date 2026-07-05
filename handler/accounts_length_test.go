package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"uneasy/db"
	appMiddleware "uneasy/middleware"
)

// Both tests exercise the username/email length caps without a DB: like the
// password-length guard, the check runs before CreateAccount/UpdateMe touch
// the store, so a nil-backed store is safe here.

func TestCreateAccountRejectsOverlongUsername(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"username":"` + strings.Repeat("a", maxUsernameLen+1) + `","password":"pw"}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreateAccount(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "username")
}

func TestCreateAccountRejectsOverlongEmail(t *testing.T) {
	store := db.NewStore(nil)
	overlong := strings.Repeat("a", maxEmailLen+1)
	body := `{"username":"newplayer","password":"pw","email":"` + overlong + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreateAccount(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "email")
}

func TestUpdateMeRejectsOverlongUsername(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"username":"` + strings.Repeat("a", maxUsernameLen+1) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/me", strings.NewReader(body))
	req = req.WithContext(
		appMiddleware.AccountContext(req.Context(), &appMiddleware.Account{ID: 1, Username: "someone"}),
	)
	w := httptest.NewRecorder()

	UpdateMe(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "username")
}
