package db

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCookieToken(t *testing.T) {
	cases := []struct {
		name string
	}{
		{"generates token 1"},
		{"generates token 2"},
		{"generates token 3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := NewCookieToken()
			require.NoError(t, err)

			// Token should not be empty
			assert.NotEmpty(t, token)

			// Token should be valid base64 URL-safe
			decoded, err := base64.URLEncoding.DecodeString(token)
			require.NoError(t, err, "returned invalid base64")

			// Decoded should be exactly cookieTokenBytes (32 bytes = 256 bits)
			assert.Len(t, decoded, cookieTokenBytes, "decoded to wrong number of bytes")

			// Token should be URL-safe (base64 URL encoding with possible padding)
			assert.Regexp(t, "^[A-Za-z0-9_-]*=*$", token, "returned non-URL-safe characters")
		})
	}
}

func TestNewCookieToken_Uniqueness(t *testing.T) {
	// Generate multiple tokens and ensure they're all unique
	tokens := make(map[string]bool)
	for i := range 100 {
		token, err := NewCookieToken()
		require.NoError(t, err)

		assert.False(t, tokens[token], "generated duplicate token after %d iterations", i)
		tokens[token] = true
	}
}

func TestGenerateJoinCode(t *testing.T) {
	cases := []struct {
		name string
	}{
		{"generates code 1"},
		{"generates code 2"},
		{"generates code 3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, err := GenerateJoinCode()
			require.NoError(t, err)

			// Code should not be empty
			assert.NotEmpty(t, code)

			// Code should be exactly joinCodeLength characters
			assert.Len(t, code, joinCodeLength)

			// Code should only contain joinCodeAlphabet characters
			for _, ch := range code {
				assert.True(t, containsChar(joinCodeAlphabet, ch), "returned invalid character %q", ch)
			}

			// Code should not contain ambiguous characters
			ambiguous := map[rune]bool{
				'0': true, 'O': true, // not in alphabet
				'1': true, 'I': true, 'L': true, // not in alphabet
			}
			for _, ch := range code {
				assert.False(t, ambiguous[ch], "returned ambiguous character %q", ch)
			}
		})
	}
}

func TestGenerateJoinCode_Uniqueness(t *testing.T) {
	// Generate multiple codes and ensure they're all unique
	codes := make(map[string]bool)
	for i := range 1000 {
		code, err := GenerateJoinCode()
		require.NoError(t, err)

		assert.False(t, codes[code], "generated duplicate code after %d iterations", i)
		codes[code] = true
	}
}

func TestGenerateJoinCode_Alphabet(t *testing.T) {
	// Generate many codes to verify we use the full alphabet
	seenChars := make(map[rune]bool)
	for range 10000 {
		code, err := GenerateJoinCode()
		require.NoError(t, err)

		for _, ch := range code {
			seenChars[ch] = true
		}

		// If we've seen all alphabet characters, we're done
		if len(seenChars) == len(joinCodeAlphabet) {
			return
		}
	}

	// With 10000 iterations, we should have hit all or nearly all alphabet chars
	assert.GreaterOrEqual(t, len(seenChars), len(joinCodeAlphabet)-2,
		"only used %d of %d alphabet characters after 10000 iterations",
		len(seenChars), len(joinCodeAlphabet))
}

// containsChar checks if a rune appears in a string
func containsChar(s string, r rune) bool {
	for _, ch := range s {
		if ch == r {
			return true
		}
	}
	return false
}
