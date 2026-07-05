package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)

	h := w.Header()
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", h.Get("X-Frame-Options"))
	assert.Equal(t, "same-origin", h.Get("Referrer-Policy"))
	assert.Contains(t, h.Get("Content-Security-Policy"), "default-src 'self'")
	assert.Contains(t, h.Get("Content-Security-Policy"), "connect-src 'self' wss:")
	assert.NotContains(
		t,
		h.Get("Content-Security-Policy"),
		"localhost:5173",
		"prod (devMode=false) must not allow the Vite HMR socket",
	)
	assert.Empty(t, h.Get("Strict-Transport-Security"), "HSTS is Session 2 (conditional on PUBLIC_ORIGIN)")
}

func TestSecurityHeadersDevModeAllowsViteHMR(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)

	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "ws://localhost:5173")
}
