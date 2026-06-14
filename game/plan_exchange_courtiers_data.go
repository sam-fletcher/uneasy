package game

// plan_exchange_courtiers_data.go — typed resolution_data for Exchange Courtiers.

// ExchangeCourtiersPhase is the explicit resolution cursor for Exchange
// Courtiers — the single "what step are we in / who acts" source, replacing the
// old reconstruct-from-flag-combinations approach (see PLAN_RESOLUTION_TECH_DEBT
// B1). Values are stable on-wire strings.
//
// The cursor is written at each transition by the handler; the make-vs-mar
// branch inside ECPhaseRoll is the one split that stays roll-derived (the dice
// roll resolves in the generic engine, with no per-plan hook to advance a
// cursor at that moment).
type ExchangeCourtiersPhase string

const (
	// ECPhaseFairTrade — the opening fair-trade step: the target offers a peer,
	// then the preparer accepts (plan resolves, make) or declines (→ roll). The
	// zero value maps here (a freshly-resolving plan hasn't written a phase yet).
	ECPhaseFairTrade ExchangeCourtiersPhase = "fair_trade"
	// ECPhaseRoll — the preparer declined; the dice roll is created and the
	// post-roll make/mar choice happens. A made roll's choice is the preparer's;
	// a marred roll's choice is the target's.
	ECPhaseRoll ExchangeCourtiersPhase = "roll"
	// ECPhaseMessyBreak — a made roll chose "messy": the target must break one
	// of the preparer's assets before the plan can complete.
	ECPhaseMessyBreak ExchangeCourtiersPhase = "messy_break"
	// ECPhasePeerClaims — a marred roll chose forfeit/riposte: the target claims
	// the preparer's peer(s). Held until PeerClaimsDone == PeerClaimsRequired.
	ECPhasePeerClaims ExchangeCourtiersPhase = "peer_claims"
	// ECPhaseDone — resolution is mechanically complete; the preparer completes
	// the plan. Rides the generic plan_resolving case.
	ECPhaseDone ExchangeCourtiersPhase = "done"
)

// ExchangeCourtiersResolutionData holds Exchange Courtiers plan state stored
// inside the plans.resolution_data JSON column, nested under the
// "exchange_courtiers" key.
type ExchangeCourtiersResolutionData struct {
	// Phase is the authoritative resolution cursor (see ExchangeCourtiersPhase).
	// Use CurrentPhase() to read it (maps the zero value to ECPhaseFairTrade).
	Phase ExchangeCourtiersPhase `json:"phase,omitempty"`

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

// CurrentPhase returns Phase, mapping the zero value to the opening
// ECPhaseFairTrade step (a freshly-resolving plan hasn't written one yet).
func (e *ExchangeCourtiersResolutionData) CurrentPhase() ExchangeCourtiersPhase {
	if e == nil || e.Phase == "" {
		return ECPhaseFairTrade
	}
	return e.Phase
}

// EnsureExchangeCourtiers returns r.ExchangeCourtiers, allocating a zero
// struct if it was nil.
func (r *ResolutionData) EnsureExchangeCourtiers() *ExchangeCourtiersResolutionData {
	if r.ExchangeCourtiers == nil {
		r.ExchangeCourtiers = &ExchangeCourtiersResolutionData{}
	}
	return r.ExchangeCourtiers
}
