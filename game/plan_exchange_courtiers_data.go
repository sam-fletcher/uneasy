package game

// plan_exchange_courtiers_data.go — typed resolution_data for Exchange Courtiers.

// ExchangeCourtiersResolutionData holds Exchange Courtiers plan state stored
// inside the plans.resolution_data JSON column, nested under the
// "exchange_courtiers" key.
type ExchangeCourtiersResolutionData struct {
	// FairTradeAssetID is the asset the target offers back during the
	// fair-trade pre-roll. Set when the target submits the offer.
	FairTradeAssetID *int64 `json:"fair_trade_asset_id,omitempty"`
	// FairTradeAccepted is set when the preparer either accepts or declines
	// the offered trade. nil = no decision yet.
	FairTradeAccepted *bool `json:"fair_trade_accepted,omitempty"`
	// MessyBreakRequired flips true when the make-choice flow selects the
	// "messy" option; it gates a follow-up break-asset extra route.
	MessyBreakRequired bool `json:"messy_break_required,omitempty"`
	// MessyBreakDone flips true once the break-asset route completes.
	MessyBreakDone bool `json:"messy_break_done,omitempty"`

	// ── Mar fields (target-driven) ────────────────────────────────────────
	// On a mar the *target* player chooses options. "fair_trade" transfers
	// the targeted peer to the preparer inline. "riposte"/"forfeit" each let
	// the target claim one of the preparer's peers, so PeerClaimsRequired is
	// the number of riposte+forfeit options chosen and PeerClaimsDone counts
	// completed claims; completion is gated until they match.
	PeerClaimsRequired int16 `json:"peer_claims_required,omitempty"`
	PeerClaimsDone     int16 `json:"peer_claims_done,omitempty"`
	// RiposteAllowed is set when "riposte" was chosen; it enables the
	// preparer's optional pre-break of one of their peers (riposte-break).
	RiposteAllowed bool `json:"riposte_allowed,omitempty"`
}

// EnsureExchangeCourtiers returns r.ExchangeCourtiers, allocating a zero
// struct if it was nil.
func (r *ResolutionData) EnsureExchangeCourtiers() *ExchangeCourtiersResolutionData {
	if r.ExchangeCourtiers == nil {
		r.ExchangeCourtiers = &ExchangeCourtiersResolutionData{}
	}
	return r.ExchangeCourtiers
}
