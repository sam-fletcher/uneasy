package game

// plan_spread_propaganda_data.go — typed resolution_data for Spread Propaganda.
//
// SpreadPropagandaResolutionData is attached to ResolutionData as the optional
// `SpreadPropaganda` field. See RESOLUTION_DATA_TYPING_PLAN.md for the design
// rationale.

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
	// mar option (d) "counter_prop") to tag it as not-eligible-to-co-opt-again.
	// Depth cap is 1.
	OriginalPlanID *int64 `json:"original_plan_id,omitempty"`

	// ArtifactID is the asset created by the make step ("Create an artifact
	// representing the societal shift"). Set in ApplyChoice on a make result.
	// It is created with a neutral placeholder name; the preparer then names it
	// via the name-asset route (ArtifactNamed flips true once they do). Naming
	// is optional — it does not gate completion.
	ArtifactID    *int64 `json:"artifact_id,omitempty"`
	ArtifactNamed bool   `json:"artifact_named,omitempty"`

	// GivePeerRequired flips true when mar option (a) "give_peer" is chosen;
	// it gates completion until the preparer hands a peer to another player
	// via POST /plans/{planId}/give-peer. GivePeerDone records completion.
	GivePeerRequired bool `json:"give_peer_required,omitempty"`
	GivePeerDone     bool `json:"give_peer_done,omitempty"`

	// BreakSelfRequired flips true when mar option (c) "break_self" is chosen;
	// it gates completion until the preparer breaks one of their own assets
	// via POST /plans/{planId}/break-self. BreakSelfDone records completion.
	BreakSelfRequired bool `json:"break_self_required,omitempty"`
	BreakSelfDone     bool `json:"break_self_done,omitempty"`
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
