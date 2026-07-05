package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireOneMarginalia(t *testing.T) {
	t.Run("exactly one non-empty entry", func(t *testing.T) {
		got, err := requireOneMarginalia([]string{"  A trait  "})
		require.NoError(t, err)
		assert.Equal(t, "A trait", got)
	})

	t.Run("empty slice", func(t *testing.T) {
		_, err := requireOneMarginalia(nil)
		assert.Error(t, err)
	})

	t.Run("all-blank entries", func(t *testing.T) {
		_, err := requireOneMarginalia([]string{"", "   "})
		assert.Error(t, err)
	})

	t.Run("two entries", func(t *testing.T) {
		_, err := requireOneMarginalia([]string{"A trait", "Another"})
		assert.Error(t, err)
	})

	t.Run("two entries, one blank, still errors", func(t *testing.T) {
		// Blanks are dropped before counting, but the rule is exactly one — a
		// second real entry always errors, blank padding or not.
		_, err := requireOneMarginalia([]string{"A trait", "", "Another"})
		assert.Error(t, err)
	})

	t.Run("rejects marginalia over the length cap", func(t *testing.T) {
		_, err := requireOneMarginalia([]string{strings.Repeat("a", maxMarginaliaLen+1)})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "300")
	})

	t.Run("accepts marginalia at exactly the length cap", func(t *testing.T) {
		got, err := requireOneMarginalia([]string{strings.Repeat("a", maxMarginaliaLen)})
		require.NoError(t, err)
		assert.Len(t, got, maxMarginaliaLen)
	})
}
