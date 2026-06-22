package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaxChoices pins the per-plan option budgets (rules' dice math) without a
// DB. result = distinct dice faces; difficulty = effective difficulty.
func TestMaxChoices(t *testing.T) {
	// Spread Propaganda: make creates exactly one artifact; mar = difficulty − result.
	assert.Equal(t, 1, spHandler{}.MaxChoices(makeOutcome, 5, 3))
	assert.Equal(t, 2, spHandler{}.MaxChoices(marOutcome, 1, 3))

	// Spread Rumors: make = result (repeatable); mar = difficulty − result.
	assert.Equal(t, 4, srHandler{}.MaxChoices(makeOutcome, 4, 2))
	assert.Equal(t, 2, srHandler{}.MaxChoices(marOutcome, 2, 4))

	// Seek Answers: result for both make and mar.
	assert.Equal(t, 3, saHandler{}.MaxChoices(makeOutcome, 3, 2))
	assert.Equal(t, 3, saHandler{}.MaxChoices(marOutcome, 3, 5))
}

// TestExchangeCourtiersValidateChoices pins EC's rules: pick exactly one option
// whose level is within the margin (result−difficulty for make, difficulty−result
// for mar). Make levels: messy 0, legal 1, conspiracy 2. Mar: fair_trade 1,
// riposte 2, forfeit 3.
func TestExchangeCourtiersValidateChoices(t *testing.T) {
	ec := ecHandler{}

	// Exactly one option, always.
	require.Error(t, ec.ValidateChoices(makeOutcome, 5, 3, nil))
	require.Error(t, ec.ValidateChoices(makeOutcome, 5, 3, []string{"legal", "conspiracy"}))

	// Make, margin 0: only Messy (level 0) allowed.
	require.NoError(t, ec.ValidateChoices(makeOutcome, 3, 3, []string{"messy"}))
	require.Error(t, ec.ValidateChoices(makeOutcome, 3, 3, []string{"legal"}))
	// Make, margin 1: messy/legal but not conspiracy.
	require.NoError(t, ec.ValidateChoices(makeOutcome, 4, 3, []string{"legal"}))
	require.Error(t, ec.ValidateChoices(makeOutcome, 4, 3, []string{"conspiracy"}))
	// Make, margin 2: anything.
	require.NoError(t, ec.ValidateChoices(makeOutcome, 5, 3, []string{"conspiracy"}))

	// Mar, margin 1: only Fair Trade (level 1).
	require.NoError(t, ec.ValidateChoices(marOutcome, 3, 4, []string{"fair_trade"}))
	require.Error(t, ec.ValidateChoices(marOutcome, 3, 4, []string{"riposte"}))
	// Mar, margin 2: fair_trade/riposte but not forfeit.
	require.NoError(t, ec.ValidateChoices(marOutcome, 2, 4, []string{"riposte"}))
	require.Error(t, ec.ValidateChoices(marOutcome, 2, 4, []string{"forfeit"}))
	// Mar, margin 3: forfeit allowed.
	require.NoError(t, ec.ValidateChoices(marOutcome, 1, 4, []string{"forfeit"}))

	// Unknown option is rejected.
	require.Error(t, ec.ValidateChoices(makeOutcome, 5, 3, []string{"bogus"}))
}
