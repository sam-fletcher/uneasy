package handler

import (
	"context"
	"maps"
	"net/http"
	"sync"

	"uneasy/db"
	"uneasy/hub"
	"uneasy/model"
)

// Plan state is saved in many places and shown on many screens. Instead of
// every route remembering to broadcast, the save itself leaves a note on the
// request ("plan N changed"), and a middleware announces the notes once the
// handler has returned — after every save, after any transaction has
// committed, and once per plan however many saves the request made.
//
// What is announced:
//   - plan.updated {plan_id}: no plan data at all, so nothing hidden can leak.
//     Each client refetches the plan through the viewer-aware endpoint, which
//     applies the per-viewer masking.
//   - the row state, unless the handler already broadcast it after its last
//     save: a plan's stored state decides who the table is waiting on.
//
// notePlanChanged is also called directly by routes whose change lives outside
// resolution_data but still alters a plan's screen or its acting set (a war
// participant joining, say).

type planAnnouncer struct {
	mu  sync.Mutex
	seq int64
	// plans maps a changed plan to the sequence number of its latest change.
	plans map[int64]int64
	// rowState maps a game to the sequence number at its last row-state
	// broadcast; a plan changed after that still owes one.
	rowState map[int64]int64
}

type planAnnouncerKey struct{}

func announcerFrom(ctx context.Context) *planAnnouncer {
	a, _ := ctx.Value(planAnnouncerKey{}).(*planAnnouncer)
	return a
}

// notePlanChanged records that planID's stored state (or something its screen
// or the acting set derives from) changed in this request. It is a no-op
// outside a request wrapped by AnnouncePlanChanges, such as unit tests.
func notePlanChanged(ctx context.Context, planID int64) {
	a := announcerFrom(ctx)
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	a.plans[planID] = a.seq
}

// noteRowStateBroadcast records that the game's row state went out just now,
// so the flush need not repeat it for plans saved before this point.
func noteRowStateBroadcast(ctx context.Context, gameID int64) {
	a := announcerFrom(ctx)
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rowState[gameID] = a.seq
}

// AnnouncePlanChanges wraps the API so every plan change a request made is
// announced to the table when the request ends.
func AnnouncePlanChanges(store *db.Store, manager *hub.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := &planAnnouncer{plans: map[int64]int64{}, rowState: map[int64]int64{}}
			ctx := context.WithValue(r.Context(), planAnnouncerKey{}, a)
			next.ServeHTTP(w, r.WithContext(ctx))
			// The request may have timed out; the saves it made did not.
			a.flush(context.WithoutCancel(ctx), store, manager)
		})
	}
}

func (a *planAnnouncer) flush(ctx context.Context, store *db.Store, manager *hub.Manager) {
	a.mu.Lock()
	changed := make(map[int64]int64, len(a.plans))
	maps.Copy(changed, a.plans)
	a.mu.Unlock()
	if len(changed) == 0 {
		return
	}
	// Games whose row state still owes a broadcast, announced once each.
	owing := map[int64]bool{}
	for planID, s := range changed {
		plan, err := store.Q.GetPlanByID(ctx, planID)
		if err != nil {
			continue
		}
		broadcastEvent(manager, plan.GameID, model.EventPlanUpdated, model.PlanUpdatedPayload{PlanID: planID})
		a.mu.Lock()
		last, seen := a.rowState[plan.GameID]
		a.mu.Unlock()
		if !seen || last < s {
			owing[plan.GameID] = true
		}
	}
	for gameID := range owing {
		broadcastRowState(ctx, store.Q, manager, gameID)
	}
}
