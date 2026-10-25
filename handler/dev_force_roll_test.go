// handler/dev_force_roll_test.go — unit tests for the pure half of
// POST /api/dev/force-roll. No DB: devForceFaces and devForceTarget are
// plain functions over counts and difficulties, and they carry the whole
// correctness argument for the route (the rest is SetDieFace in a loop plus
// the production resolution path).

package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// distinct counts the distinct values in faces — the same quantity
// calculateRollResult derives from the uncancelled actor dice.
func distinct(faces []int16) int {
	seen := map[int16]struct{}{}
	for _, f := range faces {
		seen[f] = struct{}{}
	}
	return len(seen)
}

func TestDevForceFaces_HitsTargetDistinctCount(t *testing.T) {
	for _, tc := range []struct {
		name          string
		actor, interf int
		target        int16
	}{
		{"one die, one distinct", 1, 0, 1},
		{"two dice, both distinct", 2, 0, 2},
		{"five dice held to one", 5, 0, 1},
		{"five dice held to three", 5, 0, 3},
		{"six dice, all six faces", 6, 0, 6},
		{"interference present", 4, 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			faces, interfFace, msg := devForceFaces(tc.actor, tc.interf, tc.target)
			require.Empty(t, msg)
			require.Len(t, faces, tc.actor)
			assert.Equal(t, int(tc.target), distinct(faces),
				"forced faces must yield exactly the target distinct count")
			for _, f := range faces {
				assert.GreaterOrEqual(t, f, int16(1))
				assert.LessOrEqual(t, f, int16(diceSides))
			}
			if tc.interf > 0 {
				// The whole point: no interference die may share a face with an
				// actor die, or cancelInterference would pair them and drop the
				// result below the target.
				assert.NotContains(t, faces, interfFace,
					"interference face must not collide with any actor face")
			}
		})
	}
}

func TestDevForceFaces_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name          string
		actor, interf int
		target        int16
	}{
		{"no actor dice", 0, 2, 1},
		{"target below one", 3, 0, 0},
		{"target above the actor's dice", 2, 0, 3},
		{"target above six", 8, 0, 7},
		{"six distinct under interference", 6, 1, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, msg := devForceFaces(tc.actor, tc.interf, tc.target)
			assert.NotEmpty(t, msg, "infeasible target must be refused with a reason")
		})
	}
}

func TestDevForceTarget(t *testing.T) {
	ptr := func(v int16) *int16 { return &v }

	t.Run("make aims at the difficulty", func(t *testing.T) {
		got, msg := devForceTarget(makeOutcome, nil, 4)
		require.Empty(t, msg)
		assert.EqualValues(t, 4, got, "the minimum make is result == difficulty")
	})

	t.Run("mar aims one below", func(t *testing.T) {
		got, msg := devForceTarget(marOutcome, nil, 4)
		require.Empty(t, msg)
		assert.EqualValues(t, 3, got, "the maximum mar is result == difficulty − 1")
	})

	t.Run("mar is impossible at difficulty 1", func(t *testing.T) {
		_, msg := devForceTarget(marOutcome, nil, 1)
		assert.NotEmpty(t, msg, "every result ≥ 1, so difficulty 1 always makes")
	})

	t.Run("explicit result wins", func(t *testing.T) {
		got, msg := devForceTarget("", ptr(5), 2)
		require.Empty(t, msg)
		assert.EqualValues(t, 5, got)
	})

	t.Run("result and outcome must agree", func(t *testing.T) {
		_, msg := devForceTarget(marOutcome, ptr(5), 2)
		assert.NotEmpty(t, msg, "5 vs difficulty 2 is a make, not a mar")

		got, okMsg := devForceTarget(makeOutcome, ptr(5), 2)
		require.Empty(t, okMsg)
		assert.EqualValues(t, 5, got)
	})

	t.Run("an unknown outcome is refused", func(t *testing.T) {
		_, msg := devForceTarget("fumble", nil, 3)
		assert.NotEmpty(t, msg)
	})
}
