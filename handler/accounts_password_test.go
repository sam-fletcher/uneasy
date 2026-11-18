package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"uneasy/db"
)

// The test exercises the bcrypt 72-byte guard without a DB: the check runs
// before CreateAccount touches the store, so a nil-backed store is safe here.
// (The same guard on UpdateMe needs a logged-in account, so it lives in the
// integration suite.) bcrypt.GenerateFromPassword errors on longer inputs —
// without this guard that surfaced as a confusing 500, not a policy on minimum
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
