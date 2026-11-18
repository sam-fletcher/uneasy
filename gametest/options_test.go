package gametest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The Main Event plays rows 1 to lastRecordRow and the Shake-Up begins at
// shakeUpRow. A Main Event seed must not be placed on the Shake-Up's row, which
// live play cannot reach, while the Shake-Up seed must accept its own row.
func TestCurrentRowIsBoundByPhase(t *testing.T) {
	mainEvent := applyOptions(1, lastRecordRow, []Option{WithCurrentRow(shakeUpRow)})
	require.Error(t, mainEvent.validate(3), "a Main Event cannot be seeded on the Shake-Up's row")

	shakeUp := applyOptions(shakeUpRow, shakeUpRow, nil)
	require.NoError(t, shakeUp.validate(3), "the Shake-Up's own row is valid")
}
