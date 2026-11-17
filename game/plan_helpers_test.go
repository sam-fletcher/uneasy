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

// TestLoadResolutionData_InvalidJSON: the lenient reader doesn't panic or
// return an error, but the value it hands back still remembers that it came
// from an unreadable blob.
func TestLoadResolutionData_InvalidJSON(t *testing.T) {
	invalidJSON := `{this is not valid json`
	result := LoadResolutionData(&invalidJSON)
	assert.NotNil(t, result)
	assert.ErrorIs(t, result.LoadErr(), ErrResolutionDataMalformed)
}

// A blob that isn't JSON used to read as an empty one, indistinguishable from a
// plan with no state. ParseResolutionData says which it was.
func TestParseResolutionData_BlobThatIsNotJSONSurfacesTheError(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":  `{this is not valid json`,
		"truncated": `{"make_mar_choices":[{"option":"a"`,
	} {
		t.Run(name, func(t *testing.T) {
			rd, err := ParseResolutionData(&raw)
			require.ErrorIs(t, err, ErrResolutionDataMalformed)
			assert.Equal(t, err, rd.LoadErr(), "the value carries the error it was decoded with")
		})
	}
}

// Saved fields have changed JSON type before: make_mar_choices was a list of
// strings before it became a list of Choice; Spread Propaganda's give_peer_done
// and break_self_done were booleans before they became counts; a Make
// Introductions draft's marginalia was a list before it became a string. A plan
// resolved under the old shape must still read: encoding/json skips the one
// field and decodes the rest, the plan is not marked unreadable, and a later
// save writes it back in the current shape.
func TestParseResolutionData_OlderFieldShapesStillRead(t *testing.T) {
	for name, raw := range map[string]string{
		"make_mar_choices as strings": `{"choice_auto_applied":true,"make_mar_choices":["peers_arrive"]}`,
		"a bool where a count is now": `{"choice_auto_applied":true,"spread_propaganda":{"give_peer_done":true}}`,
		"a list where a string is now": `{"choice_auto_applied":true,` +
			`"make_introductions":{"drafts":[{"id":"d1","name":"Ana","marginalia":["a note"],"creator_id":1}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			rd, err := ParseResolutionData(&raw)
			require.ErrorIs(t, err, ErrResolutionDataMistyped)
			require.NotErrorIs(t, err, ErrResolutionDataMalformed)
			require.NoError(t, rd.LoadErr(), "an older shape does not make the plan unreadable")
			assert.True(t, rd.ChoiceAutoApplied, "the rest of the blob decoded")
		})
	}
}

func TestParseResolutionData_ReadableBlobsHaveNoError(t *testing.T) {
	empty := ""
	valid := `{"choice_auto_applied":true}`
	for name, raw := range map[string]*string{"nil": nil, "empty": &empty, "valid": &valid} {
		t.Run(name, func(t *testing.T) {
			rd, err := ParseResolutionData(raw)
			require.NoError(t, err)
			assert.NoError(t, rd.LoadErr())
		})
	}
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

// A row stored before the commit markers existed carries only make_mar_choices,
// and a zero-pick commit stores nothing at all; both must still read correctly.
func TestChoicesCommitted_StoredShapes(t *testing.T) {
	cases := map[string]bool{
		`{}`:                                    false,
		`{"make_mar_choices":[{"option":"x"}]}`: true,
		`{"choice_submitted":true}`:             true,
		`{"choice_auto_applied":true}`:          true,
	}
	for raw, want := range cases {
		s := raw
		rd := LoadResolutionData(&s)
		assert.Equal(t, want, rd.ChoicesCommitted(), raw)
	}
}
