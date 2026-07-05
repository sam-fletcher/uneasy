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

// Both tests exercise the bcrypt 72-byte guard without a DB: the check runs
// before CreateAccount/UpdateMe touch the store, so a nil-backed store is
// safe here. bcrypt.GenerateFromPassword errors on longer inputs — without
// this guard that surfaced as a confusing 500, not a policy on minimum
// length (there isn't one).

func TestCreateAccountRejectsOverlongPassword(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"username":"newplayer","password":"` + strings.Repeat("a", 73) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader(body))
	w := httptest.NewRecorder()

	CreateAccount(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "password too long")
}

func TestUpdateMeRejectsOverlongPassword(t *testing.T) {
	store := db.NewStore(nil)
	body := `{"password":"` + strings.Repeat("a", 73) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/me", strings.NewReader(body))
	req = req.WithContext(
		appMiddleware.AccountContext(req.Context(), &appMiddleware.Account{ID: 1, Username: "someone"}),
	)
	w := httptest.NewRecorder()

	UpdateMe(store)(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "password too long")
}
