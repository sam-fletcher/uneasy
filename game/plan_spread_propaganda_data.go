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
	// "co-opt". Set on the parent plan. Also doubles as the "already co-opted"
	// guard: co-opt fires its side effect immediately (unlike give_peer/
	// break_self, which defer to a sub-flow), so a duplicate "counter_prop"
	// pick in the same choices list must no-op once this is set, or the
	// preparer would spawn two recursive propaganda plans.
	RecursivePlanID *int64 `json:"recursive_plan_id,omitempty"`
	// EsteemLockout is set by mar option (b) "censured": the preparer's next
	// plan cannot be an esteem plan. Checked by HasEsteemLockout.
	EsteemLockout bool `json:"esteem_lockout,omitempty"`
	// OriginalPlanID is set on a recursive SP plan (the child created by
	// mar option (d) "counter_prop") to tag it as not-eligible-to-co-opt-again.
	// Depth cap is 1.
	OriginalPlanID *int64 `json:"original_plan_id,omitempty"`

	// ArtifactRequired flips true on a made plan ("Create an artifact
	// representing the societal shift"): the preparer must then author the
	// artifact via POST /plans/{planId}/create-artifact, which creates it under
	// the chosen name (no placeholder) and records ArtifactID. It gates
	// completion until ArtifactID is set.
	ArtifactRequired bool `json:"artifact_required,omitempty"`
	// ArtifactID is the asset created — and named — by create-artifact on a
	// made plan. Nil until the preparer authors it.
	ArtifactID *int64 `json:"artifact_id,omitempty"`

	// GivePeerDone / BreakSelfDone count how many of the picked give_peer /
	// break_self sub-flow steps have actually been carried out (a peer handed
	// over, a marginalia broken). The mar is repeatable — the preparer may
	// pick either option more than once — so "how many are owed" comes from
	// pickedChoiceCount(resData, "give_peer"/"break_self") against the
	// committed choices, not a boolean flag. This is the server-authoritative
	// completion signal so the panel doesn't re-prompt after a refresh, and the
	// extra-route handlers reject any step beyond the picked count so a stale
	// client can't write a duplicate. Mirrors Spread Rumors' BreakTargetDone /
	// HideSourceDone.
	GivePeerDone  int `json:"give_peer_done,omitempty"`
	BreakSelfDone int `json:"break_self_done,omitempty"`
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
