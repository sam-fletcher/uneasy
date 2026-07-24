package game

// draft_peer_test.go — the shared draft-peer helpers and the Make Introductions
// resolution-data queries built on them.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDraftPeerID_UniqueAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		id := NewDraftPeerID()
		require.NotEmpty(t, id)
		require.Falsef(t, seen[id], "draft id %q was minted twice", id)
		seen[id] = true
	}
}

func TestFindAndRemoveDraftPeer(t *testing.T) {
	drafts := []DraftPeer{{ID: "a", Name: "Ada"}, {ID: "b", Name: "Bo"}, {ID: "c", Name: "Cy"}}

	found := FindDraftPeer(drafts, "b")
	require.NotNil(t, found)
	assert.Equal(t, "Bo", found.Name)
	assert.Nil(t, FindDraftPeer(drafts, "z"))
	assert.Nil(t, FindDraftPeer(nil, "a"))

	// Removing the middle draft must not shift anyone else's identity — the
	// whole reason drafts carry string ids rather than slice positions.
	rest := RemoveDraftPeer(drafts, "b")
	require.Len(t, rest, 2)
	assert.Equal(t, "Ada", FindDraftPeer(rest, "a").Name)
	assert.Equal(t, "Cy", FindDraftPeer(rest, "c").Name)
	assert.Nil(t, FindDraftPeer(rest, "b"))
	assert.Len(t, drafts, 3, "the input slice is left alone")
}

func TestMIResolutionData_DraftLookupSpansBothShapes(t *testing.T) {
	// A normal plan lists its peers in Drafts…
	normal := &MakeIntroductionsResolutionData{
		Drafts: []DraftPeer{{ID: "a", Name: "Ada"}},
	}
	require.NotNil(t, normal.Draft("a"))
	assert.Nil(t, normal.Draft("t"))

	// …while a synthetic delayed-arrival plan carries the one traveller
	// separately. Draft() spans both so callers can treat them alike.
	synthetic := &MakeIntroductionsResolutionData{
		DelayedArrival: true,
		DelayedDraft:   &DraftPeer{ID: "t", Name: "Traveller"},
	}
	found := synthetic.Draft("t")
	require.NotNil(t, found)
	assert.Equal(t, "Traveller", found.Name)
	assert.Nil(t, synthetic.Draft("a"))
}

func TestMIResolutionData_HasArrivedAndMarOutcomeFor(t *testing.T) {
	mi := &MakeIntroductionsResolutionData{
		Drafts:   []DraftPeer{{ID: "a"}, {ID: "b"}},
		Arrivals: []MIArrival{{DraftID: "a", AssetID: 7}},
		MarOutcomes: []MIMarOutcome{
			{DraftID: "b", Outcome: "delayed", Done: true},
		},
	}
	assert.True(t, mi.HasArrived("a"))
	assert.False(t, mi.HasArrived("b"))
	assert.Nil(t, mi.MarOutcomeFor("a"))
	require.NotNil(t, mi.MarOutcomeFor("b"))
	assert.Equal(t, "delayed", mi.MarOutcomeFor("b").Outcome)
}

func TestMIResolutionData_PendingArrivalAuthors(t *testing.T) {
	p2, p3 := int64(2), int64(3)
	mi := &MakeIntroductionsResolutionData{
		MarOutcomes: []MIMarOutcome{
			// Written already — no longer blocking.
			{DraftID: "a", Outcome: "broken_arrival", AuthorPlayerID: &p2, Done: true},
			// Both still owed, and by the same player — named once.
			{DraftID: "b", Outcome: "broken_arrival", AuthorPlayerID: &p3},
			{DraftID: "c", Outcome: "other_retinue", AuthorPlayerID: &p3},
			// Settled inline by the preparer; nobody else owes anything.
			{DraftID: "d", Outcome: "broken_journey", Done: true},
		},
	}
	assert.Equal(t, []int64{p3}, mi.PendingArrivalAuthors())

	assert.Empty(t, (&MakeIntroductionsResolutionData{}).PendingArrivalAuthors())
}
