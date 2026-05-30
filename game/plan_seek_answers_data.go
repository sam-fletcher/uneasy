package game

// plan_seek_answers_data.go — typed resolution_data for Seek Answers.

import dbgen "uneasy/db/gen"

// SeekAnswersResolutionData holds Seek Answers plan state stored inside the
// plans.resolution_data JSON column, nested under the "seek_answers" key.
type SeekAnswersResolutionData struct {
	// FlawedResourceIDs records every resource asset flawed during this
	// resolution. Each resource may be flawed at most once — the option is
	// "describe a flaw in any resource asset that has been overlooked until
	// now; break that asset" — so the break-resource route rejects any asset
	// already in this list. Covers both make-list breaks and mar-penalty
	// self-flaws.
	FlawedResourceIDs []int64 `json:"flawed_resource_ids,omitempty"`

	// MarSelfFlawsRequired is the number of the preparer's own resources that
	// must be flawed as the mar penalty. Set once in ApplyChoice on a mar to
	// min(difficulty − result, # of the preparer's eligible own resources at
	// resolution time). 0 on a make. The cap is stable because resources can
	// only gain marginalia mid-resolution, never spawn anew.
	MarSelfFlawsRequired int16 `json:"mar_self_flaws_required,omitempty"`

	// MarSelfFlawsApplied counts mar-penalty self-flaws performed so far (a
	// break-resource call on a preparer-owned resource after a mar). The plan
	// cannot complete until this reaches MarSelfFlawsRequired.
	MarSelfFlawsApplied int16 `json:"mar_self_flaws_applied,omitempty"`
}

// LoadSeekAnswersData is a read-only convenience parser; returns a zero struct
// when the nested key is absent.
func LoadSeekAnswersData(plan *dbgen.Plan) SeekAnswersResolutionData {
	rd := LoadResolutionData(plan.ResolutionData)
	if rd.SeekAnswers == nil {
		return SeekAnswersResolutionData{}
	}
	return *rd.SeekAnswers
}

// EnsureSeekAnswers returns r.SeekAnswers, allocating a zero struct if it was
// nil.
func (r *ResolutionData) EnsureSeekAnswers() *SeekAnswersResolutionData {
	if r.SeekAnswers == nil {
		r.SeekAnswers = &SeekAnswersResolutionData{}
	}
	return r.SeekAnswers
}
