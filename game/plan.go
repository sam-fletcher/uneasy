// Package game contains domain types and pure game-rule logic for the Uneasy
// TTRPG. It defines the PlanHandler interface, resolution data, and the plan
// registry — all concepts that are independent of HTTP transport.
package game

import (
	"context"
	"encoding/json"
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
	// Fields for synthetic delayed-arrival plans only:
	DelayedArrival     bool   `json:"delayed_arrival,omitempty"`
	DelayedPeerAssetID *int64 `json:"delayed_peer_asset_id,omitempty"`
	OriginalPlanID     *int64 `json:"original_plan_id,omitempty"`

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
	PartnerID           *int64 `json:"partner_id,omitempty"`
	LiaisePhase         string `json:"liaise_phase,omitempty"`
	LiaiseDelayRevealID *int64 `json:"liaise_delay_reveal_id,omitempty"`
	RedelayRevealID     *int64 `json:"redelay_reveal_id,omitempty"`

	// ── Propose Duel ──
	DuelType           string `json:"duel_type,omitempty"`
	PreparerChampionID *int64 `json:"preparer_champion_id,omitempty"`
	TargetChampionID   *int64 `json:"target_champion_id,omitempty"`
	DuelPhase          string `json:"duel_phase,omitempty"`
	PreparerStakeCount int16  `json:"preparer_stake_count,omitempty"`
	TargetStakeCount   int16  `json:"target_stake_count,omitempty"`
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

// ── Resolution data helpers ──────────────────────────────────────────────────

// LoadResolutionData unmarshals the JSON resolution_data column into a
// ResolutionData struct. Returns a zero-value struct if raw is nil or empty.
func LoadResolutionData(raw *string) ResolutionData {
	if raw == nil || *raw == "" {
		return ResolutionData{}
	}
	var d ResolutionData
	_ = json.Unmarshal([]byte(*raw), &d)
	return d
}

// SaveResolutionData marshals d to JSON and persists it to the plan row.
func SaveResolutionData(ctx context.Context, q *dbgen.Queries, planID int64, d ResolutionData) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	s := string(b)
	return q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{ID: planID, ResolutionData: &s})
}

// ── Typed accessors ──────────────────────────────────────────────────────────
//
// Plan handlers work with focused views of ResolutionData instead of the full
// union struct. Per-plan accessors make the intent explicit at call sites.

// DuelState is the Propose Duel subset of ResolutionData.
type DuelState struct {
	DuelType           string // "arms" or "wits"
	PreparerChampionID *int64
	TargetChampionID   *int64
	// Phase tracks pre-roll progression. See the propose-duel handler for
	// the set of valid values ("stake_reveal", "staking", "bouts", "roll",
	// "done").
	Phase              string
	PreparerStakeCount int16
	TargetStakeCount   int16
	CurrentBout        int16
	InitiativePlayerID *int64
}

// DuelState returns the Propose Duel view of r.
func (r *ResolutionData) DuelState() DuelState {
	return DuelState{
		DuelType:           r.DuelType,
		PreparerChampionID: r.PreparerChampionID,
		TargetChampionID:   r.TargetChampionID,
		Phase:              r.DuelPhase,
		PreparerStakeCount: r.PreparerStakeCount,
		TargetStakeCount:   r.TargetStakeCount,
		CurrentBout:        r.CurrentBout,
		InitiativePlayerID: r.InitiativePlayerID,
	}
}

// SetDuelState writes s back into r.
func (r *ResolutionData) SetDuelState(s DuelState) {
	r.DuelType = s.DuelType
	r.PreparerChampionID = s.PreparerChampionID
	r.TargetChampionID = s.TargetChampionID
	r.DuelPhase = s.Phase
	r.PreparerStakeCount = s.PreparerStakeCount
	r.TargetStakeCount = s.TargetStakeCount
	r.CurrentBout = s.CurrentBout
	r.InitiativePlayerID = s.InitiativePlayerID
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
