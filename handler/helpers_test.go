package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTextFieldTrimsAndAccepts(t *testing.T) {
	w := httptest.NewRecorder()
	got, ok := textField(w, "name", "  hello  ", 10)
	assert.True(t, ok)
	assert.Equal(t, "hello", got)
	assert.Equal(t, 200, w.Code, "textField must not write a response on success")
}

func TestTextFieldRejectsOverLimit(t *testing.T) {
	w := httptest.NewRecorder()
	got, ok := textField(w, "name", strings.Repeat("a", 11), 10)
	assert.False(t, ok)
	assert.Empty(t, got)
	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "name")
	assert.Contains(t, w.Body.String(), "10")
}

func TestTextFieldCountsRunesNotBytes(t *testing.T) {
	// "é" below is a single rune but two UTF-8 bytes; a byte-length check
	// would wrongly reject this 5-rune string against a limit of 5.
	w := httptest.NewRecorder()
	got, ok := textField(w, "name", "café!", 5)
	assert.True(t, ok)
	assert.Equal(t, "café!", got)
}

func TestTextFieldAllowsEmpty(t *testing.T) {
	// textField only bounds length; required-ness is each caller's own check.
	w := httptest.NewRecorder()
	got, ok := textField(w, "name", "   ", 5)
	assert.True(t, ok)
	assert.Empty(t, got)
}

func TestTextFieldSliceRejectsFirstOverLimitEntry(t *testing.T) {
	w := httptest.NewRecorder()
	out, ok := textFieldSlice(w, []string{"fine", strings.Repeat("b", 11)}, 10)
	assert.False(t, ok)
	assert.Nil(t, out)
	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "marginalia")
}

func TestTextFieldSliceTrimsEveryEntry(t *testing.T) {
	w := httptest.NewRecorder()
	out, ok := textFieldSlice(w, []string{" a ", " b "}, 10)
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b"}, out)
}
