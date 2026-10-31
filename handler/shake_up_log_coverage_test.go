package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// The Shake-Up / focus-pass counterpart of TestSilentEffectsAreLoggedByTheirCallers
// (log_coverage_test.go). That guard watches the asset primitives; the Shake-Up's
// board changes are not asset primitives — they are state-machine writes (the
// phase, the step, the focus marker, a spend's life-cycle, a pass) that
// BeginShakeUp, ShakeUpPass and PassFocus performed and broadcast with no post,
// because nothing in the shape of the code made the omission visible.
//
// This test fails when a function in the Shake-Up or turn files calls one of those
// writes and never reaches a log emitter. Like its sibling it is a co-occurrence
// check — it catches the FORGOTTEN post, not a post on the wrong branch or one
// that omits the public part; those stay with shake_up_log_integration_test.go.

// shakeUpStateWrites are the queries that move Shake-Up or focus state.
var shakeUpStateWrites = map[string]bool{
	"SetFocusPlayer":          true,
	"SetShakeUpStep":          true,
	"SetGamePhase":            true,
	"RefreshAllAssets":        true,
	"CreateShakeUpSpend":      true,
	"CreateShakeUpAdjustment": true,
	"CreateShakeUpPass":       true,
	"CommitShakeUpSpend":      true,
	"AbandonShakeUpSpend":     true,
}

// shakeUpLogAllowed lists the functions that may make such a write without
// reaching an emitter themselves, each with the reason. Keys are
// "<file>:<function>".
var shakeUpLogAllowed = map[string]string{
	"shake_up_pay_abandon.go:shakeUpCommitAndApply": "applyShakeUpEffect ends every applier in EmitShakeUpCommitted",
}

// shakeUpLogFile reports whether a handler file is in the sweep: the Shake-Up
// handlers and the turn file (focus passing and row advance).
func shakeUpLogFile(name string) bool {
	return strings.HasPrefix(name, "shake_up") || name == "turn.go"
}

func TestShakeUpAndFocusWritesAreLogged(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read handler dir: %v", err)
	}

	fset := token.NewFileSet()
	var offenders []string
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			!shakeUpLogFile(name) {
			continue
		}
		scanned++
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var write string
			logs := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var callee string
				switch f := call.Fun.(type) {
				case *ast.Ident:
					callee = f.Name
				case *ast.SelectorExpr:
					callee = f.Sel.Name
				}
				if shakeUpStateWrites[callee] && write == "" {
					write = callee
				}
				if isLogEmitter(callee) {
					logs = true
				}
				return true
			})
			if write == "" || logs {
				continue
			}
			key := name + ":" + fn.Name.Name
			if _, ok := shakeUpLogAllowed[key]; ok {
				continue
			}
			offenders = append(offenders, key+" calls "+write+" but never reaches a log emitter")
		}
	}
	if scanned == 0 {
		t.Fatal("no shake_up*/turn files scanned — the guard is looking in the wrong place")
	}
	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("%s — post the change to the action log (Shake-Up posts take a nil row anchor, "+
			"never game.CurrentRow), or add it to shakeUpLogAllowed with the reason", o)
	}
}
