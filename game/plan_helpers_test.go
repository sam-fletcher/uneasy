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
	assert.Nil(t, result.MakeIntroductions)
	assert.Nil(t, result.SpreadPropaganda)
	assert.Empty(t, result.MakeMarChoices)
}

// TestLoadResolutionData_Empty tests that an empty string returns a zero-value struct.
func TestLoadResolutionData_Empty(t *testing.T) {
	empty := ""
	result := LoadResolutionData(&empty)
	assert.Nil(t, result.MakeIntroductions)
	assert.Nil(t, result.SpreadPropaganda)
	assert.Empty(t, result.MakeMarChoices)
}

// TestLoadResolutionData_EmptyJSON tests that "{}" unmarshals to a zero-value struct.
func TestLoadResolutionData_EmptyJSON(t *testing.T) {
	emptyJSON := "{}"
	result := LoadResolutionData(&emptyJSON)
	assert.Nil(t, result.MakeIntroductions)
	assert.Nil(t, result.SpreadPropaganda)
	assert.Empty(t, result.MakeMarChoices)
}

// TestLoadResolutionData_PeerCount tests unmarshaling a Make Introductions
// peer_count nested under the make_introductions key.
func TestLoadResolutionData_PeerCount(t *testing.T) {
	jsonStr := `{"make_introductions": {"peer_count": 3}}`
	result := LoadResolutionData(&jsonStr)
	require.NotNil(t, result.MakeIntroductions)
	assert.Equal(t, int16(3), result.MakeIntroductions.PeerCount)
}

// TestLoadResolutionData_MultipleFields tests unmarshaling multiple fields correctly.
func TestLoadResolutionData_MultipleFields(t *testing.T) {
	id := int64(42)
	jsonStr := `{"exchange_courtiers": {"fair_trade_asset_id": 42, "fair_trade_accepted": true, "messy_break_required": true}}`
	result := LoadResolutionData(&jsonStr)

	require.NotNil(t, result.ExchangeCourtiers)
	ec := result.ExchangeCourtiers
	assert.NotNil(t, ec.FairTradeAssetID)
	assert.Equal(t, id, *ec.FairTradeAssetID)
	assert.NotNil(t, ec.FairTradeAccepted)
	assert.True(t, *ec.FairTradeAccepted)
	assert.True(t, ec.MessyBreakRequired)
}

// TestLoadResolutionData_MakeMarChoices tests unmarshaling make/mar choice entries.
func TestLoadResolutionData_MakeMarChoices(t *testing.T) {
	playerID := int64(42)
	jsonStr := `{"make_mar_choices": [
		{"option": "option1"},
		{"option": "option2", "player_id": 42},
		{"option": "option3"}
	]}`
	result := LoadResolutionData(&jsonStr)

	require.Len(t, result.MakeMarChoices, 3)
	assert.Nil(t, result.MakeMarChoices[0].PlayerID)
	assert.Equal(t, "option1", result.MakeMarChoices[0].Option)
	require.NotNil(t, result.MakeMarChoices[1].PlayerID)
	assert.Equal(t, playerID, *result.MakeMarChoices[1].PlayerID)
	assert.Equal(t, "option2", result.MakeMarChoices[1].Option)
	assert.Nil(t, result.MakeMarChoices[2].PlayerID)
}

// TestLoadResolutionData_InvokedArtifactIDs tests unmarshaling artifact IDs.
func TestLoadResolutionData_InvokedArtifactIDs(t *testing.T) {
	jsonStr := `{"chronicle_histories": {"invoked_artifact_ids": [10, 20, 30]}}`
	result := LoadResolutionData(&jsonStr)

	expected := []int64{10, 20, 30}
	require.NotNil(t, result.ChronicleHistories)
	assert.Len(t, result.ChronicleHistories.InvokedArtifactIDs, 3)
	assert.Equal(t, expected, result.ChronicleHistories.InvokedArtifactIDs)
}

// TestLoadResolutionData_RoundTrip tests that a value can be marshaled and unmarshaled.
func TestLoadResolutionData_RoundTrip(t *testing.T) {
	// Create a complex ResolutionData
	assetID := int64(999)
	playerID := int64(555)
	original := ResolutionData{
		ExchangeCourtiers: &ExchangeCourtiersResolutionData{
			FairTradeAssetID:  &assetID,
			FairTradeAccepted: new(true),
		},
		MakeIntroductions:  &MakeIntroductionsResolutionData{PeerCount: 2},
		MakeMarChoices:     []Choice{{Option: "a"}, {Option: "b", PlayerID: &playerID}},
		ChronicleHistories: &ChronicleHistoriesResolutionData{InvokedArtifactIDs: []int64{1, 2, 3}},
		SpreadPropaganda:   &SpreadPropagandaResolutionData{EsteemLockout: true},
		Liaise:             &LiaiseResolutionData{PartnerID: &playerID},
	}

	// Marshal to JSON
	b, err := json.Marshal(original)
	require.NoError(t, err)
	jsonStr := string(b)

	// Unmarshal back
	result := LoadResolutionData(&jsonStr)

	// Verify round-trip
	require.NotNil(t, result.ExchangeCourtiers)
	assert.NotNil(t, result.ExchangeCourtiers.FairTradeAssetID)
	assert.Equal(t, assetID, *result.ExchangeCourtiers.FairTradeAssetID)
	assert.NotNil(t, result.ExchangeCourtiers.FairTradeAccepted)
	assert.True(t, *result.ExchangeCourtiers.FairTradeAccepted)
	assert.False(t, result.ExchangeCourtiers.MessyBreakRequired)
	require.NotNil(t, result.MakeIntroductions)
	assert.Equal(t, int16(2), result.MakeIntroductions.PeerCount)
	require.NotNil(t, result.SpreadPropaganda)
	assert.True(t, result.SpreadPropaganda.EsteemLockout)
	require.NotNil(t, result.Liaise)
	require.NotNil(t, result.Liaise.PartnerID)
	assert.Equal(t, playerID, *result.Liaise.PartnerID)
}

// TestLoadResolutionData_InvalidJSON tests that invalid JSON is gracefully handled.
func TestLoadResolutionData_InvalidJSON(t *testing.T) {
	// The function silently ignores errors, so invalid JSON should return a zero struct
	invalidJSON := `{this is not valid json`
	result := LoadResolutionData(&invalidJSON)
	// Should not panic and should return some struct (fields will be zero-valued)
	assert.NotNil(t, result)
}

// TestEnsureDuel tests the EnsureDuel write-path helper.
func TestEnsureDuel(t *testing.T) {
	prepID := int64(100)
	targID := int64(200)
	initID := int64(300)

	rd := ResolutionData{}
	d := rd.EnsureDuel()
	d.DuelType = "wits"
	d.PreparerChampionID = &prepID
	d.TargetChampionID = &targID
	d.Phase = DuelPhaseBouts
	d.PreparerStakeCount = 2
	d.TargetStakeCount = 3
	d.CurrentBout = 1
	d.InitiativePlayerID = &initID

	// EnsureDuel returns the same pointer on subsequent calls.
	require.Same(t, d, rd.EnsureDuel())

	require.NotNil(t, rd.Duel)
	assert.Equal(t, "wits", rd.Duel.DuelType)
	assert.Equal(t, DuelPhaseBouts, rd.Duel.Phase)
	assert.Equal(t, int16(2), rd.Duel.PreparerStakeCount)
}

// TestDuel_PartialUpdate confirms that mutating Duel doesn't disturb other
// nested per-plan structs on the same ResolutionData.
func TestDuel_PartialUpdate(t *testing.T) {
	assetID := int64(42)
	rd := ResolutionData{
		ExchangeCourtiers: &ExchangeCourtiersResolutionData{FairTradeAssetID: &assetID},
		MakeIntroductions: &MakeIntroductionsResolutionData{PeerCount: 5},
	}

	prepID := int64(111)
	d := rd.EnsureDuel()
	d.DuelType = "arms"
	d.PreparerChampionID = &prepID
	d.Phase = DuelPhaseStaking

	require.NotNil(t, rd.Duel)
	assert.Equal(t, "arms", rd.Duel.DuelType)

	require.NotNil(t, rd.ExchangeCourtiers)
	assert.NotNil(t, rd.ExchangeCourtiers.FairTradeAssetID)
	assert.Equal(t, assetID, *rd.ExchangeCourtiers.FairTradeAssetID)
	require.NotNil(t, rd.MakeIntroductions)
	assert.Equal(t, int16(5), rd.MakeIntroductions.PeerCount)
}
