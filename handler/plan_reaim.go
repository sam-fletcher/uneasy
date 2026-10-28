package handler

// handler/plan_reaim.go — re-aiming a plan whose prepare-time target is gone.
//
// A plan records what it is aimed at when it is prepared — a peer, a rumor's
// subject asset, another player's plan — and acts on it three to six rows later.
// The board does not stand still in between: the peer is broken, the asset is
// taken, the plan demanded against has resolved. Left alone, a plan holding a
// dead reference either acts on it anyway (Exchange Courtiers "transferred" a
// destroyed peer) or fails somewhere deep in its machinery.
//
// The book settles what should happen (p.12): "Targets are declared while
// preparing the plan, but things change. When resolving, players are free to
// change the target of their plan if the original intent no longer makes sense."
// So when the reference is no longer valid, the preparer RE-AIMS — they pick a
// replacement — and the plan carries on from the top of its resolution. Only if
// nothing valid is left to aim at does the plan close with no effect, and that
// is logged: a silent fizzle would read as a bug.
//
// What this does NOT do is let a player re-aim a target that is still valid.
// That is a different, voluntary power which would put a gate on every plan
// (adr/DESIGN_PRINCIPLES.md principle 3); it is an open question for the owner.
//
// Shape, shared by every plan that opts in (TargetReAimer):
//
//	OnResolve ── reaimHoldAtResolve ──┬─ aim still valid ───────────► plan proceeds
//	                                  ├─ aim stale, options exist ──► ReAim.Owed (hold)
//	                                  └─ aim stale, no options ─────► close, no effect
//	POST /plans/{id}/reaim ──► validate against ReAimOptions ──► retarget ──► OnResolve again
//
// The owed state lives in resolution_data and is enforced by ReAimOwedGuard, a
// middleware over every plan route — a client that never renders the picker, or a
// stale tab, cannot act on the plan around it (feedback_subflow_server_authoritative).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ReAimOption is one aim a plan could legally take now. Exactly the fields the
// plan type aims with are set: Exchange Courtiers a player and one of their
// peers, Spread Rumors an asset (the player stays whatever the plan carried),
// Make Demands a plan. The same struct is the wire shape of both the options
// listing and the preparer's choice, so choosing is "send one back verbatim".
type ReAimOption struct {
	TargetPlayerID *int64 `json:"target_player_id"`
	TargetAssetID  *int64 `json:"target_asset_id"`
	TargetPlanID   *int64 `json:"target_plan_id"`
}

// sameAim reports whether two options aim at the same thing.
func (o ReAimOption) sameAim(p ReAimOption) bool {
	return sameID(o.TargetPlayerID, p.TargetPlayerID) &&
		sameID(o.TargetAssetID, p.TargetAssetID) &&
		sameID(o.TargetPlanID, p.TargetPlanID)
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// AimLoss says what changed about a plan's target. The zero value means the aim
// still stands. Plain is for the panel and resolution_data; Log is the same
// sentence with log markup (assetMark / playerDisplayName) for the action log.
type AimLoss struct {
	Plain string
	Log   string
}

func (l AimLoss) lost() bool { return l.Plain != "" }

// TargetReAimer is the optional PlanHandler capability for plans that hold a
// prepare-time reference across the delay. Implementing it is all a plan needs
// to do besides calling reaimHoldAtResolve first thing in OnResolve.
//
// All three methods are read-only; the shared code owns the retarget write, the
// resolution_data bookkeeping, the logging and the broadcast.
type TargetReAimer interface {
	// StaleAim reports whether the reference this plan was prepared against
	// still stands. Validity is per plan, from the rules text: it must be
	// answerable from live rows alone (never from anything cached at prepare).
	StaleAim(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (AimLoss, error)

	// ReAimOptions enumerates every aim the plan could take right now. It is the
	// single definition of "valid" for the picker AND the server: a submitted
	// choice is accepted iff it matches one of these.
	ReAimOptions(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) ([]ReAimOption, error)

	// DescribeAim renders an option for the action log ("…at **Name**").
	DescribeAim(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, opt ReAimOption) string
}

// reaimLog posts a re-aim action-log entry on the plan's row, under the plan's
// own system code (plan.exchange_courtiers, …) like every other line the plan
// writes, so it threads into the same resolution span in the chat.
func reaimLog(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan *dbgen.Plan, body string) {
	var row *int16
	if game, err := q.GetGameByID(ctx, plan.GameID); err == nil {
		row = logRow(game)
	}
	planID := plan.ID
	EmitSystemPost(ctx, q, manager, plan.GameID, "plan."+string(plan.PlanType),
		model.SeverityImportant, body, row, &planID, nil,
		map[string]any{"plan_id": plan.ID})
}

// reaimHoldAtResolve is the one call a participating plan makes at the top of
// OnResolve. It reports held=true when the plan must not start resolving:
// either it is already paused for a re-aim, it has just been paused, or it has
// just been closed because there is nothing left to aim at. held=false means the
// aim stands and OnResolve should carry on.
//
// It is idempotent: the re-aim route calls OnResolve a second time once the
// preparer has chosen, and by then the aim stands, so it falls straight through.
func reaimHoldAtResolve(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, h TargetReAimer,
) (held bool, err error) {
	resData := loadResolutionData(plan.ResolutionData)
	if resData.ReAim.ReAimOwed() {
		return true, nil
	}
	loss, err := h.StaleAim(ctx, deps.Q, plan)
	if err != nil {
		return false, fmt.Errorf("check %s aim: %w", plan.PlanType, err)
	}
	if !loss.lost() {
		return false, nil
	}
	options, err := h.ReAimOptions(ctx, deps.Q, plan)
	if err != nil {
		return false, fmt.Errorf("list %s re-aim options: %w", plan.PlanType, err)
	}

	label := planLabel(plan.PlanType)
	preparer := playerDisplayName(ctx, deps.Q, plan.PreparerID)

	if len(options) == 0 {
		// Nothing valid left to aim at. Close with no effect — and say so, with the
		// cause, in the record. The plan.resolved line that follows carries only the
		// bare outcome.
		reaimLog(ctx, deps.Q, deps.Manager, plan, fmt.Sprintf(
			"%s's %s has nothing left to act on: %s There is nothing else it could be aimed at, "+
				"so it comes to nothing.", preparer, label, loss.Log))
		// The plans.result column only admits make/mar, and this is neither: no roll
		// was made. Close it resolved with a NULL result — the honest record of "this
		// happened and had no outcome" — and tell the log and the clients the same.
		if err := deps.Q.SetPlanResult(ctx, dbgen.SetPlanResultParams{ID: plan.ID, Result: nil}); err != nil {
			return false, fmt.Errorf("close fizzled plan: %w", err)
		}
		refreshed, err := deps.Q.GetPlanByID(ctx, plan.ID)
		if err != nil {
			return false, fmt.Errorf("reload fizzled plan: %w", err)
		}
		*plan = refreshed
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanResolved, model.PlanResolvedPayload{
			PlanID: plan.ID,
			Result: "", // matches the NULL column, so a live client and a reload agree
		})
		EmitPlanResolved(ctx, deps.Q, deps.Manager, refreshed, fizzledOutcome)
		return true, nil
	}

	resData.EnsureReAim().Owed = true
	resData.ReAim.Reason = loss.Plain
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return false, fmt.Errorf("save re-aim state: %w", err)
	}
	refreshed, err := deps.Q.GetPlanByID(ctx, plan.ID)
	if err != nil {
		return false, fmt.Errorf("reload plan: %w", err)
	}
	*plan = refreshed
	reaimLog(ctx, deps.Q, deps.Manager, plan, fmt.Sprintf(
		"%s's %s has lost its mark: %s %s must choose a new target before it can go any further.",
		preparer, label, loss.Log, preparer))
	broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied,
		model.PlanChoiceAppliedPayload{PlanID: plan.ID})
	return true, nil
}

// fizzledOutcome labels a plan that closed with no effect because its target is
// gone and nothing else can be aimed at. It exists only for the log (the plan row
// keeps a NULL result — see reaimHoldAtResolve): EmitPlanResolved gives it the
// generic plan.resolved code, which the chat already files under "Resolved".
const fizzledOutcome = "fizzled"

// reaimWaitees is the waiting-on for a plan paused for a re-aim: the preparer.
//
// It answers INSTEAD of the plan's own ResolvingWaitees rather than merging with
// it, and that is deliberate. A paused plan's own cursor still describes the step
// it WILL run once resumed — Exchange Courtiers' fair-trade phase names the target
// as owing an offer — but ReAimOwedGuard refuses that offer until the preparer has
// chosen. Naming the target here would send them a "your turn" for a step the
// server turns away. Nothing else can be genuinely outstanding while the plan is
// paused, so there is no real wait to lose; the preparer is the whole acting set.
// ok is false when no re-aim is owed, leaving the plan's own answer to stand.
func reaimWaitees(plan *dbgen.Plan) (model.RowState, bool) {
	if !loadResolutionData(plan.ResolutionData).ReAim.ReAimOwed() {
		return model.RowState{}, false
	}
	return model.RowState{
		Kind:            model.RowStateAwaitPlanReAim,
		ActingPlayerIDs: []int64{plan.PreparerID},
	}, true
}

// ── Routes ───────────────────────────────────────────────────────────────────

// ReAimOwedGuard refuses every mutating request on a plan that is paused for a
// re-aim, except the re-aim itself. It is the backend half of the pause: the
// panel hides the plan's controls, but a stale tab or a hand-rolled client must
// not be able to offer a fair trade, request a take, or complete a plan that has
// no valid target.
//
// Mounted once on the /plans/{planId} route group, so a plan that opts in needs
// no per-route check and a plan added later cannot forget one.
func ReAimOwedGuard(s *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || strings.HasSuffix(r.URL.Path, "/reaim") {
				next.ServeHTTP(w, r)
				return
			}
			planID, err := strconv.ParseInt(chi.URLParam(r, "planId"), 10, 64)
			if err != nil {
				next.ServeHTTP(w, r) // the route's own handler reports the bad id
				return
			}
			plan, err := s.Q.GetPlanByID(r.Context(), planID)
			if err == nil && plan.Status == model.PlanResolving &&
				loadResolutionData(plan.ResolutionData).ReAim.ReAimOwed() {
				respondErr(w, http.StatusConflict,
					"this plan's target is gone — its preparer must choose a new one first")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireReAimPlan loads the plan, checks the caller belongs to its game, and
// returns the plan's re-aimer. Writes the error response itself when it can't.
func requireReAimPlan(
	w http.ResponseWriter, r *http.Request, q *dbgen.Queries,
) (*dbgen.Plan, *dbgen.Player, TargetReAimer, bool) {
	plan, player, ok := requirePlanAccess(w, r, q)
	if !ok {
		return nil, nil, nil, false
	}
	h, _ := GetHandler(plan.PlanType)
	re, ok := h.(TargetReAimer)
	if !ok {
		respondErr(w, http.StatusBadRequest, "this plan type has no re-aim step")
		return nil, nil, nil, false
	}
	return plan, player, re, true
}

// GetReAim handles GET /api/plans/{planId}/reaim: whether a re-aim is owed, why,
// and every aim the preparer may choose. Readable by anyone at the table — the
// options are public board state — so a bystander's panel can say what the
// preparer is choosing between.
func GetReAim(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, _, re, ok := requireReAimPlan(w, r, s.Q)
		if !ok {
			return
		}
		ra := loadResolutionData(plan.ResolutionData).ReAim
		out := map[string]any{"owed": ra.ReAimOwed(), "reason": "", "options": []ReAimOption{}}
		if ra.ReAimOwed() && plan.Status == model.PlanResolving {
			options, err := re.ReAimOptions(r.Context(), s.Q, plan)
			if err != nil {
				respondInternalErr(w, r, "could not list re-aim options", err)
				return
			}
			out["reason"] = ra.Reason
			out["options"] = options
		}
		respond(w, http.StatusOK, out)
	}
}

// ReAim handles POST /api/plans/{planId}/reaim with a ReAimOption body.
//
// Only the plan's preparer may re-aim, and only while a re-aim is owed. The
// choice must match one of the plan's current ReAimOptions — the server never
// trusts the picker's idea of "valid". On success the plan is retargeted, the
// owed state cleared, the change logged, and the plan's OnResolve re-run so it
// carries on exactly as if it had just been kicked off (a roll is created, the
// fair-trade step opens, …).
func ReAim(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, re, ok := requireReAimPlan(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the plan's preparer can choose its new target")
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		if !resData.ReAim.ReAimOwed() {
			respondErr(w, http.StatusConflict, "this plan's target still stands — nothing to re-aim")
			return
		}

		var choice ReAimOption
		if err := json.NewDecoder(r.Body).Decode(&choice); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		options, err := re.ReAimOptions(ctx, s.Q, plan)
		if err != nil {
			respondInternalErr(w, r, "could not list re-aim options", err)
			return
		}
		var picked *ReAimOption
		for i := range options {
			if options[i].sameAim(choice) {
				picked = &options[i]
				break
			}
		}
		if picked == nil {
			respondErr(w, http.StatusBadRequest, "that is not a valid new target for this plan")
			return
		}

		deps := &PlanDeps{Store: s, Manager: manager}
		if err := applyReAim(ctx, deps, plan, re, *picked, &resData); err != nil {
			respondInternalErr(w, r, "could not re-aim the plan", err)
			return
		}

		// Carry on from the top. The aim stands now, so this falls through
		// reaimHoldAtResolve and does whatever kickoff would have done.
		h, _ := GetHandler(plan.PlanType)
		roll, err := h.OnResolve(ctx, deps, plan)
		if err != nil {
			// The target is already changed and the pause lifted; put the pause back
			// so the preparer can retry rather than strand a plan with no roll.
			if restoreErr := restoreReAimOwed(ctx, s.Q, plan); restoreErr != nil {
				loggerFromContext(ctx).ErrorContext(ctx, "re-aim: could not restore pause",
					"plan_id", plan.ID, "err", restoreErr)
			}
			respondInternalErr(w, r, "could not resume the plan after re-aiming", err)
			return
		}
		broadcastEvent(manager, plan.GameID, model.EventPlanChoiceApplied,
			model.PlanChoiceAppliedPayload{PlanID: plan.ID})
		broadcastRowState(ctx, s.Q, manager, plan.GameID)

		resp := map[string]any{"plan_id": plan.ID}
		if roll != nil {
			resp["roll"] = roll
		}
		respond(w, http.StatusOK, resp)
	}
}

// applyReAim writes the chosen aim onto the plan, clears the owed state, and logs
// the change. plan is refreshed in place so the caller's OnResolve sees the new
// aim.
func applyReAim(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, re TargetReAimer,
	picked ReAimOption, resData *ResolutionData,
) error {
	before := *plan
	if picked.TargetPlanID != nil {
		if err := deps.Q.SetPlanTargetedPlan(ctx, dbgen.SetPlanTargetedPlanParams{
			ID: plan.ID, TargetedPlanID: picked.TargetPlanID,
		}); err != nil {
			return fmt.Errorf("set targeted plan: %w", err)
		}
	} else if err := deps.Q.SetPlanTargets(ctx, dbgen.SetPlanTargetsParams{
		ID: plan.ID, TargetPlayerID: picked.TargetPlayerID, TargetAssetID: picked.TargetAssetID,
	}); err != nil {
		return fmt.Errorf("set plan targets: %w", err)
	}
	resData.ReAim = &gamepkg.ReAimState{} // paused no more; the reason has served
	if err := saveResolutionData(ctx, deps.Q, plan.ID, *resData); err != nil {
		return fmt.Errorf("clear re-aim state: %w", err)
	}
	refreshed, err := deps.Q.GetPlanByID(ctx, plan.ID)
	if err != nil {
		return fmt.Errorf("reload plan: %w", err)
	}
	*plan = refreshed

	reaimLog(ctx, deps.Q, deps.Manager, plan, fmt.Sprintf(
		"%s re-aimed their %s %s.",
		playerDisplayName(ctx, deps.Q, before.PreparerID), planLabel(plan.PlanType),
		re.DescribeAim(ctx, deps.Q, plan, picked)))
	return nil
}

// restoreReAimOwed re-raises the pause after a failed resume (see ReAim).
func restoreReAimOwed(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) error {
	resData := loadResolutionData(plan.ResolutionData)
	resData.EnsureReAim().Owed = true
	if resData.ReAim.Reason == "" {
		resData.ReAim.Reason = "The plan could not resume after its target was changed."
	}
	return saveResolutionData(ctx, q, plan.ID, resData)
}
