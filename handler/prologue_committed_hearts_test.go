package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	gamepkg "uneasy/game"
)

func TestIsDeclareableTrack(t *testing.T) {
	assert.True(t, isDeclareableTrack("power"))
	assert.True(t, isDeclareableTrack("knowledge"))
	assert.True(t, isDeclareableTrack("esteem"))
	assert.False(t, isDeclareableTrack(""))
	assert.False(t, isDeclareableTrack("bogus"))
}

// isDeclareStep is what decides whether the nobody-can-act check runs, so a
// place_set_asides step reading as a declare step would resolve a track out
// from under the player who is mid-placement.
func TestIsDeclareStep(t *testing.T) {
	assert.True(t, isDeclareStep(gamepkg.PrologueStepDeclarePower))
	assert.True(t, isDeclareStep(gamepkg.PrologueStepDeclareKnowledge))
	assert.True(t, isDeclareStep(gamepkg.PrologueStepDeclareEsteem))
	assert.False(t, isDeclareStep(gamepkg.PrologueStepPlaceSetAsidesPower))
	assert.False(t, isDeclareStep(gamepkg.PrologueStepPlaceSetAsidesEsteem))
	assert.False(t, isDeclareStep(gamepkg.PrologueStepClosing))
	assert.False(t, isDeclareStep(""))
}
