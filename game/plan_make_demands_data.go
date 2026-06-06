package game

// plan_make_demands_data.go — typed resolution_data for Make Demands.

// MakeDemandsResolutionData holds Make Demands plan state stored inside the
// plans.resolution_data JSON column, nested under the "make_demands" key.
type MakeDemandsResolutionData struct {
	// DraftChoices accumulates the four-pick alternating draft of demand
	// options between preparer and target. Length 4 == complete.
	DraftChoices []DraftChoice `json:"draft_choices,omitempty"`
	// CounterDemandPlaced flips true when the target either places a
	// counter-demand or declines to. The origin demand needs this flag
	// before it can be completed.
	CounterDemandPlaced bool `json:"counter_demand_placed,omitempty"`
}

// EnsureMakeDemands returns r.MakeDemands, allocating a zero struct if it was
// nil. Use from write paths.
func (r *ResolutionData) EnsureMakeDemands() *MakeDemandsResolutionData {
	if r.MakeDemands == nil {
		r.MakeDemands = &MakeDemandsResolutionData{}
	}
	return r.MakeDemands
}
