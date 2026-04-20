package db

import (
	"encoding/base64"
	"regexp"
	"testing"
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
			if err != nil {
				t.Fatalf("NewCookieToken() returned error: %v", err)
			}

			// Token should not be empty
			if token == "" {
				t.Errorf("NewCookieToken() returned empty string")
			}

			// Token should be valid base64 URL-safe
			decoded, err := base64.URLEncoding.DecodeString(token)
			if err != nil {
				t.Errorf("NewCookieToken() returned invalid base64: %v", err)
			}

			// Decoded should be exactly cookieTokenBytes (32 bytes = 256 bits)
			if len(decoded) != cookieTokenBytes {
				t.Errorf("NewCookieToken() decoded to %d bytes, want %d", len(decoded), cookieTokenBytes)
			}

			// Token should be URL-safe (base64 URL encoding with possible padding)
			if !regexp.MustCompile("^[A-Za-z0-9_-]*=*$").MatchString(token) {
				t.Errorf("NewCookieToken() returned non-URL-safe characters: %s", token)
			}
		})
	}
}

func TestNewCookieToken_Uniqueness(t *testing.T) {
	// Generate multiple tokens and ensure they're all unique
	tokens := make(map[string]bool)
	for i := range 100 {
		token, err := NewCookieToken()
		if err != nil {
			t.Fatalf("NewCookieToken() iteration %d returned error: %v", i, err)
		}

		if tokens[token] {
			t.Errorf("NewCookieToken() generated duplicate token after %d iterations", i)
			return
		}
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
			if err != nil {
				t.Fatalf("GenerateJoinCode() returned error: %v", err)
			}

			// Code should not be empty
			if code == "" {
				t.Errorf("GenerateJoinCode() returned empty string")
			}

			// Code should be exactly joinCodeLength characters
			if len(code) != joinCodeLength {
				t.Errorf("GenerateJoinCode() returned %d characters, want %d", len(code), joinCodeLength)
			}

			// Code should only contain joinCodeAlphabet characters
			for _, ch := range code {
				if !containsChar(joinCodeAlphabet, ch) {
					t.Errorf(
						"GenerateJoinCode() returned invalid character %q, should be from %s",
						ch,
						joinCodeAlphabet,
					)
				}
			}

			// Code should not contain ambiguous characters
			ambiguous := map[rune]bool{
				'0': true, 'O': true, // not in alphabet
				'1': true, 'I': true, 'L': true, // not in alphabet
			}
			for _, ch := range code {
				if ambiguous[ch] {
					t.Errorf("GenerateJoinCode() returned ambiguous character %q", ch)
				}
			}
		})
	}
}

func TestGenerateJoinCode_Uniqueness(t *testing.T) {
	// Generate multiple codes and ensure they're all unique
	codes := make(map[string]bool)
	for i := range 1000 {
		code, err := GenerateJoinCode()
		if err != nil {
			t.Fatalf("GenerateJoinCode() iteration %d returned error: %v", i, err)
		}

		if codes[code] {
			t.Errorf("GenerateJoinCode() generated duplicate code after %d iterations", i)
			return
		}
		codes[code] = true
	}
}

func TestGenerateJoinCode_Alphabet(t *testing.T) {
	// Generate many codes to verify we use the full alphabet
	seenChars := make(map[rune]bool)
	for i := range 10000 {
		code, err := GenerateJoinCode()
		if err != nil {
			t.Fatalf("GenerateJoinCode() iteration %d returned error: %v", i, err)
		}

		for _, ch := range code {
			seenChars[ch] = true
		}

		// If we've seen all alphabet characters, we're done
		if len(seenChars) == len(joinCodeAlphabet) {
			return
		}
	}

	// With 10000 iterations, we should have hit all or nearly all alphabet chars
	if len(seenChars) < len(joinCodeAlphabet)-2 {
		t.Errorf(
			"GenerateJoinCode() only used %d of %d alphabet characters after 10000 iterations",
			len(seenChars),
			len(joinCodeAlphabet),
		)
	}
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
