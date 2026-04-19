package game

import (
	"encoding/json"
	"testing"
)

// TestLoadResolutionData_Nil tests that nil input returns a zero-value struct.
func TestLoadResolutionData_Nil(t *testing.T) {
	result := LoadResolutionData(nil)
	if result.PeerCount != 0 || result.EsteemLockout || len(result.Choices) > 0 {
		t.Errorf("LoadResolutionData(nil) should return zero-value struct, got %+v", result)
	}
}

// TestLoadResolutionData_Empty tests that an empty string returns a zero-value struct.
func TestLoadResolutionData_Empty(t *testing.T) {
	empty := ""
	result := LoadResolutionData(&empty)
	if result.PeerCount != 0 || result.EsteemLockout || len(result.Choices) > 0 {
		t.Errorf("LoadResolutionData(\"\") should return zero-value struct, got %+v", result)
	}
}

// TestLoadResolutionData_EmptyJSON tests that "{}" unmarshals to a zero-value struct.
func TestLoadResolutionData_EmptyJSON(t *testing.T) {
	emptyJSON := "{}"
	result := LoadResolutionData(&emptyJSON)
	if result.PeerCount != 0 || result.EsteemLockout || len(result.Choices) > 0 {
		t.Errorf("LoadResolutionData(\"{}\") should return zero-value struct, got %+v", result)
	}
}

// TestLoadResolutionData_PeerCount tests unmarshaling a simple PeerCount field.
func TestLoadResolutionData_PeerCount(t *testing.T) {
	jsonStr := `{"peer_count": 3}`
	result := LoadResolutionData(&jsonStr)
	if result.PeerCount != 3 {
		t.Errorf("LoadResolutionData with peer_count = %d, want 3", result.PeerCount)
	}
}

// TestLoadResolutionData_MultipleFields tests unmarshaling multiple fields correctly.
func TestLoadResolutionData_MultipleFields(t *testing.T) {
	id := int64(42)
	jsonStr := `{"fair_trade_asset_id": 42, "fair_trade_accepted": true, "messy_break_required": true}`
	result := LoadResolutionData(&jsonStr)

	if result.FairTradeAssetID == nil || *result.FairTradeAssetID != id {
		t.Errorf("FairTradeAssetID = %v, want %d", result.FairTradeAssetID, id)
	}
	if result.FairTradeAccepted == nil || !*result.FairTradeAccepted {
		t.Errorf("FairTradeAccepted = %v, want true", result.FairTradeAccepted)
	}
	if !result.MessyBreakRequired {
		t.Errorf("MessyBreakRequired = %v, want true", result.MessyBreakRequired)
	}
}

// TestLoadResolutionData_Choices tests unmarshaling choice strings.
func TestLoadResolutionData_Choices(t *testing.T) {
	jsonStr := `{"choices": ["option1", "option2", "option3"]}`
	result := LoadResolutionData(&jsonStr)

	if len(result.Choices) != 3 {
		t.Errorf("Choices length = %d, want 3", len(result.Choices))
	}
	expected := []string{"option1", "option2", "option3"}
	for i, c := range result.Choices {
		if c != expected[i] {
			t.Errorf("Choices[%d] = %s, want %s", i, c, expected[i])
		}
	}
}

// TestLoadResolutionData_InvokedArtifactIDs tests unmarshaling artifact IDs.
func TestLoadResolutionData_InvokedArtifactIDs(t *testing.T) {
	jsonStr := `{"invoked_artifact_ids": [10, 20, 30]}`
	result := LoadResolutionData(&jsonStr)

	if len(result.InvokedArtifactIDs) != 3 {
		t.Errorf("InvokedArtifactIDs length = %d, want 3", len(result.InvokedArtifactIDs))
	}
	expected := []int64{10, 20, 30}
	for i, id := range result.InvokedArtifactIDs {
		if id != expected[i] {
			t.Errorf("InvokedArtifactIDs[%d] = %d, want %d", i, id, expected[i])
		}
	}
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
		Choices:            []string{"a", "b"},
		InvokedArtifactIDs: []int64{1, 2, 3},
		EsteemLockout:      true,
		PartnerID:          &playerID,
	}

	// Marshal to JSON
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(b)

	// Unmarshal back
	result := LoadResolutionData(&jsonStr)

	// Verify round-trip
	if result.FairTradeAssetID == nil || *result.FairTradeAssetID != assetID {
		t.Errorf("FairTradeAssetID round-trip failed")
	}
	if result.FairTradeAccepted == nil || !*result.FairTradeAccepted {
		t.Errorf("FairTradeAccepted round-trip failed")
	}
	if result.MessyBreakRequired {
		t.Errorf("MessyBreakRequired round-trip failed")
	}
	if result.PeerCount != 2 {
		t.Errorf("PeerCount round-trip failed")
	}
	if result.EsteemLockout != true {
		t.Errorf("EsteemLockout round-trip failed")
	}
	if result.PartnerID == nil || *result.PartnerID != playerID {
		t.Errorf("PartnerID round-trip failed")
	}
}

// TestLoadResolutionData_InvalidJSON tests that invalid JSON is gracefully handled.
func TestLoadResolutionData_InvalidJSON(t *testing.T) {
	// The function silently ignores errors, so invalid JSON should return a zero struct
	invalidJSON := `{this is not valid json`
	result := LoadResolutionData(&invalidJSON)
	// Should not panic and should return some struct (fields will be zero-valued)
	_ = result
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

	if retrieved.DuelType != "wits" {
		t.Errorf("DuelType = %s, want wits", retrieved.DuelType)
	}
	if retrieved.PreparerChampionID == nil || *retrieved.PreparerChampionID != prepID {
		t.Errorf("PreparerChampionID mismatch")
	}
	if retrieved.TargetChampionID == nil || *retrieved.TargetChampionID != targID {
		t.Errorf("TargetChampionID mismatch")
	}
	if retrieved.Phase != "bouts" {
		t.Errorf("Phase = %s, want bouts", retrieved.Phase)
	}
	if retrieved.PreparerStakeCount != 2 {
		t.Errorf("PreparerStakeCount = %d, want 2", retrieved.PreparerStakeCount)
	}
	if retrieved.TargetStakeCount != 3 {
		t.Errorf("TargetStakeCount = %d, want 3", retrieved.TargetStakeCount)
	}
	if retrieved.CurrentBout != 1 {
		t.Errorf("CurrentBout = %d, want 1", retrieved.CurrentBout)
	}
	if retrieved.InitiativePlayerID == nil || *retrieved.InitiativePlayerID != initID {
		t.Errorf("InitiativePlayerID mismatch")
	}
}

// TestDuelState_ZeroValues tests that DuelState() works on a zero-valued ResolutionData.
func TestDuelState_ZeroValues(t *testing.T) {
	rd := ResolutionData{}
	ds := rd.DuelState()

	if ds.DuelType != "" {
		t.Errorf("DuelType should be empty, got %s", ds.DuelType)
	}
	if ds.PreparerChampionID != nil {
		t.Errorf("PreparerChampionID should be nil, got %v", ds.PreparerChampionID)
	}
	if ds.Phase != "" {
		t.Errorf("Phase should be empty, got %s", ds.Phase)
	}
	if ds.PreparerStakeCount != 0 {
		t.Errorf("PreparerStakeCount should be 0, got %d", ds.PreparerStakeCount)
	}
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
	if retrieved.DuelType != "arms" {
		t.Errorf("DuelType not updated")
	}

	// Verify other fields are NOT affected
	if rd.FairTradeAssetID == nil || *rd.FairTradeAssetID != assetID {
		t.Errorf("FairTradeAssetID was affected by SetDuelState")
	}
	if rd.PeerCount != 5 {
		t.Errorf("PeerCount was affected by SetDuelState")
	}
}
