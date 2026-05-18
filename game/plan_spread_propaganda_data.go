package game

// plan_spread_propaganda_data.go — typed resolution_data for Spread Propaganda.
//
// SpreadPropagandaResolutionData is attached to ResolutionData as the optional
// `SpreadPropaganda` field. See RESOLUTION_DATA_TYPING_PLAN.md for the design
// rationale.

import dbgen "uneasy/db/gen"

// SpreadPropagandaResolutionData holds all Spread Propaganda plan state stored
// inside the plans.resolution_data JSON column, nested under the
// "spread_propaganda" key.
type SpreadPropagandaResolutionData struct {
	// RecursivePlanID points at the child SP plan spawned by mar option (d)
	// "co-opt". Set on the parent plan.
	RecursivePlanID *int64 `json:"recursive_plan_id,omitempty"`
	// EsteemLockout is set by mar option (b) "censured": the preparer's next
	// plan cannot be an esteem plan. Checked by HasEsteemLockout.
	EsteemLockout bool `json:"esteem_lockout,omitempty"`
	// OriginalPlanID is set on a recursive SP plan (the child created by
	// mar option (d) "co-opt") to tag it as not-eligible-to-co-opt-again.
	// Depth cap is 1.
	OriginalPlanID *int64 `json:"original_plan_id,omitempty"`
}

// LoadSpreadPropagandaData is a read-only convenience that parses a plan's
// resolution_data column and returns the inner SpreadPropagandaResolutionData
// as a value (zero struct when the nested key is absent).
func LoadSpreadPropagandaData(plan *dbgen.Plan) SpreadPropagandaResolutionData {
	rd := LoadResolutionData(plan.ResolutionData)
	if rd.SpreadPropaganda == nil {
		return SpreadPropagandaResolutionData{}
	}
	return *rd.SpreadPropaganda
}

// EnsureSpreadPropaganda returns r.SpreadPropaganda, allocating a zero struct
// if it was nil. Use from write paths so handlers don't need to nil-check
// before mutating.
func (r *ResolutionData) EnsureSpreadPropaganda() *SpreadPropagandaResolutionData {
	if r.SpreadPropaganda == nil {
		r.SpreadPropaganda = &SpreadPropagandaResolutionData{}
	}
	return r.SpreadPropaganda
}
