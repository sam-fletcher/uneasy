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

// PlanDeps bundles shared dependencies passed to handler methods. The
// embedded *db.Store exposes Q and Pool directly (deps.Q, deps.Pool) and
// provides deps.InTx for atomic multi-write sequences.
type PlanDeps struct {
	*db.Store

	Manager *hub.Manager
}

// ValidationContext holds everything the validation step needs.
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
