package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The regression net for rule 3 of adr/LOG_MARKS_PLAN.md — "never bake a mark
// into a body string." The 14 house marks live only in
// frontend/src/lib/components/LogMark.svelte and reach a log row through the
// mark slot; a mark character sitting in a Go body string bypasses the slot,
// the severity colour and aria-hidden (that is how the scales headline
// announced "balance scale Rankings update" to a screen reader, and how the
// crown shipped as a colour emoji). This test is what would have caught those
// before review, and what keeps handler/ clean now that S2 removed them.
//
// The alphabet is the *mark* set, not "every non-ASCII glyph". Two arrows are
// deliberate survivors and are intentionally absent from the forbidden set:
//
//   - up-arrow  (U+2191) is verified present in the shipped Spectral woff2 and
//     is an inline ranking annotation, not a mark (system_posts.go keeps it).
//   - right-arrow (U+2192) renders via fallback in four bodies and was left as
//     flagged in S2; low stakes, never emoji-presentation.
//
// Each rune below is named in words rather than drawn, so this comment can't
// trip the scan it documents — the same habit S2's comments follow.
var forbiddenMarkRunes = map[rune]string{
	'⚑':          "plan flag (U+2691)",
	'⚖':          "law/ranking scales (U+2696)",
	'✎':          "asset/marginalia pencil (U+270E)",
	'❧':          "scene floral heart (U+2767)",
	'§':          "secret section sign (U+00A7)",
	'\U0001F451': "crown emoji (U+1F451)",
	'\U0001F5E3': "speaking-head emoji (U+1F5E3)",
	// The whole die-face block — the roll family cycled through faces during
	// design (three pips down to two), so guard every face, not just the last.
	'⚀': "die face (U+2680)",
	'⚁': "die face (U+2681)",
	'⚂': "die face (U+2682)",
	'⚃': "die face (U+2683)",
	'⚄': "die face (U+2684)",
	'⚅': "die face (U+2685)",
}

// TestNoMarkCharactersInGoStringLiterals walks the handler package's own
// non-test sources and fails if any string literal contains a mark character.
// It parses the AST and inspects only string literals, so comments (which
// deliberately spell out mark names in words) are never scanned, and `\u`
// escapes are resolved before scanning so an escaped mark can't slip through.
func TestNoMarkCharactersInGoStringLiterals(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read handler dir: %v", err)
	}

	fset := token.NewFileSet()
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++

		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			// Resolve escapes (`"⚖"` → the scales rune) so an escaped mark
			// is caught, not just a literal one; fall back to the raw form if a
			// literal ever fails to unquote (it shouldn't, for parsed source).
			value := lit.Value
			if unq, err := strconv.Unquote(lit.Value); err == nil {
				value = unq
			}
			for _, r := range value {
				if desc, bad := forbiddenMarkRunes[r]; bad {
					pos := fset.Position(lit.Pos())
					t.Errorf("%s:%d: string literal contains the %s — marks belong in "+
						"LogMark.svelte's mark slot, never a Go body string (adr/LOG_MARKS_PLAN.md rule 3)",
						name, pos.Line, desc)
				}
			}
			return true
		})
	}

	if scanned == 0 {
		t.Fatal("scanned no handler source files — is the test running in handler/?")
	}
}
