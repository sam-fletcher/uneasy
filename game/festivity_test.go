package game

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostFestivityDifficulty(t *testing.T) {
	cases := []struct {
		rank int16
		want int16
	}{
		{1, 5}, {2, 4}, {3, 3}, {4, 2}, {5, 1}, {6, 1},
	}
	for _, c := range cases {
		got := HostFestivityDifficulty(c.rank)
		assert.Equal(t, c.want, got)
	}
}

func TestFestivityStateRoundtrip(t *testing.T) {
	pid := int64(7)
	s := FestivityState{
		Phase:             FestivityPhaseSocializing,
		Guests:            []int64{1, 2, 3},
		Outcomes:          map[string]string{"1": FestivityOutcomeMake},
		GuestMakes:        map[string]string{"1": FestivityMakeSpreadRumor},
		GuestIOUs:         []int64{1},
		AcceptDuels:       []int64{2},
		PendingDuelPlanID: &pid,
	}
	var r ResolutionData
	r.SetFestivityState(s)
	got := r.FestivityState()
	assert.Equal(t, FestivityPhaseSocializing, got.Phase)
	assert.Len(t, got.Guests, 3)
	assert.True(t, got.HasAcceptDuels(2))
	assert.False(t, got.HasAcceptDuels(1))
	assert.NotNil(t, got.PendingDuelPlanID)
	assert.Equal(t, int64(7), *got.PendingDuelPlanID)
}

func TestFestivityAllGuestsResolved(t *testing.T) {
	s := FestivityState{
		Guests:   []int64{1, 2},
		Outcomes: map[string]string{"1": FestivityOutcomeMake},
	}
	assert.False(t, s.AllGuestsResolved())
	s.Outcomes["2"] = FestivityOutcomeOptOut
	assert.True(t, s.AllGuestsResolved())
}

func TestFestivityPendingHostChoices(t *testing.T) {
	s := FestivityState{
		Guests: []int64{1, 2, 3, 4},
		Outcomes: map[string]string{
			"1": FestivityOutcomeMake,
			"2": FestivityOutcomeMar,
			"3": FestivityOutcomeOptOut,
			"4": FestivityOutcomeMar,
		},
		HostChoices: map[string]string{
			"2": FestivityMakeSpreadRumor,
		},
	}
	pending := s.PendingHostChoices()
	require.Len(t, pending, 2)
	// Order-independent check: should be {3, 4}.
	seen := map[int64]bool{}
	for _, id := range pending {
		seen[id] = true
	}
	assert.True(t, seen[3])
	assert.True(t, seen[4])
}

func TestFestivityConsumeIOU(t *testing.T) {
	s := FestivityState{GuestIOUs: []int64{1, 2, 3}}
	require.True(t, s.ConsumeIOU(2))
	assert.Len(t, s.GuestIOUs, 2)
	assert.Equal(t, int64(1), s.GuestIOUs[0])
	assert.Equal(t, int64(3), s.GuestIOUs[1])
	assert.False(t, s.ConsumeIOU(99))
}

func TestFestivityValidators(t *testing.T) {
	assert.True(t, IsValidFestivityMakeOption(FestivityMakeSpreadRumor))
	assert.False(t, IsValidFestivityMakeOption("nope"))
	assert.True(t, IsValidFestivityMarOption(FestivityMarBreakSelf))
	assert.False(t, IsValidFestivityMarOption(FestivityMakeSpreadRumor))
}
