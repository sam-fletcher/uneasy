package handler

import (
	"context"
	"net/http"
	"sync"

	"uneasy/db"
	dbgen "uneasy/db/gen"
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
	if a.plans == nil {
		a.plans = map[int64]int64{}
	}
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
	if a.rowState == nil {
		a.rowState = map[int64]int64{}
	}
	a.rowState[gameID] = a.seq
}

// planAnnouncerSink is where a finished request's notes go. The real one looks
// the plan up and broadcasts; tests substitute a recorder.
type planAnnouncerSink struct {
	gameOf       func(ctx context.Context, planID int64) (int64, error)
	planUpdated  func(gameID, planID int64)
	sendRowState func(ctx context.Context, gameID int64)
}

// AnnouncePlanChanges wraps the API so every plan change a request made is
// announced to the table when the request ends.
//
// It announces whatever the status: a refetch is idempotent and always reads
// the true state, so announcing a change that was rolled back costs one fetch,
// while skipping a non-transactional save followed by an error would leave the
// table stale. The flush is deferred so a panic after a save still announces.
// The actor's own response is written after the flush (the compress layer
// outside this one holds it), so it waits for the lookups, and other clients
// can hear plan.updated before the actor's HTTP response arrives.
func AnnouncePlanChanges(store *db.Store, manager *hub.Manager) func(http.Handler) http.Handler {
	sink := planAnnouncerSink{
		gameOf: func(ctx context.Context, planID int64) (int64, error) {
			plan, err := store.Q.GetPlanByID(ctx, planID)
			return plan.GameID, err
		},
		planUpdated: func(gameID, planID int64) {
			broadcastEvent(manager, gameID, model.EventPlanUpdated, model.PlanUpdatedPayload{PlanID: planID})
		},
		sendRowState: func(ctx context.Context, gameID int64) {
			broadcastRowState(ctx, store.Q, manager, gameID)
		},
	}
	return announcePlanChanges(sink)
}

func announcePlanChanges(sink planAnnouncerSink) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := &planAnnouncer{}
			ctx := context.WithValue(r.Context(), planAnnouncerKey{}, a)
			// The request may have timed out; the saves it made did not.
			defer a.flush(context.WithoutCancel(ctx), sink)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// maxFlushPasses bounds the announce loop below; a kickoff chain is a few
// plans deep at most.
const maxFlushPasses = 8

// flush announces every noted plan. Sending the row state can itself save
// plans (it kicks off a pending plan's resolution, or advances the finale), and
// those saves note themselves on this same announcer, so it loops until a pass
// finds nothing new. Flushing with an announcer-free ctx instead would drop
// those announcements.
func (a *planAnnouncer) flush(ctx context.Context, sink planAnnouncerSink) {
	announced := map[int64]int64{}
	for range maxFlushPasses {
		a.mu.Lock()
		pending := map[int64]int64{}
		for id, s := range a.plans {
			if s > announced[id] {
				pending[id] = s
			}
		}
		a.mu.Unlock()
		if len(pending) == 0 {
			return
		}
		// Games whose row state still owes a broadcast, announced once each.
		owing := map[int64]bool{}
		for planID, s := range pending {
			announced[planID] = s
			gameID, err := sink.gameOf(ctx, planID)
			if err != nil {
				loggerFromContext(ctx).WarnContext(ctx, "plan announce: could not look up plan",
					"plan_id", planID, "err", err)
				continue
			}
			sink.planUpdated(gameID, planID)
			a.mu.Lock()
			last, seen := a.rowState[gameID]
			a.mu.Unlock()
			if !seen || last < s {
				owing[gameID] = true
			}
		}
		for gameID := range owing {
			sink.sendRowState(ctx, gameID)
		}
	}
}

// The plan-writing queries other than resolution_data. Each wrapper notes the
// plan as changed, so the table hears of it when the request ends; a handler
// must not call the raw query (TestPlanStateSavedThroughOnePath).

func setPlanStatus(ctx context.Context, q *dbgen.Queries, arg dbgen.SetPlanStatusParams) error {
	err := q.SetPlanStatus(ctx, arg)
	if err == nil {
		notePlanChanged(ctx, arg.ID)
	}
	return err
}

func setPlanResult(ctx context.Context, q *dbgen.Queries, arg dbgen.SetPlanResultParams) error {
	err := q.SetPlanResult(ctx, arg)
	if err == nil {
		notePlanChanged(ctx, arg.ID)
	}
	return err
}

func setPlanPreparationNotes(ctx context.Context, q *dbgen.Queries, arg dbgen.SetPlanPreparationNotesParams) error {
	err := q.SetPlanPreparationNotes(ctx, arg)
	if err == nil {
		notePlanChanged(ctx, arg.ID)
	}
	return err
}

func setPlanRowAndOrder(ctx context.Context, q *dbgen.Queries, arg dbgen.SetPlanRowAndOrderParams) error {
	err := q.SetPlanRowAndOrder(ctx, arg)
	if err == nil {
		notePlanChanged(ctx, arg.ID)
	}
	return err
}

func setDemandOptionWinners(ctx context.Context, q *dbgen.Queries, arg dbgen.SetDemandOptionWinnersParams) error {
	err := q.SetDemandOptionWinners(ctx, arg)
	if err == nil {
		notePlanChanged(ctx, arg.ID)
	}
	return err
}

func setPlanTargets(ctx context.Context, q *dbgen.Queries, arg dbgen.SetPlanTargetsParams) error {
	err := q.SetPlanTargets(ctx, arg)
	if err == nil {
		notePlanChanged(ctx, arg.ID)
	}
	return err
}

func setPlanTargetedPlan(ctx context.Context, q *dbgen.Queries, arg dbgen.SetPlanTargetedPlanParams) error {
	err := q.SetPlanTargetedPlan(ctx, arg)
	if err == nil {
		notePlanChanged(ctx, arg.ID)
	}
	return err
}

func setPlanFinaleBonus(ctx context.Context, q *dbgen.Queries, planID int64) error {
	err := q.SetPlanFinaleBonus(ctx, planID)
	if err == nil {
		notePlanChanged(ctx, planID)
	}
	return err
}
