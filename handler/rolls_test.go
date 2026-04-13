package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	dbgen "uneasy/db/gen"
)

// TestCancelInterference tests the pure dice cancellation algorithm.
func TestCancelInterference(t *testing.T) {
	t.Run("no interference dice, nothing cancelled", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
			{id: 3, face: 3},
		}
		interfereDice := []dieEntry{}

		result := cancelInterference(actorDice, interfereDice)

		assert.Empty(t, result, "no interference should result in no cancellations")
	})

	t.Run("interference die matches one actor die, that die cancelled", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 3},
			{id: 2, face: 4},
		}
		interfereDice := []dieEntry{
			{id: 10, face: 3},
		}

		result := cancelInterference(actorDice, interfereDice)

		assert.Len(t, result, 1)
		assert.Contains(t, result, int64(1), "actor die matching the face should be cancelled")
	})

	t.Run("multiple interference dice on same face cancel multiple actor dice", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 2},
			{id: 2, face: 2},
			{id: 3, face: 2},
		}
		interfereDice := []dieEntry{
			{id: 10, face: 2},
			{id: 11, face: 2},
		}

		result := cancelInterference(actorDice, interfereDice)

		// 2 interference dice should cancel 2 of the 3 actor dice on face 2.
		assert.Len(t, result, 2)
		assert.Contains(t, result, int64(1))
		assert.Contains(t, result, int64(2))
		// id 3 should not be cancelled
		assert.NotContains(t, result, int64(3))
	})

	t.Run("more interference dice than actor dice on a face cancels up to actor count", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 5},
			{id: 2, face: 3},
		}
		interfereDice := []dieEntry{
			{id: 10, face: 5},
			{id: 11, face: 5},
			{id: 12, face: 5},
			{id: 13, face: 5},
		}

		result := cancelInterference(actorDice, interfereDice)

		// Only 1 actor die on face 5, so only 1 cancelled despite 4 interference.
		assert.Len(t, result, 1)
		assert.Contains(t, result, int64(1))
	})

	t.Run("interference die face doesn't match any actor die, nothing cancelled", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
		}
		interfereDice := []dieEntry{
			{id: 10, face: 6},
		}

		result := cancelInterference(actorDice, interfereDice)

		assert.Empty(t, result, "no matching faces should result in no cancellations")
	})

	t.Run("mixed faces, some match some don't", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
			{id: 3, face: 3},
			{id: 4, face: 4},
		}
		interfereDice := []dieEntry{
			{id: 10, face: 2},
			{id: 11, face: 3},
			{id: 12, face: 5}, // no match
		}

		result := cancelInterference(actorDice, interfereDice)

		assert.Len(t, result, 2)
		assert.Contains(t, result, int64(2), "actor die on face 2 should be cancelled")
		assert.Contains(t, result, int64(3), "actor die on face 3 should be cancelled")
	})

	t.Run("actor and interference dice with same ID (shouldn't happen but test robustness)", func(t *testing.T) {
		// This tests the algorithm's resilience if somehow the same die ends up
		// in both actor and interference lists.
		actorDice := []dieEntry{
			{id: 1, face: 2},
		}
		interfereDice := []dieEntry{
			{id: 1, face: 2},
		}

		result := cancelInterference(actorDice, interfereDice)

		// Even with same ID, the face match should result in cancellation.
		assert.Len(t, result, 1)
		assert.Contains(t, result, int64(1))
	})
}

// TestCalculateRollResult tests the pure roll result calculation.
func TestCalculateRollResult(t *testing.T) {
	t.Run("all dice distinct faces, no cancellations", func(t *testing.T) {
		// Faces [1, 2, 3, 4] = 4 distinct, difficulty 3 → make.
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
			{id: 3, face: 3},
			{id: 4, face: 4},
		}
		cancelledIDs := make(map[int64]struct{})
		roll := &dbgen.DiceRoll{Difficulty: 3}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(4), result, "4 distinct faces → result 4")
		assert.Equal(t, makeOutcome, outcome, "result 4 >= difficulty 3 → make")
	})

	t.Run("duplicate faces count as one distinct", func(t *testing.T) {
		// Faces [1, 1, 2, 2] = 2 distinct, difficulty 2 → make.
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 1},
			{id: 3, face: 2},
			{id: 4, face: 2},
		}
		cancelledIDs := make(map[int64]struct{})
		roll := &dbgen.DiceRoll{Difficulty: 2}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(2), result, "2 distinct faces from 4 dice → result 2")
		assert.Equal(t, makeOutcome, outcome, "result 2 >= difficulty 2 → make")
	})

	t.Run("some dice cancelled, recalculate distinct faces", func(t *testing.T) {
		// Faces [1, 2, 2, 3], cancel id 2 and 4 (faces 2 and 3).
		// Remaining: [1, 2] = 2 distinct, difficulty 3 → mar.
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
			{id: 3, face: 2},
			{id: 4, face: 3},
		}
		cancelledIDs := map[int64]struct{}{
			2: {},
			4: {},
		}
		roll := &dbgen.DiceRoll{Difficulty: 3}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(2), result, "after cancellation, 2 distinct faces → result 2")
		assert.Equal(t, marOutcome, outcome, "result 2 < difficulty 3 → mar")
	})

	t.Run("all actor dice cancelled, result 0 with difficulty more than 0", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
		}
		cancelledIDs := map[int64]struct{}{
			1: {},
			2: {},
		}
		roll := &dbgen.DiceRoll{Difficulty: 1}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(0), result, "all cancelled → result 0")
		assert.Equal(t, marOutcome, outcome, "result 0 < difficulty 1 → mar")
	})

	t.Run("result exactly equals difficulty, makes", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
		}
		cancelledIDs := make(map[int64]struct{})
		roll := &dbgen.DiceRoll{Difficulty: 2}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(2), result)
		assert.Equal(t, makeOutcome, outcome, "result 2 >= difficulty 2 → make")
	})

	t.Run("adjusted difficulty takes precedence over base difficulty", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 1},
			{id: 2, face: 2},
		}
		cancelledIDs := make(map[int64]struct{})
		adjustedDiff := int16(4)
		roll := &dbgen.DiceRoll{
			Difficulty:         2,
			AdjustedDifficulty: &adjustedDiff,
		}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(2), result)
		assert.Equal(t, marOutcome, outcome, "result 2 < adjusted difficulty 4 → mar")
	})

	t.Run("no actor dice, result 0", func(t *testing.T) {
		actorDice := []dieEntry{}
		cancelledIDs := make(map[int64]struct{})
		roll := &dbgen.DiceRoll{Difficulty: 1}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(0), result)
		assert.Equal(t, marOutcome, outcome)
	})

	t.Run("single die, single face, difficulty 1", func(t *testing.T) {
		actorDice := []dieEntry{
			{id: 1, face: 3},
		}
		cancelledIDs := make(map[int64]struct{})
		roll := &dbgen.DiceRoll{Difficulty: 1}

		result, outcome := calculateRollResult(actorDice, cancelledIDs, roll)

		assert.Equal(t, int16(1), result)
		assert.Equal(t, makeOutcome, outcome)
	})
}
