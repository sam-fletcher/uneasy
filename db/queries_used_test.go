package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var sqlcQueryName = regexp.MustCompile(`(?m)^-- name: (\w+) :`)

// TestEverySQLCQueryIsCalled fails when a query in queries/*.sql has no Go
// caller anywhere in the module outside the generated package.
//
// Nothing else catches these. golangci-lint's `unused` skips generated files,
// and `make deadcode` is blind to them even with -generated: PoolBacked
// reflects over *dbgen.Queries, and RTA then treats every exported method on
// it as reachable. A Sept 2026 audit found 22 orphans that way — 14 of them
// had never had a caller at all. As with deadcode's -test, a test counts as a
// caller: an integration test that asserts through a query is a real use.
//
// "Called" means a call expression whose selector is the query name
// (`q.GetX(...)`), so a mention in a comment or a string can't keep a dead
// query alive. Method values are not counted — nothing in the module passes a
// Queries method uncalled, and a regexp over source would count comments.
func TestEverySQLCQueryIsCalled(t *testing.T) {
	sqlFiles, err := filepath.Glob("queries/*.sql")
	if err != nil {
		t.Fatalf("glob queries: %v", err)
	}
	if len(sqlFiles) == 0 {
		t.Fatal("no queries/*.sql found — is the test running from db/?")
	}

	definedIn := map[string]string{}
	for _, path := range sqlFiles {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, m := range sqlcQueryName.FindAllStringSubmatch(string(src), -1) {
			if prev, dup := definedIn[m[1]]; dup {
				t.Errorf("query %s defined in both %s and %s", m[1], prev, path)
			}
			definedIn[m[1]] = filepath.Base(path)
		}
	}

	called := map[string]bool{}
	fset := token.NewFileSet()
	scanned := 0
	root := ".."
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			switch {
			case rel == filepath.Join("db", "gen"), // the generated package itself
				d.Name() == "frontend", d.Name() == "node_modules",
				rel != "." && strings.HasPrefix(d.Name(), "."):
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		scanned++
		// Build tags are ignored on purpose: integration-tagged tests are
		// callers too.
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				called[sel.Sel.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	if scanned == 0 {
		t.Fatal("no .go files scanned — is the test running from db/?")
	}

	var dead []string
	for name := range definedIn {
		if !called[name] {
			dead = append(dead, name)
		}
	}
	sort.Strings(dead)
	for _, name := range dead {
		t.Errorf("sqlc query %s (queries/%s) has no caller outside db/gen — "+
			"delete its block and run `make sqlc`", name, definedIn[name])
	}
}
