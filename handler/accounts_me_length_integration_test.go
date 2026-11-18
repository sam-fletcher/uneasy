//go:build integration

// handler/accounts_me_length_integration_test.go — the username and password
// caps on PATCH /api/accounts/me. UpdateMe answers only a logged-in account, so
// these run through a real session (serveAs) and need the database. The
// CreateAccount caps stay in the unit suite, which needs no login.

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

func TestUpdateMeRejectsOverlongUsername(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	acct, err := q.CreateAccount(t.Context(), dbgen.CreateAccountParams{
		Username: "lenme-" + randSuffix(), PasswordHash: "x",
	})
	require.NoError(t, err)

	body := `{"username":"` + strings.Repeat("a", maxUsernameLen+1) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/me", strings.NewReader(body))
	w := serveAs(t, q, acct.ID, UpdateMe(store), req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "username")
}

func TestUpdateMeRejectsOverlongPassword(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	acct, err := q.CreateAccount(t.Context(), dbgen.CreateAccountParams{
		Username: "pwlen-" + randSuffix(), PasswordHash: "x",
	})
	require.NoError(t, err)

	body := `{"password":"` + strings.Repeat("a", 73) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/me", strings.NewReader(body))
	w := serveAs(t, q, acct.ID, UpdateMe(store), req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "password too long")
}
