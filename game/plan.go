// Package game contains domain types and pure game-rule logic for the Uneasy
// TTRPG. It defines the PlanHandler interface, resolution data, and the plan
// registry — all concepts that are independent of HTTP transport.
package game

import (
	"context"
	"encoding/json"
	"net/http"

	"uneasy/db"
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

// PlanMetadata holds static plan properties.
type PlanMetadata struct {
	Category model.RankingCategory
	Delay    int16 // Fixed delay (≥1), or -1 for variable
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
	Notes          string
}

// ResolutionData holds plan-specific state stored as JSON in plans.resolution_data.
// It is a superset of the old planResData type, extended with fields for all
// 12 plan types. Only the fields relevant to a given plan type will be set.
type ResolutionData struct {
	// ── Exchange Courtiers ──
	// All EC-specific state lives on the nested struct; see
	// plan_exchange_courtiers_data.go.
	ExchangeCourtiers *ExchangeCourtiersResolutionData `json:"exchange_courtiers,omitempty"`

	// ── Make Introductions ──
	// All MI-specific state lives on the nested struct; see
	// plan_make_introductions_data.go.
	MakeIntroductions *MakeIntroductionsResolutionData `json:"make_introductions,omitempty"`

	// ── Spread Propaganda ──
	// All SP-specific state lives on the nested struct; see
	// plan_spread_propaganda_data.go. Per-plan handlers go through
	// r.EnsureSpreadPropaganda() for writes and r.SpreadPropaganda (or
	// LoadSpreadPropagandaData) for reads.
	SpreadPropaganda *SpreadPropagandaResolutionData `json:"spread_propaganda,omitempty"`

	// ── Make/Mar choices ──
	// Set by the generic POST /api/plans/:id/make-choice endpoint and by
	// per-plan handlers (e.g. Chronicle) that record per-player make/mar
	// entries. Holds make/mar state only — pre-roll sub-state belongs on
	// per-plan typed fields, not here.
	//
	// Entries from the generic endpoint have PlayerID == nil. Per-plan
	// handlers that track which player made each choice (Chronicle) set
	// PlayerID.
	MakeMarChoices []Choice `json:"make_mar_choices,omitempty"`

	// ── Spread Rumors ──
	// All SR-specific state lives on the nested struct; see
	// plan_spread_rumors_data.go.
	SpreadRumors *SpreadRumorsResolutionData `json:"spread_rumors,omitempty"`

	// ── Chronicle Histories ──
	// All CH-specific state lives on the nested struct; see
	// plan_chronicle_histories_data.go.
	ChronicleHistories *ChronicleHistoriesResolutionData `json:"chronicle_histories,omitempty"`

	// ── Propose Decree ──
	// All PD-specific state lives on the nested struct; see
	// plan_propose_decree_data.go.
	ProposeDecree *ProposeDecreeResolutionData `json:"propose_decree,omitempty"`

	// ── Clandestinely Liaise ──
	// All Liaise-specific state lives on the nested struct; see
	// plan_liaise_data.go. Per-plan handlers go through r.EnsureLiaise()
	// for writes and r.Liaise (or LoadLiaiseData) for reads.
	Liaise *LiaiseResolutionData `json:"liaise,omitempty"`

	// ── Propose Duel ──
	// All duel-specific state lives on the nested struct; see
	// plan_propose_duel_data.go.
	Duel *DuelResolutionData `json:"duel,omitempty"`

	// ── Host Festivity ──
	FestivityPhase       string            `json:"festivity_phase,omitempty"`
	GuestPlayerIDs       []int64           `json:"guest_player_ids,omitempty"`
	GuestOutcomes        map[string]string `json:"guest_outcomes,omitempty"`
	GuestMakeChoices     map[string]string `json:"guest_make_choices,omitempty"`
	GuestMarChoices      map[string]string `json:"guest_mar_choices,omitempty"`
	HostGuestChoices     map[string]string `json:"host_guest_choices,omitempty"`
	GuestRollIDs         map[string]int64  `json:"guest_roll_ids,omitempty"`
	GuestIOUs            []int64           `json:"guest_ious,omitempty"`
	HostMarInsists       []string          `json:"host_mar_insists,omitempty"`
	AcceptDuelsPlayerIDs []int64           `json:"accept_duels_player_ids,omitempty"`
	PendingDuelPlanID    *int64            `json:"pending_duel_plan_id,omitempty"`
	PendingChallenge     *PendingChallenge `json:"pending_challenge,omitempty"`
	CenteredAssetIDs     []int64           `json:"centered_asset_ids,omitempty"`

	// ── Make War ──
	WarID             *int64  `json:"war_id,omitempty"`
	DelayRevealID     *int64  `json:"delay_reveal_id,omitempty"`
	WarEnemyPlayerIDs []int64 `json:"war_enemy_player_ids,omitempty"`
	WarScenePosted    bool    `json:"war_scene_posted,omitempty"`

	// ── Make Demands ──
	// All MD-specific state lives on the nested struct; see
	// plan_make_demands_data.go.
	MakeDemands *MakeDemandsResolutionData `json:"make_demands,omitempty"`
}

// DraftChoice records a player's draft pick in Make Demands.
type DraftChoice struct {
	PlayerID int64  `json:"player_id"`
	Option   string `json:"option"`
}

// KeptSecret records one player's keep-secret submission in Clandestinely
// Liaise's "Secrets We Keep" phase: the player nominates one of their own
// assets to hold the secret of the meeting.
type KeptSecret struct {
	PlayerID int64 `json:"player_id"`
	AssetID  int64 `json:"asset_id"`
}

// Choice is one entry in ResolutionData.MakeMarChoices.
//
// Entries written by the generic POST /api/plans/:id/make-choice endpoint
// leave PlayerID nil. Per-plan handlers that track per-player make/mar
// (e.g. Chronicle) set PlayerID to the submitting player.
type Choice struct {
	PlayerID *int64 `json:"player_id,omitempty"`
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
