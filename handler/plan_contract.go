package handler

// handler/plan_contract.go — the storage-coupled plan orchestration contract.
//
// These types previously lived in the game package, which forced game/ to
// import db, hub, and net/http. They belong in the imperative shell: the
// PlanHandler interface and its dependencies (PlanDeps, ValidationContext) are
// all about wiring rule resolution to Postgres, the WebSocket hub, and HTTP
// routes. Keeping them here lets game/ hold only pure rules + domain types.
//
// Pure data/metadata (ResolutionData, PlanMetadata, ChoiceLimiter, …) remain in
// game/ and are referenced here via the aliases in plan_registry.go.

import (
	"context"
	"encoding/json"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// PlanMetadata holds static plan properties, returned by PlanHandler.Metadata.
type PlanMetadata struct {
	Category model.RankingCategory
	Delay    int16 // Fixed delay (≥1), or -1 for variable
}

// ChoiceLimiter is an optional PlanHandler capability that bounds how many
// make/mar options the focus player may submit, enforcing the rules' dice
// math (e.g. "choose options equal to your result", "up to (difficulty −
// result)"). The make-choice flow checks for it via a type assertion.
//
// result is the outcome ("make"/"mar"); rollResult is the dice result (the
// count of distinct faces); difficulty is the effective difficulty. Return a
// negative number for "no fixed limit" (per-peer plans, unscoped options).
// The limit is only enforced when rollResult/result are internally consistent
// (make ⇒ result ≥ difficulty), so a handler can assume that invariant.
type ChoiceLimiter interface {
	MaxChoices(result string, rollResult, difficulty int16) int
}

// PlanHandler is implemented by each plan type.
//
// Dependency-passing convention (intentional, two tiers — don't "unify" it):
//
//   - Read/compute methods take a bare *dbgen.Queries (ComputeDifficulty) or a
//     parsed-request bundle holding one (ValidatePreparation's
//     ValidationContext). They are side-effect-free, so they get only a query
//     handle — never the hub Manager. This keeps them composable and
//     transaction-agnostic: the caller passes the base pool *or* a tx-scoped q,
//     and ComputeDifficulty can recurse into another plan's ComputeDifficulty
//     (Make Demands derives its difficulty from its target plan) with the same
//     handle.
//   - Effect methods (OnResolve, ApplyChoice, OnPrepare, ExtraRoutes) take
//     *PlanDeps because they mutate state and broadcast: they need the hub
//     Manager (WebSocket events) and Store.InTx (atomic multi-write), which the
//     compute tier deliberately must not have.
//
// CanComplete takes neither — it is a pure check over an already-loaded plan +
// resolution data.
type PlanHandler interface {
	// Metadata returns the plan's category and base delay.
	// Delay is -1 for variable-delay plans (Make War, Clandestinely Liaise,
	// Make Demands), meaning the handler computes the target row itself.
	Metadata() PlanMetadata

	// ValidatePreparation checks plan-specific requirements beyond the
	// common checks (game phase, eligibility, peer ownership, row bounds).
	// Returns a user-facing error message, or "" if valid.
	//
	// targetRow encodes one of three states:
	//   nil  — "no row yet" (Make War, Clandestinely Liaise: row is decided
	//          by a simultaneous reveal after prep).
	//   *int — the computed target row (Make Demands: derived from the
	//          targeted plan's row).
	// Fixed-delay plans (Metadata.Delay >= 0) return nil here; the common
	// code computes targetRow as game.CurrentRow + meta.Delay.
	ValidatePreparation(ctx context.Context, v *ValidationContext) (targetRow *int16, errMsg string)

	// ComputeDifficulty returns the base difficulty for this plan.
	ComputeDifficulty(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan, resData *ResolutionData) (int16, error)

	// OnResolve is called when the plan transitions to 'resolving'.
	// Most plans create a dice roll here and return it.
	// Plans with custom pre-roll flows (EC fair trade, CL) return nil
	// and handle resolution through their extra routes.
	OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error)

	// ApplyChoice executes server-side mechanical effects after
	// make/mar choices are recorded. Return nil if choices are
	// purely narrative (players use existing asset endpoints).
	ApplyChoice(
		ctx context.Context,
		deps *PlanDeps,
		plan *dbgen.Plan,
		resData *ResolutionData,
		choices []string,
		result string,
	) error

	// CanComplete checks whether the plan is ready to be marked resolved.
	// Return nil if ready, or an error describing what's still needed.
	CanComplete(plan *dbgen.Plan, resData *ResolutionData) error

	// ExtraRoutes returns plan-specific sub-routes mounted at
	// /api/plans/:planId/<key>. Return nil if the plan has no extra routes.
	ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc
}

// ResolvingWaitees is an optional PlanHandler capability that lets a plan own
// the "who is the row waiting on?" computation while it is resolving, instead
// of a central switch in row_state.go reaching into each plan's resolution_data.
//
// ComputeRowState calls it for a plan in 'resolving' status. Return ok=true
// with the narrower RowState (Kind + ActingPlayerIDs populated) to override the
// generic case; the caller fills in PlanID. Return ok=false to ride the generic
// PlanResolving case, which names the plan's preparer — preparer-only sub-steps
// (the common case) need not be enumerated, they just fall through.
//
// Cohesion is the point: a plan's waiting-on logic lives next to its OnResolve /
// CanComplete, so the two are reviewed together. The June 2026 audit's missed
// gaps were all in plans whose waiting-on sat far away in the central switch.
type ResolvingWaitees interface {
	ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool)
}

// Per-player submission-state convention (read before adding a plan sub-flow).
//
// When a plan's resolution waits on one or more players to each submit
// something (a make/mar choice, a kept secret, a peer claim, a counter-vote),
// where that "has player X submitted yet?" bit lives is a contract, not a
// free choice:
//
//   - DEFAULT: store it in the plan's resolution_data (the server-authoritative
//     game-state snapshot), and guard the write with a backend dedupe check —
//     a stale client re-prompted after a refresh must produce one effect, not
//     two (return 409 on the second submit). Examples already in the tree:
//     PeerClaimsDone >= PeerClaimsRequired (Exchange Courtiers), the keep-secret
//     and break/hide done-markers (Liaise, Spread Rumors), MarSelfFlawsApplied
//     (Seek Answers).
//   - Side tables (liaise_choices, duel_staked_assets) are fine for the *bulk*
//     payload of a submission, but a submission MARKER must still land in
//     resolution_data so ComputeRowState / ResolvingWaitees can answer
//     "who still has to act?" without joining every side table.
//
// The rule exists because submission state historically lived in four places
// (resolution_data, side tables, reveal entries, dice rolls), so waiting-on had
// to special-case each — the source of the June 2026 waiting-on bugs. Keep it
// in one place. See also feedback_subflow_server_authoritative.

// OnPreparer is an optional interface for plan handlers that need to run setup
// immediately after the plan row is created in PreparePlan. For example,
// Clandestinely Liaise uses it to create the simultaneous reveal and register
// both participants before the plan is broadcast.
//
// Handlers that do not need post-creation setup omit this interface; the
// PreparePlan handler checks for it via a type assertion.
type OnPreparer interface {
	OnPrepare(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) error
}

// PreparedDescriber is an optional interface for plan handlers that want a
// custom plan.prepared log body instead of the default "<preparer> prepared
// <Label>: <notes>". The returned descriptor is the text after the preparer
// name (e.g. `prepared Spread Rumors: there's a rumor brewing about "Julius"`).
// Return ok=false to fall back to the default. Spread Rumors uses this to name
// the target asset and avoid leaking a kept-secret rumor's text.
type PreparedDescriber interface {
	PreparedDescriptor(
		ctx context.Context,
		q *dbgen.Queries,
		plan dbgen.Plan,
		resData *ResolutionData,
	) (descriptor string, ok bool)
}

// PlanDeps bundles shared dependencies passed to handler methods. The
// embedded *db.Store exposes Q and Pool directly (deps.Q, deps.Pool) and
// provides deps.InTx for atomic multi-write sequences.
type PlanDeps struct {
	*db.Store

	Manager *hub.Manager
}

// ValidationContext is the parsed prepare-plan request, handed to a plan's
// ValidatePreparation. PreparePlan decodes the single (polymorphic)
// /prepare-plan request body once, fills this struct, and passes it to whatever
// plan type is being prepared; each validator reads only the subset of fields
// its rules need (the rest stay nil/zero). It is the request DTO for the
// compute tier — a query handle plus context, no hub Manager (see PlanHandler's
// dependency-passing convention). The optional fields are the union of all
// plans' validation inputs, hence the per-plan annotations below; this is
// inherent to having one prepare endpoint for heterogeneous plans, not a smell
// to flatten.
type ValidationContext struct {
	Q              *dbgen.Queries
	Game           *dbgen.Game
	Player         *dbgen.Player
	TargetPlayerID *int64
	TargetAssetID  *int64
	TargetPlanID   *int64  // Make Demands only
	PeerCount      int16   // Make Introductions only
	EnemyPlayerIDs []int64 // Make War only
	PreparerPeerID *int64  // Clandestinely Liaise only — preparer's meeting peer
	PartnerPeerID  *int64  // Clandestinely Liaise only — partner's meeting peer
	Notes          string
}

// ── Registry ─────────────────────────────────────────────────────────────────

// registry maps plan types to their handlers.
// Populated by init() functions in each plan_*.go file.
var registry = map[model.PlanType]PlanHandler{}

// RegisterPlan adds a handler to the registry. Called from init() in each
// plan_*.go file.
func RegisterPlan(pt model.PlanType, h PlanHandler) {
	if _, exists := registry[pt]; exists {
		panic("duplicate plan handler: " + string(pt))
	}
	registry[pt] = h
}

// GetHandler returns the handler for a plan type, or (nil, false) if not registered.
func GetHandler(pt model.PlanType) (PlanHandler, bool) {
	h, ok := registry[pt]
	return h, ok
}

// AllHandlers returns all registered handlers. Used by the router at startup
// to mount extra routes from each handler's ExtraRoutes() method.
func AllHandlers() map[model.PlanType]PlanHandler {
	return registry
}

// ── Resolution data persistence ──────────────────────────────────────────────

// saveResolutionData marshals d to JSON and persists it to the plan row.
func saveResolutionData(ctx context.Context, q *dbgen.Queries, planID int64, d ResolutionData) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	s := string(b)
	return q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{ID: planID, ResolutionData: &s})
}

// pickedChoiceCount returns how many times option appears in the committed
// make/mar choices — i.e. how many of that post-commit sub-flow step the actor
// owes. Sub-flow handlers (Spread Rumors break/hide-source, Seek Answers
// break/reveal, Chronicle break-artifact) gate against it so a step can't run
// more times than was picked — which a stale client, re-prompted after a
// refresh/remount, would otherwise attempt.
func pickedChoiceCount(resData *ResolutionData, option string) int {
	n := 0
	for _, c := range resData.MakeMarChoices {
		if c.Option == option {
			n++
		}
	}
	return n
}
