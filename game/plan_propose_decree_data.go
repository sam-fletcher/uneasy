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

	// Addendum is the signatory's optional free-text rider. AddendumConnector
	// ("and"/"but") is prepended to it in the final law. AddendumPlaced flips
	// true when the signatory confirms their addendum (even if blank) — a
	// required blocking step before the plan can complete.
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

	// LawID is the law row created at enact (make-choice) time. Its text/addendum
	// are updated in place by amendments and the addendum step.
	LawID *int64 `json:"law_id,omitempty"`
	// LawText holds the decree's working body. The preparer finalizes it when
	// opening the debate (start-debate), it becomes the enacted law row's text at
	// make-choice, and on a mar it mirrors the row as the council amends it — so
	// the resolve panel can always show the latest text without a separate laws
	// fetch. Kept in sync by pdComposeLaw after enactment.
	LawText string `json:"law_text,omitempty"`

	// ResourceAssetID is the resource asset created by the make step. It is
	// created with a neutral placeholder name; the preparer then names it via
	// the name-asset route (ResourceNamed flips true once they do). Naming is
	// optional — it does not gate completion.
	ResourceAssetID *int64 `json:"resource_asset_id,omitempty"`
	ResourceNamed   bool   `json:"resource_named,omitempty"`
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
