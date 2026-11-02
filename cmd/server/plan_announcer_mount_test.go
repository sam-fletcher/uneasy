package main

import (
	"log/slog"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"uneasy/db"
	"uneasy/hub"
)

// Plan changes reach the table only if the announcing middleware wraps every
// plan route. Walk the real router and require it on each one, so moving the
// plan routes out of the group (or the middleware out of it) fails here.
func TestPlanRoutesRunUnderAnnouncer(t *testing.T) {
	r := setupRouter(slog.Default(), &db.Store{}, hub.NewManager(), true, false, "", "", "", "", "")
	seen := 0
	err := chi.Walk(r, func(method, route string, _ http.Handler, mws ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/plans/") {
			return nil
		}
		seen++
		for _, mw := range mws {
			if strings.Contains(runtime.FuncForPC(reflectPtr(mw)).Name(), "handler.announcePlanChanges") {
				return nil
			}
		}
		t.Errorf("%s %s is not wrapped by handler.AnnouncePlanChanges", method, route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("found no /api/plans routes; the walk is checking nothing")
	}
}

func reflectPtr(f func(http.Handler) http.Handler) uintptr { return reflect.ValueOf(f).Pointer() }
