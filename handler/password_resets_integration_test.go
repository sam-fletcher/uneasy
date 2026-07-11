//go:build integration

// handler/password_resets_integration_test.go — Session 2 coverage for
// adr/FEEDBACK_AND_RESET_PLAN.md: POST /api/password-resets, the redemption
// side of the operator-driven password reset. Tokens are inserted directly
// via InsertPasswordResetToken (bypassing cmd/resetlink, which is a thin CLI
// wrapper around the same query) so tests control expiry precisely.

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
)

// passwordResetHarness wires POST /api/password-resets and POST
// /api/sessions (login), both logged-out, against a freshly seeded game.
// tg.Players[0]'s account has password "dev" (see gametest.findOrCreateAccount).
type passwordResetHarness struct {
	t      *testing.T
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	tg     testGame
	router http.Handler
}

func newPasswordResetHarness(t *testing.T) *passwordResetHarness {
	t.Helper()
	pool := openTestDB(t)
	q := dbgen.New(pool)
	tg := newTestGame(t, q, 2)
	store := db.NewStore(pool)

	r := chi.NewRouter()
	r.Post("/api/password-resets", CreatePasswordReset(store))
	r.Post("/api/sessions", CreateSession(store))

	return &passwordResetHarness{t: t, pool: pool, q: q, tg: tg, router: r}
}

func (h *passwordResetHarness) do(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		require.NoError(h.t, err)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

// insertToken inserts a password_reset_tokens row for rawToken, hashing it
// exactly as the handler does (hashResetToken, package-internal), so the
// handler can find it by hash.
func (h *passwordResetHarness) insertToken(accountID int64, rawToken string, expiresAt time.Time) {
	h.t.Helper()
	_, err := h.q.InsertPasswordResetToken(context.Background(), dbgen.InsertPasswordResetTokenParams{
		TokenHash: hashResetToken(rawToken),
		AccountID: accountID,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	require.NoError(h.t, err)
}

func (h *passwordResetHarness) sessionCount(accountID int64) int {
	h.t.Helper()
	var n int
	err := h.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM sessions WHERE account_id = $1", accountID).Scan(&n)
	require.NoError(h.t, err)
	return n
}

// TestPasswordReset_FullCycle proves the whole redemption loop: the old
// password stops working, the new one works, every existing session for the
// account is gone, and the same token cannot be redeemed twice.
func TestPasswordReset_FullCycle(t *testing.T) {
	h := newPasswordResetHarness(t)
	acctID := h.tg.Players[0].AccountID

	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = h.q.CreateSession(context.Background(), dbgen.CreateSessionParams{Token: tok, AccountID: acctID})
	require.NoError(t, err)
	require.Equal(t, 1, h.sessionCount(acctID), "precondition: account has an existing session")

	h.insertToken(acctID, "full-cycle-raw-token", time.Now().Add(time.Hour))

	code, out := h.do(http.MethodPost, "/api/password-resets", map[string]any{
		"token":        "full-cycle-raw-token",
		"new_password": "a-brand-new-password",
	})
	require.Equalf(t, http.StatusOK, code, "reset accepted: %v", out)
	assert.Equal(t, true, out["ok"])
	assert.Zero(t, h.sessionCount(acctID), "redemption must delete every session for the account")

	acct, err := h.q.GetAccountByID(context.Background(), acctID)
	require.NoError(t, err)
	username := acct.Username

	code, out = h.do(http.MethodPost, "/api/sessions", map[string]any{
		"username": username, "password": "dev",
	})
	require.Equalf(t, http.StatusUnauthorized, code, "old password must no longer work: %v", out)

	code, out = h.do(http.MethodPost, "/api/sessions", map[string]any{
		"username": username, "password": "a-brand-new-password",
	})
	require.Equalf(t, http.StatusOK, code, "new password must work: %v", out)

	code, out = h.do(http.MethodPost, "/api/password-resets", map[string]any{
		"token":        "full-cycle-raw-token",
		"new_password": "yet-another-password",
	})
	require.Equalf(t, http.StatusBadRequest, code, "second redemption of the same token rejected: %v", out)
	assert.Equal(t, resetTokenInvalidMsg, out["error"])
}

func TestPasswordReset_ExpiredTokenRejected(t *testing.T) {
	h := newPasswordResetHarness(t)
	acctID := h.tg.Players[0].AccountID
	h.insertToken(acctID, "expired-raw-token", time.Now().Add(-time.Hour))

	code, out := h.do(http.MethodPost, "/api/password-resets", map[string]any{
		"token":        "expired-raw-token",
		"new_password": "does-not-matter",
	})
	require.Equalf(t, http.StatusBadRequest, code, "expired token rejected: %v", out)
	assert.Equal(t, resetTokenInvalidMsg, out["error"])

	acct, err := h.q.GetAccountByID(context.Background(), acctID)
	require.NoError(t, err)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(acct.PasswordHash), []byte("dev")),
		"password must be unchanged after a rejected reset")
}

func TestPasswordReset_UnknownTokenRejected(t *testing.T) {
	h := newPasswordResetHarness(t)

	code, out := h.do(http.MethodPost, "/api/password-resets", map[string]any{
		"token":        "this-token-was-never-issued",
		"new_password": "does-not-matter",
	})
	require.Equalf(t, http.StatusBadRequest, code, "unknown/garbage token rejected: %v", out)
	assert.Equal(t, resetTokenInvalidMsg, out["error"])
}

func TestPasswordReset_RejectsMissingToken(t *testing.T) {
	h := newPasswordResetHarness(t)

	code, out := h.do(http.MethodPost, "/api/password-resets", map[string]any{
		"new_password": "does-not-matter",
	})
	require.Equalf(t, http.StatusBadRequest, code, "missing token rejected: %v", out)
	assert.Equal(t, resetTokenInvalidMsg, out["error"])
}

func TestPasswordReset_RejectsEmptyPassword(t *testing.T) {
	h := newPasswordResetHarness(t)
	acctID := h.tg.Players[0].AccountID
	h.insertToken(acctID, "empty-pw-token", time.Now().Add(time.Hour))

	code, out := h.do(http.MethodPost, "/api/password-resets", map[string]any{
		"token":        "empty-pw-token",
		"new_password": "",
	})
	require.Equalf(t, http.StatusBadRequest, code, "empty password rejected: %v", out)
	assert.Contains(t, out["error"], "password is required")
}

func TestPasswordReset_RejectsOverlongPassword(t *testing.T) {
	h := newPasswordResetHarness(t)
	acctID := h.tg.Players[0].AccountID
	h.insertToken(acctID, "overlong-pw-token", time.Now().Add(time.Hour))

	code, out := h.do(http.MethodPost, "/api/password-resets", map[string]any{
		"token":        "overlong-pw-token",
		"new_password": strings.Repeat("a", maxPasswordBytes+1),
	})
	require.Equalf(t, http.StatusBadRequest, code, "overlong password rejected: %v", out)
	assert.Contains(t, out["error"], "password too long")
}
