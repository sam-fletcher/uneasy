package game

// Coverage for previously-untested pure helpers across the game package,
// surfaced by a "which exported funcs have no test" audit (Step 4 / option 2 of
// the testability roadmap). These are cheap, hermetic unit tests for logic that
// the integration tests exercise only incidentally — or not at all.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── ProposeDecree: NextAmender (mar amendment turn order) ─────────────────────

func TestNextAmender(t *testing.T) {
	pd := &ProposeDecreeResolutionData{AmendmentOrder: []int64{10, 20, 30}}

	assert.EqualValues(t, 10, pd.NextAmender(), "first unamended in order")

	pd.AmendedBy = []int64{10}
	assert.EqualValues(t, 20, pd.NextAmender())

	pd.AmendedBy = []int64{10, 20}
	assert.EqualValues(t, 30, pd.NextAmender())

	pd.AmendedBy = []int64{10, 20, 30}
	assert.EqualValues(t, 0, pd.NextAmender(), "chain complete")

	// Empty order → 0.
	assert.EqualValues(t, 0, (&ProposeDecreeResolutionData{}).NextAmender())

	// Out-of-order / stray AmendedBy entries don't matter — order is authoritative.
	pd2 := &ProposeDecreeResolutionData{AmendmentOrder: []int64{10, 20, 30}, AmendedBy: []int64{20, 99}}
	assert.EqualValues(t, 10, pd2.NextAmender(), "returns first in order not yet amended")
}

// ── Prologue: committed-heart validation ─────────────────────────────────────

func TestValidateCommittedHearts(t *testing.T) {
	t.Run("distinct cards are valid", func(t *testing.T) {
		err := ValidateCommittedHearts([]CommittedHeart{
			{PlayerID: 1, CardID: 1, Track: "power"},
			{PlayerID: 1, CardID: 2, Track: "esteem"},
			{PlayerID: 2, CardID: 3, Track: "power"},
		})
		assert.NoError(t, err)
	})

	t.Run("a card committed to two tracks is invalid", func(t *testing.T) {
		err := ValidateCommittedHearts([]CommittedHeart{
			{PlayerID: 1, CardID: 7, Track: "power"},
			{PlayerID: 1, CardID: 7, Track: "esteem"},
		})
		assert.Error(t, err)
	})

	t.Run("empty is valid", func(t *testing.T) {
		assert.NoError(t, ValidateCommittedHearts(nil))
	})
}

// ── Prologue: suit / track / sheet mappings ──────────────────────────────────

func TestAssetTypeForSuit(t *testing.T) {
	assert.Equal(t, prologueAssetHolding, AssetTypeForSuit(SuitClubs))
	assert.Equal(t, prologueAssetResource, AssetTypeForSuit(SuitDiamonds))
	assert.Equal(t, prologueAssetArtifact, AssetTypeForSuit(SuitSpades))
	assert.Equal(t, "peer", AssetTypeForSuit(SuitHearts))
	assert.Empty(t, AssetTypeForSuit('X'), "unknown suit → empty")
}

func TestSuitForTrack(t *testing.T) {
	assert.Equal(t, SuitClubs, SuitForTrack(PrologueTrackPower))
	assert.Equal(t, SuitDiamonds, SuitForTrack(PrologueTrackKnowledge))
	assert.Equal(t, SuitSpades, SuitForTrack(PrologueTrackEsteem))
	assert.EqualValues(t, 0, SuitForTrack("nonsense"), "unknown track → 0")
}

func TestAssetTypeForSheet_RoundTripsEverySheet(t *testing.T) {
	require.NotEmpty(t, PrologueSheets, "sanity: sheets are defined")
	for _, s := range PrologueSheets {
		assert.Equalf(t, s.ChoiceAssetType, AssetTypeForSheet(s.Type),
			"sheet %q should map to its own ChoiceAssetType", s.Type)
	}
	assert.Empty(t, AssetTypeForSheet("not_a_sheet"), "unknown sheet → empty")
}

func TestFindPrologueChoice(t *testing.T) {
	require.NotEmpty(t, PrologueSheets)
	// Every (sheet, choice) name resolves to that exact choice.
	for _, s := range PrologueSheets {
		for _, c := range s.Choices {
			got := FindPrologueChoice(s.Type, c.Name)
			if assert.NotNilf(t, got, "choice %q in sheet %q should be found", c.Name, s.Type) {
				assert.Equal(t, c.Name, got.Name)
			}
		}
	}
	assert.Nil(t, FindPrologueChoice("not_a_sheet", "whatever"), "unknown sheet → nil")
	if len(PrologueSheets) > 0 {
		assert.Nil(t, FindPrologueChoice(PrologueSheets[0].Type, "not_a_choice"),
			"unknown choice name → nil")
	}
}
