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
	// IsSecret is set when the preparer chose to keep the rumor secret at prep
	// time ("write it on the underside of one of your assets"). The rumor text
	// then lives only in the Secret below — never in preparation_notes — until a
	// Make publishes it. SecretAssetID/SecretID are non-sensitive metadata (the
	// prepared-log post already names the holding asset).
	IsSecret      bool   `json:"is_secret,omitempty"`
	SecretAssetID *int64 `json:"secret_asset_id,omitempty"`
	SecretID      *int64 `json:"secret_id,omitempty"`
}

// EnsureSpreadRumors returns r.SpreadRumors, allocating a zero struct if it
// was nil. Use from write paths.
func (r *ResolutionData) EnsureSpreadRumors() *SpreadRumorsResolutionData {
	if r.SpreadRumors == nil {
		r.SpreadRumors = &SpreadRumorsResolutionData{}
	}
	return r.SpreadRumors
}
