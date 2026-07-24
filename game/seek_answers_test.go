package game

// Pure unit tests for the Seek Answers resolution rules — no DB, no build tag,
// run by `make test`. This is the Option E payoff: the rule logic is now
// testable in microseconds without seeding a game.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"uneasy/model"
)

func TestSeekAnswersMarPenalty(t *testing.T) {
	cases := []struct {
		name                         string
		difficulty, result, eligible int16
		want                         int16
	}{
		{"shortfall capped by eligible count", 4, 1, 2, 2}, // shortfall 3, only 2 flawable
		{"shortfall below eligible count", 4, 2, 5, 2},     // shortfall 2
		{"no shortfall (result ≥ difficulty)", 2, 4, 5, 0},
		{"shortfall exactly equals eligible", 3, 1, 2, 2},
		{"no eligible resources", 5, 1, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want,
				SeekAnswersMarPenalty(tc.difficulty, tc.result, tc.eligible))
		})
	}
}

func TestEligibleSelfFlawResourceIDs(t *testing.T) {
	views := []ResourceFlawView{
		{AssetID: 1, AssetType: model.AssetResource, IntactMarginaliaCount: 2, TotalMarginaliaCount: 2},
		{
			AssetID:               2,
			AssetType:             model.AssetResource,
			IsDestroyed:           true,
			IntactMarginaliaCount: 1,
			TotalMarginaliaCount:  1,
		},
		{AssetID: 3, AssetType: model.AssetPeer, IntactMarginaliaCount: 3, TotalMarginaliaCount: 3}, // not a resource
		// All four notes torn but somehow still alive — the one unbreakable
		// shape. No live game reaches it (the last tear destroys), but the rule
		// must still exclude it.
		{AssetID: 4, AssetType: model.AssetResource, IntactMarginaliaCount: 0, TotalMarginaliaCount: 4},
		{
			AssetID:               5,
			AssetType:             model.AssetResource,
			IntactMarginaliaCount: 1,
			TotalMarginaliaCount:  1,
		}, // already flawed
	}

	got := EligibleSelfFlawResourceIDs(views, []int64{5})
	assert.Equal(t, []int64{1}, got, "only the undamaged, unflawed resource is eligible")

	// With nothing pre-flawed, both intact resources qualify (order preserved).
	got = EligibleSelfFlawResourceIDs(views, nil)
	assert.Equal(t, []int64{1, 5}, got)

	assert.Empty(t, EligibleSelfFlawResourceIDs(nil, nil))
}

// A blank resource — no marginalia rows at all — is a valid self-flaw target
// even though there is nothing to tear: the break destroys it outright
// (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D3). Before the backstop it was
// silently invulnerable, which let a marred plan under-count its penalty.
func TestEligibleSelfFlawResourceIDs_BlankIsEligible(t *testing.T) {
	views := []ResourceFlawView{
		{AssetID: 1, AssetType: model.AssetResource, IntactMarginaliaCount: 0, TotalMarginaliaCount: 0},
		{AssetID: 2, AssetType: model.AssetResource, IntactMarginaliaCount: 0, TotalMarginaliaCount: 2},
	}
	assert.Equal(t, []int64{1}, EligibleSelfFlawResourceIDs(views, nil),
		"the blank resource is breakable; the all-torn one is not")

	// A blank resource already flawed this resolution is spent like any other.
	assert.Empty(t, EligibleSelfFlawResourceIDs(views, []int64{1}))
}
