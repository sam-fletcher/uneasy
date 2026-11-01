package handler

import (
	"os"
	"strings"
	"testing"
)

// A plan that closes with a result must say so the same way everywhere: the
// stored result, the plan.resolved event and the action-log line travel
// together in resolvePlan. A handler that calls SetPlanResult itself can store
// a result and post nothing (or post a different line), which is how the table
// ends up with a plan that silently vanished from the log.
//
// The first legitimate exception adds a line to setPlanResultAllowed, with the
// reason.
var setPlanResultAllowed = map[string]string{
	"plan_resolution.go":         "resolvePlan itself",
	"plan_exchange_courtiers.go": "stores the result before logging its two asset transfers, then calls announcePlanResolved",
}

func TestPlansResolveThroughResolvePlan(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read handler dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := setPlanResultAllowed[name]; ok {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(src), ".SetPlanResult(") {
			t.Errorf(
				"%s calls SetPlanResult directly; close the plan with resolvePlan so the event and the log line go with it",
				name,
			)
		}
	}
}
