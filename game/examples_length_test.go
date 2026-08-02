package game

import "testing"

// maxExampleLen bounds every suggestion string in both example pools.
//
// SuggestionPicker.svelte shows one example at a time on a single line at a
// fixed height, shrinking the type only as far as it must to avoid wrapping.
// The floor is 11px; past that the text ellipsizes. In the narrowest column
// (300px content box — docs/STYLE_GUIDE.md "Layout widths") the example gets
// ~206px of text width, which 34 characters clear at ~12px. A longer entry
// would silently ellipsize on a phone, so the pools are capped where the
// layout was measured rather than where the DB column ends.
const maxExampleLen = 34

func TestExamplePoolsFitOneLine(t *testing.T) {
	pools := map[string]map[string][]string{
		"prologue":   AssetExamples,
		"marginalia": MarginaliaExamples,
	}
	for pool, byType := range pools {
		for assetType, examples := range byType {
			for _, s := range examples {
				if n := len([]rune(s)); n > maxExampleLen {
					t.Errorf("%s/%s: %q is %d chars, over the %d-char one-line budget",
						pool, assetType, s, n, maxExampleLen)
				}
			}
		}
	}
}
