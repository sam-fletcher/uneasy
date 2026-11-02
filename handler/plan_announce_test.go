package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Plan state reaches the table because the save itself leaves a note that the
// AnnouncePlanChanges middleware turns into plan.updated. A handler that wrote
// resolution_data through the raw query would skip the note and leave every
// other open panel stale until a reload, which is the bug this guard exists
// to stop. The only caller of SetPlanResolutionData is saveResolutionData.
func TestPlanStateSavedThroughOnePath(t *testing.T) {
	var offenders []string
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "gen", ".claude", "frontend":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Base(path) == "plan_contract.go" {
			return nil // saveResolutionData itself
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(src), ".SetPlanResolutionData(") {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	for _, p := range offenders {
		t.Errorf("%s calls SetPlanResolutionData directly; use saveResolutionData so the change is announced", p)
	}
}

// The middleware must be mounted, or the notes are never read.
func TestServerMountsPlanAnnouncer(t *testing.T) {
	src, err := os.ReadFile("../cmd/server/main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(src), "handler.AnnouncePlanChanges(") {
		t.Error("cmd/server/main.go does not mount handler.AnnouncePlanChanges on the API group")
	}
}

// A save after the last row-state broadcast still owes one; a save before it
// does not. Outside a request the notes are a no-op.
func TestPlanAnnouncerRowStateBookkeeping(t *testing.T) {
	notePlanChanged(context.Background(), 1) // no announcer: must not panic

	a := &planAnnouncer{plans: map[int64]int64{}, rowState: map[int64]int64{}}
	ctx := context.WithValue(context.Background(), planAnnouncerKey{}, a)

	notePlanChanged(ctx, 7)
	noteRowStateBroadcast(ctx, 10)
	notePlanChanged(ctx, 8)
	notePlanChanged(ctx, 7)

	if a.rowState[10] >= a.plans[7] {
		t.Errorf("plan 7 changed again after the row-state broadcast; it should still owe one")
	}
	if a.plans[8] <= a.rowState[10] {
		t.Errorf("plan 8 changed after the broadcast; it should owe one")
	}
}
