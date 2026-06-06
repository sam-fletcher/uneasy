package game

// plan_spread_rumors_data.go — typed resolution_data for Spread Rumors.

// SpreadRumorsResolutionData holds Spread Rumors plan state stored inside the
// plans.resolution_data JSON column, nested under the "spread_rumors" key.
type SpreadRumorsResolutionData struct {
	// SourceHidden is set after a successful "stay anonymous" mar choice; it
	// causes the rumor's authorship to be hidden in the public record.
	SourceHidden bool `json:"source_hidden,omitempty"`
	// RumorID is the rumor row created at resolve time.
	RumorID *int64 `json:"rumor_id,omitempty"`
}

// EnsureSpreadRumors returns r.SpreadRumors, allocating a zero struct if it
// was nil. Use from write paths.
func (r *ResolutionData) EnsureSpreadRumors() *SpreadRumorsResolutionData {
	if r.SpreadRumors == nil {
		r.SpreadRumors = &SpreadRumorsResolutionData{}
	}
	return r.SpreadRumors
}
