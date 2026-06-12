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

	// PendingTakeConsent is non-nil while a "take asset" choice is waiting for
	// the victim (the player who would lose the asset) to agree or disagree.
	// While set, the make/mar choices are NOT yet committed — the aggressor's
	// picks live inside this struct and are only applied once the victim
	// agrees. ComputeRowState surfaces this as await_take_consent.
	PendingTakeConsent *TakeConsentRequest `json:"pending_take_consent,omitempty"`
	// TakeAssetDenied is set when the victim disagreed. It disables the
	// "take asset" option when the aggressor returns to the option picker, so
	// the resolution can't loop on an asset the owner won't surrender.
	TakeAssetDenied bool `json:"take_asset_denied,omitempty"`
	// TakeResolved is set once the agreed-to transfer(s) have been applied. It
	// tells the panel the take-asset step is complete (the transfer happens at
	// consent time, not in a later sub-flow).
	TakeResolved bool `json:"take_resolved,omitempty"`
}

// TakeConsentRequest captures an open "take asset" consent gate: the
// aggressor's full set of make/mar picks (applied verbatim on agreement), the
// specific assets they want to take, and who must consent. It is stored inside
// SpreadRumorsResolutionData.PendingTakeConsent.
type TakeConsentRequest struct {
	// Choices is the full flat list of picked option keys, applied exactly as
	// MakeChoice would once the victim agrees.
	Choices []string `json:"choices"`
	// Result is the roll outcome these choices belong to ("make" | "mar").
	Result string `json:"result"`
	// AssetIDs are the victim's assets the aggressor wants to take (one per
	// "take_asset" pick). On make these may be any of the victim's assets, not
	// only the rumor's target asset.
	AssetIDs []int64 `json:"asset_ids"`
	// VictimID owns AssetIDs and must agree before anything is committed.
	VictimID int64 `json:"victim_id"`
	// RequestedBy is the aggressor (the preparer on make, the target-asset
	// owner on mar).
	RequestedBy int64 `json:"requested_by"`
}

// EnsureSpreadRumors returns r.SpreadRumors, allocating a zero struct if it
// was nil. Use from write paths.
func (r *ResolutionData) EnsureSpreadRumors() *SpreadRumorsResolutionData {
	if r.SpreadRumors == nil {
		r.SpreadRumors = &SpreadRumorsResolutionData{}
	}
	return r.SpreadRumors
}
