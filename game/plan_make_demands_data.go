package game

// plan_make_demands_data.go — typed resolution_data for Make Demands.

// MakeDemandsResolutionData holds Make Demands plan state stored inside the
// plans.resolution_data JSON column, nested under the "make_demands" key.
type MakeDemandsResolutionData struct {
	// Outcome is the demand roll's result ("make"/"mar"), recorded by
	// ApplyChoice the moment the dice land (mdHandler opts into
	// AutoApplyChoiceOnRoll). It exists because CanComplete is a PURE
	// function — no ctx, no queries — so it cannot look the roll up, and
	// plans.result is unusable there: SetPlanResult writes it atomically
	// with status='resolved', while CanComplete only ever runs on a plan
	// still in 'resolving'. Gating on plans.result made the demand
	// impossible to complete at all (audit D1).
	Outcome string `json:"outcome,omitempty"`
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
