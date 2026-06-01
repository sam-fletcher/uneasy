package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPickUnusedSuggestions(t *testing.T) {
	pool := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}

	t.Run("caps at n", func(t *testing.T) {
		got := pickUnusedSuggestions(pool, map[string]struct{}{}, 3)
		assert.Len(t, got, 3)
	})

	t.Run("excludes used (case-insensitive, trimmed)", func(t *testing.T) {
		// Keys are normalized the way the endpoint builds the `used` set —
		// varied case/whitespace all collapse via normForDedup.
		used := map[string]struct{}{}
		for _, raw := range []string{"ALPHA", "  beta", "Gamma  ", "delta", "EpSiLoN"} {
			used[normForDedup(raw)] = struct{}{}
		}
		got := pickUnusedSuggestions(pool, used, 3)
		assert.Empty(t, got, "every pool entry is used")
	})

	t.Run("returns fewer than n when pool is small", func(t *testing.T) {
		used := map[string]struct{}{"alpha": {}, "beta": {}, "gamma": {}}
		got := pickUnusedSuggestions(pool, used, 3)
		assert.Len(t, got, 2, "only Delta and Epsilon remain")
		for _, s := range got {
			assert.NotContains(t, []string{"Alpha", "Beta", "Gamma"}, s)
		}
	})

	t.Run("never returns a used entry", func(t *testing.T) {
		used := map[string]struct{}{normForDedup("Gamma"): {}}
		for range 50 { // shuffled — sample repeatedly
			for _, s := range pickUnusedSuggestions(pool, used, 5) {
				assert.NotEqual(t, "Gamma", s)
			}
		}
	})
}

func TestValidAssetType(t *testing.T) {
	for _, ok := range []string{"peer", "holding", "artifact", "resource"} {
		assert.True(t, validAssetType(ok), ok)
	}
	for _, bad := range []string{"", "Peer", "law", "secret", "rumor"} {
		assert.False(t, validAssetType(bad), bad)
	}
}
