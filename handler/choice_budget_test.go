package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMaxChoices pins the per-plan option budgets (rules' dice math) without a
// DB. result = distinct dice faces; difficulty = effective difficulty.
func TestMaxChoices(t *testing.T) {
	// Spread Propaganda: make creates exactly one artifact; mar = difficulty − result.
	assert.Equal(t, 1, spHandler{}.MaxChoices(makeOutcome, 5, 3))
	assert.Equal(t, 2, spHandler{}.MaxChoices(marOutcome, 1, 3))

	// Exchange Courtiers: make = result − difficulty; mar = difficulty − result.
	assert.Equal(t, 2, ecHandler{}.MaxChoices(makeOutcome, 5, 3))
	assert.Equal(t, 0, ecHandler{}.MaxChoices(makeOutcome, 3, 3))
	assert.Equal(t, 3, ecHandler{}.MaxChoices(marOutcome, 1, 4))

	// Spread Rumors: make = result (repeatable); mar = difficulty − result.
	assert.Equal(t, 4, srHandler{}.MaxChoices(makeOutcome, 4, 2))
	assert.Equal(t, 2, srHandler{}.MaxChoices(marOutcome, 2, 4))

	// Seek Answers: result for both make and mar.
	assert.Equal(t, 3, saHandler{}.MaxChoices(makeOutcome, 3, 2))
	assert.Equal(t, 3, saHandler{}.MaxChoices(marOutcome, 3, 5))
}
