package game

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadResolutionData_Nil tests that nil input returns a zero-value struct.
func TestLoadResolutionData_Nil(t *testing.T) {
	result := LoadResolutionData(nil)
	assert.Equal(t, int16(0), result.PeerCount)
	assert.False(t, result.EsteemLockout)
	assert.Empty(t, result.MakeMarChoices)
}

// TestLoadResolutionData_Empty tests that an empty string returns a zero-value struct.
func TestLoadResolutionData_Empty(t *testing.T) {
	empty := ""
	result := LoadResolutionData(&empty)
	assert.Equal(t, int16(0), result.PeerCount)
	assert.False(t, result.EsteemLockout)
	assert.Empty(t, result.MakeMarChoices)
}

// TestLoadResolutionData_EmptyJSON tests that "{}" unmarshals to a zero-value struct.
func TestLoadResolutionData_EmptyJSON(t *testing.T) {
	emptyJSON := "{}"
	result := LoadResolutionData(&emptyJSON)
	assert.Equal(t, int16(0), result.PeerCount)
	assert.False(t, result.EsteemLockout)
	assert.Empty(t, result.MakeMarChoices)
}

// TestLoadResolutionData_PeerCount tests unmarshaling a simple PeerCount field.
func TestLoadResolutionData_PeerCount(t *testing.T) {
	jsonStr := `{"peer_count": 3}`
	result := LoadResolutionData(&jsonStr)
	assert.Equal(t, int16(3), result.PeerCount)
}

// TestLoadResolutionData_MultipleFields tests unmarshaling multiple fields correctly.
func TestLoadResolutionData_MultipleFields(t *testing.T) {
	id := int64(42)
	jsonStr := `{"fair_trade_asset_id": 42, "fair_trade_accepted": true, "messy_break_required": true}`
	result := LoadResolutionData(&jsonStr)

	assert.NotNil(t, result.FairTradeAssetID)
	assert.Equal(t, id, *result.FairTradeAssetID)
	assert.NotNil(t, result.FairTradeAccepted)
	assert.True(t, *result.FairTradeAccepted)
	assert.True(t, result.MessyBreakRequired)
}

// TestLoadResolutionData_MakeMarChoices tests unmarshaling make/mar choice strings.
func TestLoadResolutionData_MakeMarChoices(t *testing.T) {
	jsonStr := `{"make_mar_choices": ["option1", "option2", "option3"]}`
	result := LoadResolutionData(&jsonStr)

	expected := []string{"option1", "option2", "option3"}
	assert.Len(t, result.MakeMarChoices, 3)
	assert.Equal(t, expected, result.MakeMarChoices)
}

// TestLoadResolutionData_InvokedArtifactIDs tests unmarshaling artifact IDs.
func TestLoadResolutionData_InvokedArtifactIDs(t *testing.T) {
	jsonStr := `{"invoked_artifact_ids": [10, 20, 30]}`
	result := LoadResolutionData(&jsonStr)

	expected := []int64{10, 20, 30}
	assert.Len(t, result.InvokedArtifactIDs, 3)
	assert.Equal(t, expected, result.InvokedArtifactIDs)
}

// TestLoadResolutionData_RoundTrip tests that a value can be marshaled and unmarshaled.
func TestLoadResolutionData_RoundTrip(t *testing.T) {
	// Create a complex ResolutionData
	assetID := int64(999)
	playerID := int64(555)
	original := ResolutionData{
		FairTradeAssetID:   &assetID,
		FairTradeAccepted:  new(true),
		MessyBreakRequired: false,
		PeerCount:          2,
		MakeMarChoices:     []string{"a", "b"},
		InvokedArtifactIDs: []int64{1, 2, 3},
		EsteemLockout:      true,
		PartnerID:          &playerID,
	}

	// Marshal to JSON
	b, err := json.Marshal(original)
	require.NoError(t, err)
	jsonStr := string(b)

	// Unmarshal back
	result := LoadResolutionData(&jsonStr)

	// Verify round-trip
	assert.NotNil(t, result.FairTradeAssetID)
	assert.Equal(t, assetID, *result.FairTradeAssetID)
	assert.NotNil(t, result.FairTradeAccepted)
	assert.True(t, *result.FairTradeAccepted)
	assert.False(t, result.MessyBreakRequired)
	assert.Equal(t, int16(2), result.PeerCount)
	assert.True(t, result.EsteemLockout)
	assert.NotNil(t, result.PartnerID)
	assert.Equal(t, playerID, *result.PartnerID)
}

// TestLoadResolutionData_InvalidJSON tests that invalid JSON is gracefully handled.
func TestLoadResolutionData_InvalidJSON(t *testing.T) {
	// The function silently ignores errors, so invalid JSON should return a zero struct
	invalidJSON := `{this is not valid json`
	result := LoadResolutionData(&invalidJSON)
	// Should not panic and should return some struct (fields will be zero-valued)
	assert.NotNil(t, result)
}

// TestDuelState_GetAndSet tests the DuelState accessor and mutator.
func TestDuelState_GetAndSet(t *testing.T) {
	prepID := int64(100)
	targID := int64(200)
	initID := int64(300)

	original := DuelState{
		DuelType:           "wits",
		PreparerChampionID: &prepID,
		TargetChampionID:   &targID,
		Phase:              "bouts",
		PreparerStakeCount: 2,
		TargetStakeCount:   3,
		CurrentBout:        1,
		InitiativePlayerID: &initID,
	}

	// Create a ResolutionData and set the DuelState
	rd := ResolutionData{}
	rd.SetDuelState(original)

	// Retrieve the DuelState and verify
	retrieved := rd.DuelState()

	assert.Equal(t, "wits", retrieved.DuelType)
	assert.NotNil(t, retrieved.PreparerChampionID)
	assert.Equal(t, prepID, *retrieved.PreparerChampionID)
	assert.NotNil(t, retrieved.TargetChampionID)
	assert.Equal(t, targID, *retrieved.TargetChampionID)
	assert.Equal(t, "bouts", retrieved.Phase)
	assert.Equal(t, int16(2), retrieved.PreparerStakeCount)
	assert.Equal(t, int16(3), retrieved.TargetStakeCount)
	assert.Equal(t, int16(1), retrieved.CurrentBout)
	assert.NotNil(t, retrieved.InitiativePlayerID)
	assert.Equal(t, initID, *retrieved.InitiativePlayerID)
}

// TestDuelState_ZeroValues tests that DuelState() works on a zero-valued ResolutionData.
func TestDuelState_ZeroValues(t *testing.T) {
	rd := ResolutionData{}
	ds := rd.DuelState()

	assert.Empty(t, ds.DuelType)
	assert.Nil(t, ds.PreparerChampionID)
	assert.Empty(t, ds.Phase)
	assert.Equal(t, int16(0), ds.PreparerStakeCount)
}

// TestDuelState_PartialUpdate tests that SetDuelState only updates relevant fields.
func TestDuelState_PartialUpdate(t *testing.T) {
	// Start with some data in other fields
	assetID := int64(42)
	rd := ResolutionData{
		FairTradeAssetID: &assetID,
		PeerCount:        5,
	}

	// Update just the DuelState fields
	prepID := int64(111)
	ds := DuelState{
		DuelType:           "arms",
		PreparerChampionID: &prepID,
		Phase:              "stake_reveal",
	}
	rd.SetDuelState(ds)

	// Verify DuelState fields are updated
	retrieved := rd.DuelState()
	assert.Equal(t, "arms", retrieved.DuelType)

	// Verify other fields are NOT affected
	assert.NotNil(t, rd.FairTradeAssetID)
	assert.Equal(t, assetID, *rd.FairTradeAssetID)
	assert.Equal(t, int16(5), rd.PeerCount)
}
