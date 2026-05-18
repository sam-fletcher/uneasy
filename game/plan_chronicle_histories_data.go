package game

// plan_chronicle_histories_data.go — typed resolution_data for Chronicle Histories.

import dbgen "uneasy/db/gen"

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
}

// LoadChronicleHistoriesData is a read-only convenience parser; returns a
// zero struct when the nested key is absent.
func LoadChronicleHistoriesData(plan *dbgen.Plan) ChronicleHistoriesResolutionData {
	rd := LoadResolutionData(plan.ResolutionData)
	if rd.ChronicleHistories == nil {
		return ChronicleHistoriesResolutionData{}
	}
	return *rd.ChronicleHistories
}

// EnsureChronicleHistories returns r.ChronicleHistories, allocating a zero
// struct if it was nil.
func (r *ResolutionData) EnsureChronicleHistories() *ChronicleHistoriesResolutionData {
	if r.ChronicleHistories == nil {
		r.ChronicleHistories = &ChronicleHistoriesResolutionData{}
	}
	return r.ChronicleHistories
}
