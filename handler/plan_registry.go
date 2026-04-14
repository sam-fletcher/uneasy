package handler

// handler/plan_registry.go — PlanHandler interface, registry, and shared types.
//
// Each plan type registers itself via RegisterPlan (called from init() in its
// own file). The common lifecycle handlers (PreparePlan, ResolvePlan, etc.)
// dispatch through this registry instead of using switch statements.

import (
	"context"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// PlanHandler is implemented by each plan type.
type PlanHandler interface {
	// Metadata returns the plan's category and base delay.
	// Delay is -1 for variable-delay plans (Make War, Clandestinely Liaise,
	// Make Demands), meaning the handler computes the target row itself.
	Metadata() PlanMetadata

	// ValidatePreparation checks plan-specific requirements beyond the
	// common checks (game phase, eligibility, peer ownership, row bounds).
	// Returns a user-facing error message, or "" if valid.
	// For variable-delay plans, also returns the computed target row;
	// fixed-delay plans return 0 (target row is computed from Metadata.Delay).
	ValidatePreparation(ctx context.Context, v *ValidationContext) (targetRow int16, errMsg string)

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

// PlanMetadata holds static plan properties.
type PlanMetadata struct {
	Category model.RankingCategory
	Delay    int16 // Fixed delay (≥1), or -1 for variable
}

// PlanDeps bundles shared dependencies passed to handler methods.
type PlanDeps struct {
	Q       *dbgen.Queries
	Manager *hub.Manager
}

// ValidationContext holds everything the validation step needs.
type ValidationContext struct {
	Q              *dbgen.Queries
	Game           *dbgen.Game
	Player         *dbgen.Player
	TargetPlayerID *int64
	TargetAssetID  *int64
	TargetPlanID   *int64 // Make Demands only
	PeerCount      int16  // Make Introductions only
	Notes          string
}

// ResolutionData holds plan-specific state stored as JSON in plans.resolution_data.
// It is a superset of the old planResData type, extended with fields for all
// 12 plan types. Only the fields relevant to a given plan type will be set.
type ResolutionData struct {
	// ── Exchange Courtiers ──
	FairTradeAssetID   *int64 `json:"fair_trade_asset_id,omitempty"`
	FairTradeAccepted  *bool  `json:"fair_trade_accepted,omitempty"`
	MessyBreakRequired bool   `json:"messy_break_required,omitempty"`
	MessyBreakDone     bool   `json:"messy_break_done,omitempty"`

	// ── Make Introductions ──
	PeerCount          int16   `json:"peer_count,omitempty"`
	DelayedPeerPlanIDs []int64 `json:"delayed_peer_plan_ids,omitempty"`

	// ── Spread Propaganda ──
	RecursivePlanID *int64 `json:"recursive_plan_id,omitempty"`
	EsteemLockout   bool   `json:"esteem_lockout,omitempty"`

	// ── Seek Answers / generic choices ──
	Choices []string `json:"choices,omitempty"`

	// ── Spread Rumors ──
	SourceHidden bool   `json:"source_hidden,omitempty"`
	RumorID      *int64 `json:"rumor_id,omitempty"`

	// ── Chronicle Histories ──
	InvokedArtifactIDs []int64 `json:"invoked_artifact_ids,omitempty"`

	// ── Propose Decree ──
	SignatoryPlayerIDs []int64 `json:"signatory_player_ids,omitempty"`
	SignatoryID        *int64  `json:"signatory_id,omitempty"`
	Addendum           string  `json:"addendum,omitempty"`
	LawID              *int64  `json:"law_id,omitempty"`

	// ── Clandestinely Liaise ──
	PartnerID       *int64 `json:"partner_id,omitempty"`
	LiaisePhase     string `json:"liaise_phase,omitempty"`
	RedelayRevealID *int64 `json:"redelay_reveal_id,omitempty"`

	// ── Propose Duel ──
	DuelType           string `json:"duel_type,omitempty"`
	PreparerChampionID *int64 `json:"preparer_champion_id,omitempty"`
	TargetChampionID   *int64 `json:"target_champion_id,omitempty"`
	CurrentBout        int16  `json:"current_bout,omitempty"`
	InitiativePlayerID *int64 `json:"initiative_player_id,omitempty"`

	// ── Host Festivity ──
	GuestPlayerIDs   []int64           `json:"guest_player_ids,omitempty"`
	GuestOutcomes    map[string]string `json:"guest_outcomes,omitempty"`
	HostGuestChoices map[string]string `json:"host_guest_choices,omitempty"`

	// ── Make War ──
	WarID         *int64 `json:"war_id,omitempty"`
	DelayRevealID *int64 `json:"delay_reveal_id,omitempty"`

	// ── Make Demands ──
	DraftChoices        []DraftChoice `json:"draft_choices,omitempty"`
	CounterDemandPlaced bool          `json:"counter_demand_placed,omitempty"`
}

// DraftChoice records a player's draft pick in Make Demands.
type DraftChoice struct {
	PlayerID int64  `json:"player_id"`
	Option   string `json:"option"`
}

// ── Registry ─────────────────────────────────────────────────────────────────

// registry maps plan types to their handlers.
// Populated by init() functions in each plan_*.go file.
//
//nolint:gochecknoglobals // Registry must be package-level; populated at init time.
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
