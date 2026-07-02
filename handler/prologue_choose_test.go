package handler

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Pure Function Unit Tests ────────────────────────────────────────────────

// TestValidateChooseRequestBody tests the JSON parsing and validation logic.
func TestValidateChooseRequestBody(t *testing.T) {
	t.Run("valid titles choice", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte(`{
			"sheet_type": "titles",
			"choice_name": "Lady",
			"asset_text": "  Lady of the North  ",
			"asset_marginalia": ["  Keeps a hidden blade  "],
			"marginalia_text": "  Noble Blood  ",
			"law_or_rumor_text": "",
			"card_assets": [
				{"suit": "h", "value": "K", "text": "  A Loyal Guard  "},
				{"suit": "d", "value": "Q", "text": ""}
			]
		}`)))

		r := &http.Request{Body: body}
		result, err := validateChooseRequestBody(r)

		require.NoError(t, err)
		assert.Equal(t, "titles", result.SheetType)
		assert.Equal(t, "Lady", result.ChoiceName)
		assert.Equal(t, "Lady of the North", result.AssetText)
		assert.Equal(t, "Keeps a hidden blade", result.AssetMarginalia)
		assert.Equal(t, "Noble Blood", result.MarginaliumText)
		assert.Empty(t, result.LawOrRumorText)
		assert.Len(t, result.CardAssets, 2)
	})

	t.Run("valid laws_rumors choice", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte(`{
			"sheet_type": "laws_rumors",
			"choice_name": "The Law of Equal Exchange",
			"asset_text": "Scroll of Laws",
			"asset_marginalia": ["Bound in leather"],
			"marginalia_text": "",
			"law_or_rumor_text": "All trades must be witnessed by a third party",
			"card_assets": []
		}`)))

		r := &http.Request{Body: body}
		result, err := validateChooseRequestBody(r)

		require.NoError(t, err)
		assert.Equal(t, "laws_rumors", result.SheetType)
		assert.Equal(t, "Scroll of Laws", result.AssetText)
		assert.Equal(t, "Bound in leather", result.AssetMarginalia)
		assert.Equal(t, "All trades must be witnessed by a third party", result.LawOrRumorText)
	})

	t.Run("missing asset_marginalia", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte(`{
			"sheet_type": "laws_rumors",
			"choice_name": "The Law of Equal Exchange",
			"asset_text": "Scroll of Laws",
			"law_or_rumor_text": "All trades must be witnessed by a third party",
			"card_assets": []
		}`)))

		r := &http.Request{Body: body}
		_, err := validateChooseRequestBody(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "one marginalia")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte("not json")))
		r := &http.Request{Body: body}

		_, err := validateChooseRequestBody(r)
		assert.Error(t, err)
	})

	t.Run("missing asset_text", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte(`{
			"sheet_type": "titles",
			"choice_name": "Lady",
			"marginalia_text": "Text"
		}`)))
		r := &http.Request{Body: body}

		_, err := validateChooseRequestBody(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "asset_text is required")
	})

	t.Run("asset_text is only whitespace", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte(`{
			"sheet_type": "titles",
			"choice_name": "Lady",
			"asset_text": "   ",
			"marginalia_text": "Text"
		}`)))
		r := &http.Request{Body: body}

		_, err := validateChooseRequestBody(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "asset_text is required")
	})

	t.Run("missing marginalia_text for titles", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte(`{
			"sheet_type": "titles",
			"choice_name": "Lady",
			"asset_text": "Asset",
			"asset_marginalia": ["a trait"]
		}`)))
		r := &http.Request{Body: body}

		_, err := validateChooseRequestBody(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "marginalia_text is required")
	})

	t.Run("missing law_or_rumor_text for laws_rumors", func(t *testing.T) {
		body := io.NopCloser(bytes.NewReader([]byte(`{
			"sheet_type": "laws_rumors",
			"choice_name": "A Law",
			"asset_text": "Asset",
			"asset_marginalia": ["a trait"]
		}`)))
		r := &http.Request{Body: body}

		_, err := validateChooseRequestBody(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "law_or_rumor_text is required")
	})
}

// TestIsLawChoice tests the law/rumor detection logic.
func TestIsLawChoice(t *testing.T) {
	tests := []struct {
		name      string
		choice    string
		expectLaw bool
	}{
		{"law at start", "Law of the Land", true},
		{"law at end", "The Grand Law", true},
		{"law mixed case", "LAW OF KINGS", true},
		{"law lowercase", "law is law", true},
		{"rumor keyword", "The Rumor Mill", false},
		{"rumor keyword end", "It's a rumor", false},
		{"generic choice", "Inheritance", false},
		{"generic choice", "Testimony of Truth", false},
		{"word containing law", "unlawful Practice", true}, // substring match — potential issue
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLawChoice(tt.choice)
			assert.Equal(t, tt.expectLaw, result)
		})
	}
}

// TestBuildCardTextLookup tests the card text mapping.
func TestBuildCardTextLookup(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		result := buildCardTextLookup([]CardAssetText{})
		assert.Empty(t, result)
	})

	t.Run("single card", func(t *testing.T) {
		cards := []CardAssetText{
			{Suit: "h", Value: "K", Text: "  My Guard  "},
		}
		result := buildCardTextLookup(cards)

		assert.Len(t, result, 1)
		assert.Equal(t, "My Guard", result["H|K"])
	})

	t.Run("multiple cards with case normalization", func(t *testing.T) {
		cards := []CardAssetText{
			{Suit: "h", Value: "K", Text: "Guard"},
			{Suit: "D", Value: "q", Text: "  Wealth  "},
			{Suit: "s", Value: "A", Text: "Weapon"},
		}
		result := buildCardTextLookup(cards)

		assert.Len(t, result, 3)
		assert.Equal(t, "Guard", result["H|K"])
		assert.Equal(t, "Wealth", result["D|Q"])
		assert.Equal(t, "Weapon", result["S|A"])
	})

	t.Run("whitespace trimming", func(t *testing.T) {
		cards := []CardAssetText{
			{Suit: "C", Value: "2", Text: "   spaced text   "},
		}
		result := buildCardTextLookup(cards)

		assert.Equal(t, "spaced text", result["C|2"])
	})

	t.Run("empty text preserved", func(t *testing.T) {
		cards := []CardAssetText{
			{Suit: "C", Value: "5", Text: ""},
		}
		result := buildCardTextLookup(cards)

		assert.Empty(t, result["C|5"])
	})
}

// BenchmarkBuildCardTextLookup benchmarks the card text lookup building.
func BenchmarkBuildCardTextLookup(b *testing.B) {
	cards := []CardAssetText{
		{Suit: "H", Value: "K", Text: "Guard"},
		{Suit: "D", Value: "Q", Text: "Treasure"},
		{Suit: "S", Value: "A", Text: "Weapon"},
		{Suit: "C", Value: "10", Text: "Building"},
	}

	b.ResetTimer()
	for range b.N {
		buildCardTextLookup(cards)
	}
}
