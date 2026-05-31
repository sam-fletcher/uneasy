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

// ChoiceLimiter is an optional PlanHandler capability that bounds how many
// make/mar options the focus player may submit, enforcing the rules' dice
// math (e.g. "choose options equal to your result", "up to (difficulty −
// result)"). MakeChoice checks for it via a type assertion.
//
// result is the outcome ("make"/"mar"); rollResult is the dice result (the
// count of distinct faces); difficulty is the effective difficulty. Return a
// negative number for "no fixed limit" (per-peer plans, unscoped options).
// The limit is only enforced when rollResult/result are internally consistent
// (make ⇒ result ≥ difficulty), so a handler can assume that invariant.
type ChoiceLimiter interface {
	MaxChoices(result string, rollResult, difficulty int16) int
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
	PreparerPeerID *int64  // Clandestinely Liaise only — preparer's meeting peer
	PartnerPeerID  *int64  // Clandestinely Liaise only — partner's meeting peer
	Notes          string
}

// ResolutionData is the unmarshal target for the plans.resolution_data JSON
// column. It's a discriminator-free umbrella: loading and saving don't need
// to know the plan type. Each plan with non-trivial state owns a nested
// optional struct (defined in plan_<name>_data.go); for any given plan row,
// at most one of those pointers is set. MakeMarChoices is the only field
// shared across plans. Writers obtain a non-nil nested struct via
// r.EnsureX(); readers use r.X (nil-check) or LoadXData(plan).
type ResolutionData struct {
	// ── Make/Mar choices ──
	// Set by the generic POST /api/plans/:id/make-choice endpoint and by
	// per-plan handlers (e.g. Chronicle) that record per-player make/mar
	// entries. Holds make/mar state only — pre-roll sub-state belongs on
	// per-plan typed fields, not here.
	//
	// Entries from the generic endpoint have PlayerID == nil. Per-plan
	// handlers that track which player made each choice set PlayerID.
	MakeMarChoices []Choice `json:"make_mar_choices,omitempty"`

	ExchangeCourtiers  *ExchangeCourtiersResolutionData  `json:"exchange_courtiers,omitempty"`
	MakeIntroductions  *MakeIntroductionsResolutionData  `json:"make_introductions,omitempty"`
	SeekAnswers        *SeekAnswersResolutionData        `json:"seek_answers,omitempty"`
	SpreadPropaganda   *SpreadPropagandaResolutionData   `json:"spread_propaganda,omitempty"`
	SpreadRumors       *SpreadRumorsResolutionData       `json:"spread_rumors,omitempty"`
	ChronicleHistories *ChronicleHistoriesResolutionData `json:"chronicle_histories,omitempty"`
	ProposeDecree      *ProposeDecreeResolutionData      `json:"propose_decree,omitempty"`
	Liaise             *LiaiseResolutionData             `json:"liaise,omitempty"`
	Duel               *DuelResolutionData               `json:"duel,omitempty"`
	Festivity          *FestivityResolutionData          `json:"festivity,omitempty"`
	MakeWar            *MakeWarResolutionData            `json:"make_war,omitempty"`
	MakeDemands        *MakeDemandsResolutionData        `json:"make_demands,omitempty"`
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
