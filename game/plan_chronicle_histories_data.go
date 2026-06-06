package game

// plan_chronicle_histories_data.go — typed resolution_data for Chronicle Histories.

// ChronicleHistoriesResolutionData holds Chronicle Histories plan state stored
// inside the plans.resolution_data JSON column, nested under the
// "chronicle_histories" key.
type ChronicleHistoriesResolutionData struct {
	// InvokedArtifactIDs records each artifact invoked during the pre-roll
	// phase. Invocation difficulty is max(preparer knowledge rank, count).
	InvokedArtifactIDs []int64 `json:"invoked_artifact_ids,omitempty"`
	// InvokePhaseClosed flips true the moment OnResolve creates the dice
	// roll. After that, no further invocations may change difficulty. The
	// mar-choice "invoke_another" route may still append to
	// InvokedArtifactIDs for narrative tracking even when this flag is set.
	InvokePhaseClosed bool `json:"invoke_phase_closed,omitempty"`

	// MarActive flips true the first time a mar-choice is submitted. It marks
	// the plan as a mar resolution so CanComplete (which has no roll access)
	// can enforce the "all players choose one option" gate.
	MarActive bool `json:"mar_active,omitempty"`

	// MarRequiredChoices is the number of distinct players who must each submit
	// a mar choice before the plan can complete — the count of players in the
	// game at the time the mar scene began. Captured from the mar-choice route
	// (which has DB access) so CanComplete can gate without a query.
	MarRequiredChoices int16 `json:"mar_required_choices,omitempty"`
}

// EnsureChronicleHistories returns r.ChronicleHistories, allocating a zero
// struct if it was nil.
func (r *ResolutionData) EnsureChronicleHistories() *ChronicleHistoriesResolutionData {
	if r.ChronicleHistories == nil {
		r.ChronicleHistories = &ChronicleHistoriesResolutionData{}
	}
	return r.ChronicleHistories
}
