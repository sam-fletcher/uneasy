package game

import (
	"testing"

	"uneasy/model"
)

func TestEsteemLockoutActive(t *testing.T) {
	sp := func(lockout bool) PlanLockoutView {
		return PlanLockoutView{
			Category:      model.CategoryEsteem,
			PlanType:      model.PlanSpreadPropaganda,
			EsteemLockout: lockout,
		}
	}
	esteemOther := PlanLockoutView{
		Category: model.CategoryEsteem,
		PlanType: model.PlanHostFestivity, // an esteem plan that is not Spread Propaganda
	}
	powerPlan := PlanLockoutView{
		Category: model.CategoryPower,
		PlanType: model.PlanMakeDemands,
	}

	tests := []struct {
		name        string
		newestFirst []PlanLockoutView
		want        bool
	}{
		{"no plans", nil, false},
		{"newest non-esteem", []PlanLockoutView{powerPlan}, false},
		{"esteem SP with lockout", []PlanLockoutView{sp(true)}, true},
		{"esteem SP without lockout", []PlanLockoutView{sp(false)}, false},
		{"esteem non-SP plan only", []PlanLockoutView{esteemOther}, false},
		{
			name:        "lockout cleared by newer non-esteem plan",
			newestFirst: []PlanLockoutView{powerPlan, sp(true)},
			want:        false,
		},
		{
			name:        "lockout still active under older esteem plans",
			newestFirst: []PlanLockoutView{sp(true), sp(true)},
			want:        true,
		},
		{
			name: "active lockout reached past an intervening esteem non-SP plan",
			// All esteem (no clearing non-esteem plan), so the older SP lockout
			// still counts.
			newestFirst: []PlanLockoutView{esteemOther, sp(true)},
			want:        true,
		},
		{
			name:        "newer non-esteem wins over older esteem lockout",
			newestFirst: []PlanLockoutView{powerPlan, esteemOther, sp(true)},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EsteemLockoutActive(tt.newestFirst); got != tt.want {
				t.Errorf("EsteemLockoutActive() = %v, want %v", got, tt.want)
			}
		})
	}
}
