package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Plan state reaches the table because every save leaves a note that the
// AnnouncePlanChanges middleware turns into plan.updated. A handler that wrote
// a plan column through the raw query would skip the note and leave every
// other open panel stale until a reload, which is the bug this guard exists
// to stop. Raw plan-writing queries may be called only from the wrappers in
// plan_announce.go and saveResolutionData in plan_contract.go.
//
// Exceptions go in planWriteAllowed with the reason.
var planWriteQueries = []string{
	"SetPlanResolutionData", "SetPlanStatus", "SetPlanResult", "SetPlanPreparationNotes",
	"SetPlanRowAndOrder", "SetDemandOptionWinners", "SetPlanTargets", "SetPlanTargetedPlan",
	"SetPlanFinaleBonus", "ShiftRowOrderAtOrAfter",
}

// file -> the queries it may call raw, with the reason.
var planWriteAllowed = map[string]struct {
	queries []string
	reason  string
}{
	"plan_contract.go": {[]string{"SetPlanResolutionData"}, "saveResolutionData itself"},
	"plan_announce.go": {planWriteQueries, "the wrappers that note the change"},
	// Re-ranks neighbours on the record; every caller goes on to create or
	// move a plan, which announces itself.
	"plans.go": {
		[]string{"ShiftRowOrderAtOrAfter"},
		"re-ranks neighbours while a plan is prepared",
	},
	"plan_make_demands_counter.go": {[]string{"ShiftRowOrderAtOrAfter"}, "slots a counter-plan in"},
}

func TestPlanStateSavedThroughOnePath(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read handler dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, rerr := os.ReadFile(name)
		if rerr != nil {
			t.Fatalf("read %s: %v", name, rerr)
		}
		for _, q := range planWriteQueries {
			if !strings.Contains(string(src), "."+q+"(") || slices.Contains(planWriteAllowed[name].queries, q) {
				continue
			}
			t.Errorf("%s calls %s directly; use the announced wrapper (set%s)", name, q, strings.TrimPrefix(q, "Set"))
		}
	}
	// And nothing outside handler/ writes plans at all.
	for _, dir := range []string{"../game", "../cmd", "../middleware", "../hub"} {
		if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, _ := os.ReadFile(path)
			for _, q := range planWriteQueries {
				if strings.Contains(string(src), "."+q+"(") {
					t.Errorf("%s calls %s outside handler/; plan writes belong in the announced wrappers", path, q)
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}

// (That the middleware wraps every plan route is checked against the real
// router in cmd/server/plan_announcer_mount_test.go.)

type announceRecord struct {
	updated  [][2]int64 // gameID, planID
	rowState []int64
}

func recordingSink(rec *announceRecord, gameOf map[int64]int64) planAnnouncerSink {
	return planAnnouncerSink{
		gameOf: func(_ context.Context, planID int64) (int64, error) {
			if g, ok := gameOf[planID]; ok {
				return g, nil
			}
			return 0, errors.New("no such plan")
		},
		planUpdated:  func(g, p int64) { rec.updated = append(rec.updated, [2]int64{g, p}) },
		sendRowState: func(_ context.Context, g int64) { rec.rowState = append(rec.rowState, g) },
	}
}

func serve(sink planAnnouncerSink, h http.HandlerFunc) {
	w := httptest.NewRecorder()
	announcePlanChanges(sink)(h).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
}

// A request that saves one plan five times announces it once, with one row
// state; a plan saved before the handler's own row-state broadcast needs none
// extra; an error status and a panic still announce; no saves, no events.
func TestAnnouncePlanChangesSendsOncePerPlan(t *testing.T) {
	games := map[int64]int64{5: 1, 6: 1, 7: 2}

	t.Run("five saves, one event, one row state", func(t *testing.T) {
		rec := &announceRecord{}
		serve(recordingSink(rec, games), func(w http.ResponseWriter, r *http.Request) {
			for range 5 {
				notePlanChanged(r.Context(), 5)
			}
			notePlanChanged(r.Context(), 6)
		})
		if len(rec.updated) != 2 {
			t.Errorf("plan.updated sent %d times, want once per plan (2): %v", len(rec.updated), rec.updated)
		}
		if len(rec.rowState) != 1 || rec.rowState[0] != 1 {
			t.Errorf("row state sent %v, want once for game 1", rec.rowState)
		}
	})

	t.Run("handler already broadcast row state after its saves", func(t *testing.T) {
		rec := &announceRecord{}
		serve(recordingSink(rec, games), func(w http.ResponseWriter, r *http.Request) {
			notePlanChanged(r.Context(), 7)
			noteRowStateBroadcast(r.Context(), 2)
		})
		if len(rec.updated) != 1 || len(rec.rowState) != 0 {
			t.Errorf("updated=%v rowState=%v, want one event and no repeated row state", rec.updated, rec.rowState)
		}
	})

	t.Run("a save after the row-state broadcast still owes one", func(t *testing.T) {
		rec := &announceRecord{}
		serve(recordingSink(rec, games), func(w http.ResponseWriter, r *http.Request) {
			noteRowStateBroadcast(r.Context(), 2)
			notePlanChanged(r.Context(), 7)
		})
		if len(rec.rowState) != 1 {
			t.Errorf("rowState=%v, want one", rec.rowState)
		}
	})

	t.Run("error status still announces", func(t *testing.T) {
		rec := &announceRecord{}
		serve(recordingSink(rec, games), func(w http.ResponseWriter, r *http.Request) {
			notePlanChanged(r.Context(), 5)
			w.WriteHeader(http.StatusInternalServerError)
		})
		if len(rec.updated) != 1 {
			t.Errorf("updated=%v, want the save announced", rec.updated)
		}
	})

	t.Run("a panic after a save still announces", func(t *testing.T) {
		rec := &announceRecord{}
		func() {
			defer func() { _ = recover() }()
			serve(recordingSink(rec, games), func(w http.ResponseWriter, r *http.Request) {
				notePlanChanged(r.Context(), 5)
				panic("boom")
			})
		}()
		if len(rec.updated) != 1 {
			t.Errorf("updated=%v, want the save announced despite the panic", rec.updated)
		}
	})

	t.Run("a save made while sending the row state is announced too", func(t *testing.T) {
		rec := &announceRecord{}
		sink := recordingSink(rec, games)
		kickedOff := false
		sink.sendRowState = func(ctx context.Context, g int64) {
			rec.rowState = append(rec.rowState, g)
			if !kickedOff { // the kickoff the row state triggers saves plan 7
				kickedOff = true
				notePlanChanged(ctx, 7)
			}
		}
		serve(sink, func(w http.ResponseWriter, r *http.Request) { notePlanChanged(r.Context(), 5) })
		if len(rec.updated) != 2 {
			t.Errorf("updated=%v, want plans 5 and 7 (the save made during the flush)", rec.updated)
		}
	})

	t.Run("no saves, no events", func(t *testing.T) {
		rec := &announceRecord{}
		serve(recordingSink(rec, games), func(w http.ResponseWriter, r *http.Request) {})
		if len(rec.updated)+len(rec.rowState) != 0 {
			t.Errorf("got %v %v, want nothing", rec.updated, rec.rowState)
		}
	})

	t.Run("outside a request the notes are a no-op", func(t *testing.T) {
		notePlanChanged(context.Background(), 1)
		noteRowStateBroadcast(context.Background(), 1)
	})
}
