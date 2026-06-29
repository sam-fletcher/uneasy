package game

// plan_propose_decree_data.go — typed resolution_data for Propose Decree.

import "slices"

// ProposeDecreeResolutionData holds Propose Decree plan state stored inside
// the plans.resolution_data JSON column, nested under the "propose_decree" key.
type ProposeDecreeResolutionData struct {
	// SignatoryPlayerIDs is the council: the preparer plus every player ranked
	// above them on power (auto-seated at OnResolve, no leverage needed), plus
	// any eligible player who joined by leveraging an asset.
	SignatoryPlayerIDs []int64 `json:"signatory_player_ids,omitempty"`
	// SignatoryID is the highest-power council member — they call the roll and
	// attach the addendum.
	SignatoryID *int64 `json:"signatory_id,omitempty"`

	// DeclinedPlayerIDs are the eligible-to-join players (ranked below the
	// preparer on power, not auto-seated) who explicitly declined to join the
	// council. Joining and declining are the two ways an eligible player records
	// a decision; the signatory cannot call the roll until every eligible player
	// has done one or the other.
	DeclinedPlayerIDs []int64 `json:"declined_player_ids,omitempty"`

	// DebateStarted flips true when the preparer finalizes the decree's text and
	// opens the council debate (the start-debate route). Until then the preparer
	// is still drafting; the signatory cannot call the roll before the debate has
	// been opened. The finalized text is stored in LawText.
	DebateStarted bool `json:"debate_started,omitempty"`

	// Outcome is the roll's result ("make"/"mar"), recorded when the preparer
	// submits make-choice ("pass the decree"). Its presence means the roll is
	// resolved and the law-writing sub-flow (amendments on a mar, then the
	// addendum) has begun — but the law is NOT yet enacted: per the rules the law
	// only goes into effect WITH its addendum, so the law row and (on a make) the
	// resource asset are created later, at set-addendum. See OutcomeApplied.
	Outcome string `json:"outcome,omitempty"`

	// Addendum is the signatory's optional free-text rider. AddendumConnector
	// ("and"/"but") is prepended to it in the final law. AddendumPlaced flips
	// true when the signatory confirms their addendum (even if blank). Placing
	// the addendum is the step that ENACTS the law (creates the law row and, on a
	// make, the resource asset) — the rules put the law into effect with its
	// addendum, so it cannot precede this step.
	Addendum          string `json:"addendum,omitempty"`
	AddendumConnector string `json:"addendum_connector,omitempty"`
	AddendumPlaced    bool   `json:"addendum_placed,omitempty"`

	// Mar amendment chain. On a marred decree the non-preparer council members
	// amend the law body in turn, lowest power first. AmendmentOrder is that
	// ordered player list (computed at enact); AmendedBy records who has taken
	// their turn so far. The plan can't complete until every member in
	// AmendmentOrder has amended.
	AmendmentOrder []int64 `json:"amendment_order,omitempty"`
	AmendedBy      []int64 `json:"amended_by,omitempty"`

	// LawID is the law row, created when the addendum is placed (set-addendum) —
	// the enactment step. Nil until then; its presence means the law is in effect.
	LawID *int64 `json:"law_id,omitempty"`
	// LawText holds the decree's working body throughout resolution: the preparer
	// finalizes it when opening the debate (start-debate), the council rewrites it
	// in place on a mar (amend-decree), and it becomes the enacted law row's text
	// at set-addendum. The resolve panel reads it directly, so it is always the
	// latest body without a separate laws fetch.
	LawText string `json:"law_text,omitempty"`

	// ResourceAssetID records the resource asset created by a made decree. The
	// asset is created already named, in a single transaction, at enactment
	// (enact-law) — the preparer authors the name with the final law text in
	// view — so there is no placeholder and no separate naming step.
	ResourceAssetID *int64 `json:"resource_asset_id,omitempty"`
}

// OutcomeApplied reports whether make-choice has been submitted — i.e. the roll
// resolved and the law-writing sub-flow has begun. The law itself is not yet
// enacted (that happens at set-addendum); use LawID != nil for that.
func (pd *ProposeDecreeResolutionData) OutcomeApplied() bool {
	return pd.Outcome != ""
}

// NextAmender returns the next council member who must amend the law on a mar,
// or 0 if the amendment chain is complete (or empty). Walks AmendmentOrder and
// returns the first player not yet in AmendedBy.
func (pd *ProposeDecreeResolutionData) NextAmender() int64 {
	for _, id := range pd.AmendmentOrder {
		if !slices.Contains(pd.AmendedBy, id) {
			return id
		}
	}
	return 0
}

// EnsureProposeDecree returns r.ProposeDecree, allocating a zero struct if it
// was nil.
func (r *ResolutionData) EnsureProposeDecree() *ProposeDecreeResolutionData {
	if r.ProposeDecree == nil {
		r.ProposeDecree = &ProposeDecreeResolutionData{}
	}
	return r.ProposeDecree
}
