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

// The structural guard for adr/audit/FINDINGS.md pattern 3, "mutations without a
// log post": an effect helper that moves the board and broadcasts a WS event but
// posts nothing, called from a handler that forgets to post for it either.
//
// breakMarginalia / breakAsset / takeAssetEffect (and the raw queries beneath
// them) deliberately leave the narration to the caller, because every plan words
// it differently ("X broke Y", "X damaged their own Y before surrendering it").
// That freedom is the trap: Exchange Courtiers' messy break called the helper and
// wrote nothing, so the table saw a lone, actorless "X destroyed." — or, when the
// tear didn't destroy, nothing at all.
//
// This test fails when a function calls one of those effects and never reaches a
// log emitter itself. It is a co-occurrence check, not a proof: it can't tell a
// post on the success path from one on a branch that doesn't run, so it catches
// the FORGOTTEN post (the observed failure) and leaves the WRONG post to the
// per-plan integration tests in log_completeness_integration_test.go. The first
// person to hit it with a legitimate case adds a line to logCoverageAllowed with
// the reason; that is the intended friction.

// silentEffects are the calls that change the board without writing the log.
var silentEffects = map[string]bool{
	"breakMarginalia": true,
	"breakAsset":      true,
	"takeAssetEffect": true,
	"TearMarginalia":  true,
	"TransferAsset":   true,
	"DestroyAsset":    true,
}

// logCoverageAllowed lists the functions that may call a silent effect without
// logging, each with the reason. Keys are "<file>:<function>".
var logCoverageAllowed = map[string]string{
	"asset_effects.go:breakMarginalia":             "primitive: a thin wrapper over breakAsset",
	"asset_effects.go:breakAsset":                  "primitive: posts asset.destroyed via emitAssetDestruction; the actor line is the caller's",
	"asset_effects.go:takeAssetEffect":             "primitive: transfers and grants secret visibility; the caller words the post",
	"plan_chronicle_histories.go:chApplyMarEffect": "returns the log body to chMarChoiceHandler, which posts it",
}

// isLogEmitter reports whether a call name writes to the action log: the
// canonical emitters, a plan's own xxLog helper, or the narration poster.
func isLogEmitter(name string) bool {
	switch {
	case name == "EmitSystemPost", strings.HasPrefix(name, "Emit"):
		return true
	case name == "postPlanNarration", name == "reaimLog":
		return true
	case strings.HasSuffix(name, "Log") || strings.HasSuffix(name, "LogQ"):
		return true
	}
	return false
}

func TestSilentEffectsAreLoggedByTheirCallers(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read handler dir: %v", err)
	}

	fset := token.NewFileSet()
	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var effect string
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
				if silentEffects[callee] && effect == "" {
					effect = callee
				}
				if isLogEmitter(callee) {
					logs = true
				}
				return true
			})
			if effect == "" || logs {
				continue
			}
			key := name + ":" + fn.Name.Name
			if _, ok := logCoverageAllowed[key]; ok {
				continue
			}
			offenders = append(offenders, key+" calls "+effect+" but never reaches a log emitter")
		}
	}
	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("%s — post the change to the action log (see adr/audit/FINDINGS.md pattern 3), "+
			"or add it to logCoverageAllowed with the reason", o)
	}
}
