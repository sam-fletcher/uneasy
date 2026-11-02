package handler

import (
	"os"
	"strings"
	"testing"
)

// Every new Secret must go through secrets.go, so it is broadcast and logged
// the same way wherever it comes from (owner ruling 2026-11-01). A handler
// that calls CreateSecret itself can store a secret and announce nothing — or
// invent its own wording.
func TestSecretsAreCreatedThroughTheHelper(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read handler dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "secrets.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(src), ".CreateSecret(") {
			t.Errorf(
				"%s calls CreateSecret directly; use writeSecret (secrets.go) so the secret is broadcast and logged",
				name,
			)
		}
	}
}
