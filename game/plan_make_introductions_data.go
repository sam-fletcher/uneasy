package game

// plan_make_introductions_data.go — typed resolution_data for Make Introductions.

// MakeIntroductionsResolutionData holds Make Introductions plan state stored
// inside the plans.resolution_data JSON column, nested under the
// "make_introductions" key.
type MakeIntroductionsResolutionData struct {
	// PeerCount is the number of peers being introduced (1–4). Set at
	// preparation time; drives the difficulty (2 + peer_count).
	PeerCount int16 `json:"peer_count,omitempty"`
	// Drafts holds the peers named during the pre-roll naming step, in naming
	// order. They are drafts, not assets: naming creates no `assets` row, and
	// nothing exists in any retinue until a peer arrives (D4 — arrival is
	// creation). The dice roll cannot be created until len(Drafts) == PeerCount.
	Drafts []DraftPeer `json:"drafts,omitempty"`
	// Arrivals records the drafts that have materialized into real assets,
	// paired with the asset each became. A draft's presence here is what gates
	// completion on the make path and on a delayed-arrival plan.
	Arrivals []MIArrival `json:"arrivals,omitempty"`
	// DelayedPeerPlanIDs records synthetic per-peer arrival plans spawned on
	// future rows after the parent plan resolves.
	DelayedPeerPlanIDs []int64 `json:"delayed_peer_plan_ids,omitempty"`

	// MakePending is set when the roll made: every named draft still owes its
	// arrival (the rule's "add marginalia to each") before the plan completes.
	MakePending bool `json:"make_pending,omitempty"`
	// MarPending is set when the roll resolved as mar; the preparer must then
	// resolve every introduced peer (one of the four per-peer outcomes) before
	// the plan can complete.
	MarPending bool `json:"mar_pending,omitempty"`
	// MarOutcomes records the per-peer mar resolution. One entry per resolved
	// draft; completion is gated until every named draft has a Done entry.
	MarOutcomes []MIMarOutcome `json:"mar_outcomes,omitempty"`

	// ── Fields below are only set on synthetic delayed-arrival child plans ──

	// DelayedArrival flags this plan row as a synthetic per-peer arrival
	// (no roll; one delayed peer turns up on its row).
	DelayedArrival bool `json:"delayed_arrival,omitempty"`
	// DelayedDraft is the peer riding to this row. They materialize when the
	// plan resolves, which is why a delayed peer cannot be broken, taken or
	// staked while still on the road.
	DelayedDraft *DraftPeer `json:"delayed_draft,omitempty"`
	// OriginalPlanID is the parent MI plan whose roll spawned this synthetic
	// arrival.
	OriginalPlanID *int64 `json:"original_plan_id,omitempty"`
}

// MIArrival pairs a materialized draft with the asset it became.
type MIArrival struct {
	DraftID string `json:"draft_id"`
	AssetID int64  `json:"asset_id"`
}

// MIMarOutcome is one introduced peer's per-peer mar resolution.
//
// Outcome is one of:
//   - "other_retinue"  → the peer joins another player's retinue, and THEY write
//     the marginalia (D6 — the owner describes their own assets); Done flips
//     once they have.
//   - "broken_arrival" → another player (AuthorPlayerID) writes the marginalia;
//     Done flips once they've written it.
//   - "delayed"        → arrival rescheduled d6 rows ahead (synthetic plan), or
//     the draft is dropped entirely when that row runs past the record.
//   - "broken_journey" → the preparer writes both the peer's marginalia and the
//     mark of the journey; the peer arrives with the latter torn.
type MIMarOutcome struct {
	// DraftID names the draft this outcome resolves. Drafts, not asset ids: for
	// three of the four outcomes no asset exists yet when the outcome is chosen.
	DraftID string `json:"draft_id"`
	Outcome string `json:"outcome"`
	// AuthorPlayerID is the player who owes the peer's marginalia before the
	// draft can materialize — the receiving player for "other_retinue", the
	// assigned other player for "broken_arrival". Nil for the outcomes the
	// preparer settles inline.
	AuthorPlayerID *int64 `json:"author_player_id,omitempty"`
	// OwnerPlayerID is the retinue the draft materializes into once its
	// marginalia is written. Nil for outcomes that materialize inline or not
	// at all.
	OwnerPlayerID *int64 `json:"owner_player_id,omitempty"`
	// Done marks the outcome fully applied. "other_retinue" and "broken_arrival"
	// stay false until the assigned player writes the marginalia; the others
	// complete inline.
	Done bool `json:"done"`
}

// EnsureMakeIntroductions returns r.MakeIntroductions, allocating a zero
// struct if it was nil.
func (r *ResolutionData) EnsureMakeIntroductions() *MakeIntroductionsResolutionData {
	if r.MakeIntroductions == nil {
		r.MakeIntroductions = &MakeIntroductionsResolutionData{}
	}
	return r.MakeIntroductions
}

// Draft returns the named draft, or nil. On a synthetic delayed-arrival plan the
// single travelling peer lives in DelayedDraft rather than Drafts, so both are
// consulted — callers can then treat the two plan shapes alike.
func (d *MakeIntroductionsResolutionData) Draft(draftID string) *DraftPeer {
	if found := FindDraftPeer(d.Drafts, draftID); found != nil {
		return found
	}
	if d.DelayedDraft != nil && d.DelayedDraft.ID == draftID {
		return d.DelayedDraft
	}
	return nil
}

// HasArrived reports whether the named draft has already materialized.
func (d *MakeIntroductionsResolutionData) HasArrived(draftID string) bool {
	for _, a := range d.Arrivals {
		if a.DraftID == draftID {
			return true
		}
	}
	return false
}

// MarOutcomeFor returns the recorded mar outcome for a draft, or nil.
func (d *MakeIntroductionsResolutionData) MarOutcomeFor(draftID string) *MIMarOutcome {
	for i := range d.MarOutcomes {
		if d.MarOutcomes[i].DraftID == draftID {
			return &d.MarOutcomes[i]
		}
	}
	return nil
}

// PendingArrivalAuthors names every player who still owes a mar peer's
// marginalia — the receiving player of an "other_retinue" peer and the assigned
// author of a "broken_arrival" one. The table is blocked on them, and they are
// typically not the preparer, so the waiting bar reads from here.
func (d *MakeIntroductionsResolutionData) PendingArrivalAuthors() []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, o := range d.MarOutcomes {
		if o.Done || o.AuthorPlayerID == nil || seen[*o.AuthorPlayerID] {
			continue
		}
		seen[*o.AuthorPlayerID] = true
		out = append(out, *o.AuthorPlayerID)
	}
	return out
}
