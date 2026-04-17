package game

import "testing"

func TestHostFestivityDifficulty(t *testing.T) {
	cases := []struct {
		rank int16
		want int16
	}{
		{1, 5}, {2, 4}, {3, 3}, {4, 2}, {5, 1}, {6, 1},
	}
	for _, c := range cases {
		got := HostFestivityDifficulty(c.rank)
		if got != c.want {
			t.Errorf("HostFestivityDifficulty(%d)=%d want %d", c.rank, got, c.want)
		}
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
	if got.Phase != FestivityPhaseSocializing {
		t.Errorf("phase: got %q", got.Phase)
	}
	if len(got.Guests) != 3 {
		t.Errorf("guests: got %v", got.Guests)
	}
	if !got.HasAcceptDuels(2) || got.HasAcceptDuels(1) {
		t.Errorf("HasAcceptDuels wrong: %v", got.AcceptDuels)
	}
	if got.PendingDuelPlanID == nil || *got.PendingDuelPlanID != 7 {
		t.Errorf("pending duel: got %v", got.PendingDuelPlanID)
	}
}

func TestFestivityAllGuestsResolved(t *testing.T) {
	s := FestivityState{
		Guests:   []int64{1, 2},
		Outcomes: map[string]string{"1": FestivityOutcomeMake},
	}
	if s.AllGuestsResolved() {
		t.Error("want not resolved with missing outcome")
	}
	s.Outcomes["2"] = FestivityOutcomeOptOut
	if !s.AllGuestsResolved() {
		t.Error("want resolved after all outcomes recorded")
	}
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
	if len(pending) != 2 {
		t.Fatalf("want 2 pending, got %v", pending)
	}
	// Order-independent check: should be {3, 4}.
	seen := map[int64]bool{}
	for _, id := range pending {
		seen[id] = true
	}
	if !seen[3] || !seen[4] {
		t.Errorf("unexpected pending set: %v", pending)
	}
}

func TestFestivityConsumeIOU(t *testing.T) {
	s := FestivityState{GuestIOUs: []int64{1, 2, 3}}
	if !s.ConsumeIOU(2) {
		t.Fatal("expected to consume")
	}
	if len(s.GuestIOUs) != 2 || s.GuestIOUs[0] != 1 || s.GuestIOUs[1] != 3 {
		t.Errorf("remaining IOUs wrong: %v", s.GuestIOUs)
	}
	if s.ConsumeIOU(99) {
		t.Error("expected false for missing player")
	}
}

func TestFestivityValidators(t *testing.T) {
	if !IsValidFestivityMakeOption(FestivityMakeSpreadRumor) {
		t.Error("spread_rumor should be valid make")
	}
	if IsValidFestivityMakeOption("nope") {
		t.Error("nope should be invalid")
	}
	if !IsValidFestivityMarOption(FestivityMarBreakSelf) {
		t.Error("break_self should be valid mar")
	}
	if IsValidFestivityMarOption(FestivityMakeSpreadRumor) {
		t.Error("make option leaked into mar")
	}
}
