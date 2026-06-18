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
		Outcomes:          map[string]string{"1": FestivityOutcomeMake},
		GuestMakes:        map[string]string{"1": FestivityMakeSpreadRumor},
		HostMakesTaken:    []string{FestivityMakeIntroducePeer},
		GuestIOUs:         []int64{1},
		AcceptDuels:       []int64{2},
		PendingDuelPlanID: &pid,
	}
	var r ResolutionData
	*r.EnsureFestivity() = s
	got := *r.EnsureFestivity()
	assert.Equal(t, []string{FestivityMakeIntroducePeer}, got.HostMakesTaken)
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

func TestFestivityHostMakesAndEndable(t *testing.T) {
	roster := []int64{1, 2, 3, 4, 5}
	s := FestivityResolutionData{
		Outcomes: map[string]string{
			"1": FestivityOutcomeMake,   // success → holds a mar to inflict
			"2": FestivityOutcomeMar,    // +1 host make
			"3": FestivityOutcomeOptOut, // +1 host make
			"4": FestivityOutcomeMar,    // +1 host make
			"5": FestivityOutcomeHost,   // +1 host make (for hosting)
		},
		GuestIOUs: []int64{1},
	}
	// Earned = mar + mar + opt_out + host = 4; a guest's own make never earns
	// the host one.
	assert.Equal(t, 4, s.EarnedHostMakes(roster))
	assert.Equal(t, 4, s.RemainingHostMakes(roster))

	// Not endable: host has taken no makes and player 1's mar is unspent.
	assert.False(t, s.EventEndable(roster))

	// Host takes all four makes — still not endable while the IOU is unspent.
	s.HostMakesTaken = []string{
		FestivityMakeSpreadRumor, FestivityMakeIntroducePeer,
		FestivityMakeSpreadRumor, FestivityMakeIntroducePeer,
	}
	assert.Equal(t, 0, s.RemainingHostMakes(roster))
	assert.False(t, s.EventEndable(roster))

	// Player 1 inflicts their mar → everything settled → endable.
	require.True(t, s.ConsumeIOU(1))
	assert.True(t, s.EventEndable(roster))
}

func TestFestivityEventEndableRequiresAllChosen(t *testing.T) {
	roster := []int64{1, 2}
	// Player 2 hasn't chosen yet → not endable even with no makes/mars pending.
	s := FestivityResolutionData{
		Outcomes: map[string]string{"1": FestivityOutcomeHost},
	}
	s.HostMakesTaken = []string{FestivityMakeSpreadRumor} // the host's one earned make
	assert.False(t, s.EventEndable(roster))
	s.Outcomes["2"] = FestivityOutcomeMake
	assert.True(t, s.EventEndable(roster))
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
