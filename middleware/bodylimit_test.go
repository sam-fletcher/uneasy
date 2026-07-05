package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyLimitRejectsOversizedBody(t *testing.T) {
	oversized := strings.Repeat("a", maxBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(oversized))
	w := httptest.NewRecorder()

	var readErr error
	handler := BodyLimit(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		buf := make([]byte, len(oversized))
		_, readErr = r.Body.Read(buf)
		for readErr == nil {
			_, readErr = r.Body.Read(buf)
		}
	}))
	handler.ServeHTTP(w, req)

	require.Error(t, readErr)
	var maxErr *http.MaxBytesError
	assert.ErrorAs(t, readErr, &maxErr)
}

func TestBodyLimitAllowsBodyUnderCap(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small body"))
	w := httptest.NewRecorder()

	var gotErr error
	handler := BodyLimit(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 64)
		_, err := r.Body.Read(buf)
		if err != nil {
			gotErr = err
		}
	}))
	handler.ServeHTTP(w, req)

	assert.NoError(t, gotErr)
}
