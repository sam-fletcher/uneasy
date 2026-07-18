package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrackResolved(t *testing.T) {
	cases := []struct {
		track, currentStep string
		want               bool
	}{
		// Power has not yet been resolved at the start.
		{"power", "declare_power", false},
		{"knowledge", "declare_power", false},
		{"esteem", "declare_power", false},

		// After power is finalized and set-asides are being placed.
		{"power", "place_set_asides_power", false},
		{"knowledge", "place_set_asides_power", false},

		// Mid-knowledge: power is resolved.
		{"power", "declare_knowledge", true},
		{"knowledge", "declare_knowledge", false},
		{"esteem", "declare_knowledge", false},

		// Mid-esteem: power and knowledge resolved.
		{"power", "declare_esteem", true},
		{"knowledge", "declare_esteem", true},
		{"esteem", "declare_esteem", false},

		// closing / past all tracks → all resolved.
		{"power", "closing", true},
		{"esteem", "closing", true},
	}
	for _, c := range cases {
		got := trackResolved(c.track, c.currentStep)
		assert.Equal(t, c.want, got, "track=%s step=%s", c.track, c.currentStep)
	}
}

func TestIsDeclareableTrack(t *testing.T) {
	assert.True(t, isDeclareableTrack("power"))
	assert.True(t, isDeclareableTrack("knowledge"))
	assert.True(t, isDeclareableTrack("esteem"))
	assert.False(t, isDeclareableTrack(""))
	assert.False(t, isDeclareableTrack("bogus"))
}
