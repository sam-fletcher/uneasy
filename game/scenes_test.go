package game

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"uneasy/model"
)

// FollowOnPrompt must return non-empty text for every supported plan type
// so the scene-setup form always has something to display when following on
// from a resolved plan.
func TestFollowOnPrompt_AllPlanTypesCovered(t *testing.T) {
	all := []model.PlanType{
		model.PlanMakeDemands,
		model.PlanProposeDecree,
		model.PlanExchangeCourtiers,
		model.PlanMakeWar,
		model.PlanMakeIntroductions,
		model.PlanSeekAnswers,
		model.PlanChronicleHistories,
		model.PlanClandestinelyLiaise,
		model.PlanSpreadPropaganda,
		model.PlanSpreadRumors,
		model.PlanProposeDuel,
		model.PlanHostFestivity,
	}
	for _, pt := range all {
		got := FollowOnPrompt(pt)
		assert.NotEmpty(t, got, "FollowOnPrompt(%s) returned empty string", pt)
		// Every prompt should start with the rules' canonical "Set a scene"
		// stem so the surrounding UI can rely on a consistent shape.
		assert.True(t, strings.HasPrefix(got, "Set a scene"),
			"FollowOnPrompt(%s) = %q; want prefix \"Set a scene\"", pt, got)
	}
}

func TestFollowOnPrompt_UnknownReturnsEmpty(t *testing.T) {
	got := FollowOnPrompt(model.PlanType("not_a_real_plan"))
	assert.Empty(t, got, "FollowOnPrompt(unknown)")
}
