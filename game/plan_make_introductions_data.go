package game

// plan_make_introductions_data.go — typed resolution_data for Make Introductions.

import dbgen "uneasy/db/gen"

// MakeIntroductionsResolutionData holds Make Introductions plan state stored
// inside the plans.resolution_data JSON column, nested under the
// "make_introductions" key.
type MakeIntroductionsResolutionData struct {
	// PeerCount is the number of peers being introduced (1–4). Set at
	// preparation time; drives the difficulty (2 + peer_count).
	PeerCount int16 `json:"peer_count,omitempty"`
	// CreatedPeerIDs records the asset IDs of peers named during the
	// pre-roll naming step (one entry per /create-peer call). The dice
	// roll cannot be created until len(CreatedPeerIDs) == PeerCount.
	CreatedPeerIDs []int64 `json:"created_peer_ids,omitempty"`
	// DelayedPeerPlanIDs records synthetic per-peer arrival plans spawned on
	// future rows after the parent plan resolves.
	DelayedPeerPlanIDs []int64 `json:"delayed_peer_plan_ids,omitempty"`

	// MarPending is set when the roll resolved as mar; the focus player must
	// then resolve every introduced peer (one of the four per-peer outcomes)
	// before the plan can complete.
	MarPending bool `json:"mar_pending,omitempty"`
	// MarOutcomes records the per-peer mar resolution. One entry per resolved
	// peer; completion is gated until every created peer has a Done entry.
	MarOutcomes []MIMarOutcome `json:"mar_outcomes,omitempty"`

	// ── Fields below are only set on synthetic delayed-arrival child plans ──

	// DelayedArrival flags this plan row as a synthetic per-peer arrival
	// (no roll; introduces a single peer asset on its row).
	DelayedArrival bool `json:"delayed_arrival,omitempty"`
	// DelayedPeerAssetID is the peer asset being introduced by this
	// delayed-arrival plan.
	DelayedPeerAssetID *int64 `json:"delayed_peer_asset_id,omitempty"`
	// OriginalPlanID is the parent MI plan whose roll spawned this synthetic
	// arrival.
	OriginalPlanID *int64 `json:"original_plan_id,omitempty"`
}

// MIMarOutcome is one introduced peer's per-peer mar resolution.
//
// Outcome is one of:
//   - "other_retinue"  → the peer joins another player's retinue (transfer).
//   - "broken_arrival" → another player (AuthorPlayerID) writes a marginalia;
//     Done flips once they've written it.
//   - "delayed"        → arrival rescheduled d6 rows ahead (synthetic plan).
//   - "broken_journey" → the focus player writes a marginalia then breaks the
//     peer.
type MIMarOutcome struct {
	PeerAssetID int64  `json:"peer_asset_id"`
	Outcome     string `json:"outcome"`
	// AuthorPlayerID is the other player assigned to write the marginalia for
	// a "broken_arrival" outcome (nil for the other outcomes).
	AuthorPlayerID *int64 `json:"author_player_id,omitempty"`
	// Done marks the outcome fully applied. "broken_arrival" stays false until
	// the assigned author writes the marginalia; the others complete inline.
	Done bool `json:"done"`
}

// LoadMakeIntroductionsData is a read-only convenience parser; returns a zero
// struct when the nested key is absent.
func LoadMakeIntroductionsData(plan *dbgen.Plan) MakeIntroductionsResolutionData {
	rd := LoadResolutionData(plan.ResolutionData)
	if rd.MakeIntroductions == nil {
		return MakeIntroductionsResolutionData{}
	}
	return *rd.MakeIntroductions
}

// EnsureMakeIntroductions returns r.MakeIntroductions, allocating a zero
// struct if it was nil.
func (r *ResolutionData) EnsureMakeIntroductions() *MakeIntroductionsResolutionData {
	if r.MakeIntroductions == nil {
		r.MakeIntroductions = &MakeIntroductionsResolutionData{}
	}
	return r.MakeIntroductions
}
