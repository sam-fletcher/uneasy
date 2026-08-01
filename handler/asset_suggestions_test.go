package handler

import (
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPickUnusedSuggestions(t *testing.T) {
	pool := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}

	// The whole unused pool comes back, not a page of it — the client walks
	// the response locally so a reroll never blocks on the network.
	t.Run("returns the entire unused pool", func(t *testing.T) {
		got := pickUnusedSuggestions(pool, map[string]struct{}{})
		assert.ElementsMatch(t, pool, got)
	})

	t.Run("excludes used (case-insensitive, trimmed)", func(t *testing.T) {
		// Keys are normalized the way the endpoint builds the `used` set —
		// varied case/whitespace all collapse via normForDedup.
		used := map[string]struct{}{}
		for _, raw := range []string{"ALPHA", "  beta", "Gamma  ", "delta", "EpSiLoN"} {
			used[normForDedup(raw)] = struct{}{}
		}
		got := pickUnusedSuggestions(pool, used)
		assert.Empty(t, got, "every pool entry is used")
	})

	t.Run("returns only what's left when most of the pool is used", func(t *testing.T) {
		used := map[string]struct{}{"alpha": {}, "beta": {}, "gamma": {}}
		got := pickUnusedSuggestions(pool, used)
		assert.ElementsMatch(t, []string{"Delta", "Epsilon"}, got)
	})

	t.Run("never returns a used entry", func(t *testing.T) {
		used := map[string]struct{}{normForDedup("Gamma"): {}}
		for range 50 { // shuffled — sample repeatedly
			for _, s := range pickUnusedSuggestions(pool, used) {
				assert.NotEqual(t, "Gamma", s)
			}
		}
	})

	// Order varies run to run; that shuffle is what makes each picker's walk
	// through the pool feel fresh.
	t.Run("shuffles", func(t *testing.T) {
		big := make([]string, 0, 30)
		for i := range 30 {
			big = append(big, string(rune('a'+i%26))+strconv.Itoa(i))
		}
		first := pickUnusedSuggestions(big, map[string]struct{}{})
		for range 50 {
			if !slices.Equal(first, pickUnusedSuggestions(big, map[string]struct{}{})) {
				return
			}
		}
		t.Fatal("50 draws all came back in the same order — not shuffled")
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
