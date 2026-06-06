package game

// plan_make_war_data.go — typed resolution_data for Make War.
//
// MakeWarResolutionData is attached to ResolutionData as the optional
// `MakeWar` field; it holds Make War's plan-specific state (war pointer,
// delay reveal pointer, declared enemy list). The war itself persists in
// the wars table; this struct only holds the cursor state the plan
// handler needs between rows.

// MakeWarResolutionData holds all Make War plan state stored inside the
// plans.resolution_data JSON column, nested under the "make_war" key.
type MakeWarResolutionData struct {
	// WarID points at the wars row created at preparation.
	WarID *int64 `json:"war_id,omitempty"`

	// DelayRevealID points at the simultaneous reveal that sets the plan's
	// delay (ceil-average of participants' revealed faces).
	DelayRevealID *int64 `json:"delay_reveal_id,omitempty"`

	// EnemyPlayerIDs is the declared enemy list captured at preparation.
	// The authoritative participant list lives on the war_participants
	// table; this is the snapshot used during OnPrepare.
	EnemyPlayerIDs []int64 `json:"enemy_player_ids,omitempty"`
}

// LoadMakeWarData is a read-only convenience that parses a plan's
// resolution_data column and returns the inner MakeWarResolutionData as a
// value (zero struct when the nested key is absent).
func LoadMakeWarData(raw *string) MakeWarResolutionData {
	rd := LoadResolutionData(raw)
	if rd.MakeWar == nil {
		return MakeWarResolutionData{}
	}
	return *rd.MakeWar
}

// EnsureMakeWar returns r.MakeWar, allocating a zero struct if it was nil.
func (r *ResolutionData) EnsureMakeWar() *MakeWarResolutionData {
	if r.MakeWar == nil {
		r.MakeWar = &MakeWarResolutionData{}
	}
	return r.MakeWar
}
