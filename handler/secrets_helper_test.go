package handler

import (
	"os"
	"strings"
	"testing"
)

// createSecretCallers are the only files besides secrets.go that may store a
// secret without announcing it in the same call: each stores it in a
// transaction together with its own state and must announce after the commit.
var createSecretCallers = map[string]bool{
	"plan_spread_rumors_routes.go":        true, // prepare stash, hide-source
	"plan_clandestinely_liaise_phases.go": true, // keep-secret
}

// Every new Secret must go through secrets.go, so it is broadcast and logged
// the same way wherever it comes from (owner ruling 2026-11-01). A handler
// that calls CreateSecret itself can store a secret and announce nothing — or
// invent its own wording. createSecret (store only) is allowed in the files
// above, which must also call announceSecret.
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
		text := string(src)
		if strings.Contains(text, ".CreateSecret(") {
			t.Errorf(
				"%s calls CreateSecret directly; use writeSecret (secrets.go) so the secret is broadcast and logged",
				name,
			)
		}
		if strings.Contains(text, "createSecret(") && !createSecretCallers[name] {
			t.Errorf(
				"%s calls createSecret, which stores without announcing; use writeSecret, or add the file to createSecretCallers with a reason",
				name,
			)
		}
		if createSecretCallers[name] && !strings.Contains(text, "announceSecret(") {
			t.Errorf("%s stores a secret with createSecret but never calls announceSecret", name)
		}
	}
}
