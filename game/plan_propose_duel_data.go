package game

// plan_propose_duel_data.go — typed resolution_data for Propose Duel.
//
// Replaces the historical DuelState view-struct + r.DuelState() / r.SetDuelState()
// accessors that lived on the fat ResolutionData. Handlers now read and write
// resData.Duel.* directly (use r.EnsureDuel() to get a non-nil pointer from
// write paths).

// DuelPhase enumerates the phases of a Propose Duel plan.
// Values are stable on-wire strings.
type DuelPhase string

const (
	DuelPhaseSetup   DuelPhase = "setup"
	DuelPhaseStaking DuelPhase = "staking"
	DuelPhaseBouts   DuelPhase = "bouts"
	DuelPhaseRoll    DuelPhase = "roll"
	DuelPhaseDone    DuelPhase = "done"
)

// DuelResolutionData holds all Propose Duel plan state stored inside the
// plans.resolution_data JSON column, nested under the "duel" key.
type DuelResolutionData struct {
	// DuelType is "arms" or "wits", chosen at preparation.
	DuelType string `json:"duel_type,omitempty"`

	// PreparerChampionID / TargetChampionID name the peer asset (if any)
	// elected to fight in stead of the duellist themselves.
	PreparerChampionID *int64 `json:"preparer_champion_id,omitempty"`
	TargetChampionID   *int64 `json:"target_champion_id,omitempty"`

	// *Declared flags flip true when each side submits an elect-champion
	// call (with or without an asset). They're how the UI knows the
	// initiative-holder has moved so the second player's picker can unlock.
	PreparerChampionDeclared bool `json:"preparer_champion_declared,omitempty"`
	TargetChampionDeclared   bool `json:"target_champion_declared,omitempty"`

	// Phase tracks pre-roll progression (setup → staking → bouts → roll → done).
	Phase DuelPhase `json:"phase,omitempty"`

	// PreparerStakeCount / TargetStakeCount are the canonical stake counts
	// once both have submitted; they pin the number of bouts that must run.
	PreparerStakeCount int16 `json:"preparer_stake_count,omitempty"`
	TargetStakeCount   int16 `json:"target_stake_count,omitempty"`

	// CurrentBout counts bouts that have been declared (1-indexed).
	CurrentBout int16 `json:"current_bout,omitempty"`

	// InitiativePlayerID is the side currently expected to act.
	InitiativePlayerID *int64 `json:"initiative_player_id,omitempty"`

	// StakeCounts is the pre-reveal accumulator for stake-reveal
	// submissions. Keyed by player ID. Becomes vestigial once both have
	// submitted and PreparerStakeCount / TargetStakeCount are written.
	StakeCounts map[int64]int16 `json:"stake_counts,omitempty"`
}

// LoadDuelData is a read-only convenience parser; returns a zero struct when
// the nested key is absent.
func LoadDuelData(raw *string) DuelResolutionData {
	rd := LoadResolutionData(raw)
	if rd.Duel == nil {
		return DuelResolutionData{}
	}
	return *rd.Duel
}

// EnsureDuel returns r.Duel, allocating a zero struct if it was nil.
func (r *ResolutionData) EnsureDuel() *DuelResolutionData {
	if r.Duel == nil {
		r.Duel = &DuelResolutionData{}
	}
	return r.Duel
}
