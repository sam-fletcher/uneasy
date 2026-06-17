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
	s := FestivityResolutionData{
		Phase:             FestivityPhaseSocializing,
		Outcomes:          map[string]string{"1": FestivityOutcomeMake},
		GuestMakes:        map[string]string{"1": FestivityMakeSpreadRumor},
		GuestIOUs:         []int64{1},
		AcceptDuels:       []int64{2},
		PendingDuelPlanID: &pid,
	}
	var r ResolutionData
	*r.EnsureFestivity() = s
	got := *r.EnsureFestivity()
	assert.Equal(t, FestivityPhaseSocializing, got.Phase)
	assert.True(t, got.HasAcceptDuels(2))
	assert.False(t, got.HasAcceptDuels(1))
	assert.NotNil(t, got.PendingDuelPlanID)
	assert.Equal(t, int64(7), *got.PendingDuelPlanID)
}

func TestFestivityAllGuestsResolved(t *testing.T) {
	roster := []int64{1, 2}
	s := FestivityResolutionData{
		Outcomes: map[string]string{"1": FestivityOutcomeMake},
	}
	assert.False(t, s.AllGuestsResolved(roster))
	s.Outcomes["2"] = FestivityOutcomeOptOut
	assert.True(t, s.AllGuestsResolved(roster))
}

func TestFestivityPendingHostChoices(t *testing.T) {
	roster := []int64{1, 2, 3, 4}
	s := FestivityResolutionData{
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
	pending := s.PendingHostChoices(roster)
	require.Len(t, pending, 2)
	// Order-independent check: should be {3, 4}.
	seen := map[int64]bool{}
	for _, id := range pending {
		seen[id] = true
	}
	assert.True(t, seen[3])
	assert.True(t, seen[4])
}

func TestFestivityPendingGuests(t *testing.T) {
	roster := []int64{10, 20, 30}
	s := FestivityResolutionData{
		Outcomes: map[string]string{"20": FestivityOutcomeMake},
	}
	// Roster order preserved; resolved guest (20) dropped.
	assert.Equal(t, []int64{10, 30}, s.PendingGuests(roster))
	s.Outcomes["10"] = FestivityOutcomeOptOut
	s.Outcomes["30"] = FestivityOutcomeMar
	assert.Empty(t, s.PendingGuests(roster))
}

func TestFestivityActiveRoller(t *testing.T) {
	roster := []int64{10, 20, 30}
	// Nobody rolling.
	s := FestivityResolutionData{Outcomes: map[string]string{}, GuestRollIDs: map[string]int64{}}
	assert.EqualValues(t, 0, s.ActiveRoller(roster))
	// 20 has rolled but not chosen → mid-turn.
	s.GuestRollIDs["20"] = 99
	assert.EqualValues(t, 20, s.ActiveRoller(roster))
	// 20 has now chosen → no longer mid-turn.
	s.Outcomes["20"] = FestivityOutcomeMar
	assert.EqualValues(t, 0, s.ActiveRoller(roster))
}

func TestFestivityConsumeIOU(t *testing.T) {
	s := FestivityResolutionData{GuestIOUs: []int64{1, 2, 3}}
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
