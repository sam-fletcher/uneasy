// Package game contains domain types and pure game-rule logic for the Uneasy
// TTRPG. It defines the resolution-data model (the typed view over the
// plans.resolution_data JSON column) and pure plan metadata — concepts that
// are independent of storage and HTTP transport.
//
// The storage-coupled plan orchestration contract (the PlanHandler interface,
// PlanDeps, ValidationContext, the handler registry, plus the contract-only
// PlanMetadata and ChoiceLimiter types) lives in the handler package; keeping
// it there is what lets this package stay free of db/hub imports.
package game

import "encoding/json"

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
