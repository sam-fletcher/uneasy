package game

// plan_propose_decree_data.go — typed resolution_data for Propose Decree.

import dbgen "uneasy/db/gen"

// ProposeDecreeResolutionData holds Propose Decree plan state stored inside
// the plans.resolution_data JSON column, nested under the "propose_decree" key.
type ProposeDecreeResolutionData struct {
	// SignatoryPlayerIDs is the council: players who have joined the decree.
	SignatoryPlayerIDs []int64 `json:"signatory_player_ids,omitempty"`
	// SignatoryID is the council member elected by call-roll to sign the law.
	SignatoryID *int64 `json:"signatory_id,omitempty"`
	// Addendum is an optional rider attached by the signatory.
	Addendum string `json:"addendum,omitempty"`
	// LawID is the law row created at resolve time.
	LawID *int64 `json:"law_id,omitempty"`
}

// LoadProposeDecreeData is a read-only convenience parser; returns a zero
// struct when the nested key is absent.
func LoadProposeDecreeData(plan *dbgen.Plan) ProposeDecreeResolutionData {
	rd := LoadResolutionData(plan.ResolutionData)
	if rd.ProposeDecree == nil {
		return ProposeDecreeResolutionData{}
	}
	return *rd.ProposeDecree
}

// EnsureProposeDecree returns r.ProposeDecree, allocating a zero struct if it
// was nil.
func (r *ResolutionData) EnsureProposeDecree() *ProposeDecreeResolutionData {
	if r.ProposeDecree == nil {
		r.ProposeDecree = &ProposeDecreeResolutionData{}
	}
	return r.ProposeDecree
}
