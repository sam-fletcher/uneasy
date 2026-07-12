//go:build integration

// handler/accounts_notify_cadence_integration_test.go — round-trip coverage
// for PATCH /api/accounts/me's notify_cadence_hours field
// (adr/NOTIFICATIONS_PLAN.md Session 3). The interesting case is presence:
// {"notify_cadence_hours": null} must clear the cadence (notifications off),
// distinct from omitting the field (leave it untouched) — see UpdateMe's doc
// comment for why a plain *int16 struct field can't tell those apart alone.

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	appMiddleware "uneasy/middleware"
)

func patchMe(t *testing.T, store *db.Store, acctID int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/accounts/me", strings.NewReader(body))
	req = req.WithContext(appMiddleware.AccountContext(req.Context(), &appMiddleware.Account{ID: acctID}))
	w := httptest.NewRecorder()
	UpdateMe(store)(w, req)
	return w
}

func TestUpdateMe_NotifyCadenceRoundTrip(t *testing.T) {
	pool := openTestDB(t)
	q := dbgen.New(pool)
	store := db.NewStore(pool)
	ctx := context.Background()

	acct, err := q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: "cadence-" + randSuffix(), PasswordHash: "x",
	})
	require.NoError(t, err)
	require.NotNil(t, acct.NotifyCadenceHours, "migration default is 24, not off")
	assert.EqualValues(t, 24, *acct.NotifyCadenceHours)

	// Omitting the field entirely leaves the existing cadence untouched.
	w := patchMe(t, store, acct.ID, `{"username":"`+acct.Username+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, err := q.GetAccountByID(ctx, acct.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.NotifyCadenceHours)
	assert.EqualValues(t, 24, *reloaded.NotifyCadenceHours, "omitted field must not change the cadence")

	// Setting an explicit valid value.
	w = patchMe(t, store, acct.ID, `{"notify_cadence_hours": 3}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, err = q.GetAccountByID(ctx, acct.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.NotifyCadenceHours)
	assert.EqualValues(t, 3, *reloaded.NotifyCadenceHours)

	// An explicit JSON null turns notifications off — presence-aware, not
	// just "field absent".
	w = patchMe(t, store, acct.ID, `{"notify_cadence_hours": null}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, err = q.GetAccountByID(ctx, acct.ID)
	require.NoError(t, err)
	assert.Nil(t, reloaded.NotifyCadenceHours, "explicit null must clear the cadence (notifications off)")

	// An invalid cadence value is rejected and leaves the stored value
	// (still NULL, from the previous step) untouched.
	w = patchMe(t, store, acct.ID, `{"notify_cadence_hours": 5}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	reloaded, err = q.GetAccountByID(ctx, acct.ID)
	require.NoError(t, err)
	assert.Nil(t, reloaded.NotifyCadenceHours, "rejected value must not be written")
}
