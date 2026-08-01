package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── CanComplete (audit D1) ────────────────────────────────────────────────────
//
// CanComplete is pure, so the whole gate is unit-testable without a DB. These
// pin the fix for D1: it must read the outcome recorded in resolution_data, and
// must NEVER consult plan.Result — the server writes plan.Result only in the
// same statement that sets status='resolved', so it is nil for every plan
// CompletePlan can be called on. Gating on it made the demand impossible to
// complete and froze the row behind it.

func mdPlanWith(md *MakeDemandsResolutionData) (*dbgen.Plan, *ResolutionData) {
	return &dbgen.Plan{ID: 1}, &ResolutionData{MakeDemands: md}
}

func TestMDCanComplete_RejectsBeforeTheRollResolves(t *testing.T) {
	plan, resData := mdPlanWith(nil)
	require.Error(t, mdHandler{}.CanComplete(plan, resData), "no resolution data at all")

	plan, resData = mdPlanWith(&MakeDemandsResolutionData{})
	err := mdHandler{}.CanComplete(plan, resData)
	require.Error(t, err, "resolution data present but no outcome recorded")
	assert.Contains(t, err.Error(), "has not resolved yet")
}

func TestMDCanComplete_MadeDemandNeedsAllFourPicks(t *testing.T) {
	picks := make([]gamepkg.DraftChoice, 0, 4)
	for _, opt := range []string{
		gamepkg.DemandOptionControlLeverage,
		gamepkg.DemandOptionKeepOrChangeTarget,
		gamepkg.DemandOptionKeepAssets,
	} {
		picks = append(picks, gamepkg.DraftChoice{PlayerID: 7, Option: opt})
		plan, resData := mdPlanWith(&MakeDemandsResolutionData{
			Outcome: makeOutcome, DraftChoices: picks,
		})
		err := mdHandler{}.CanComplete(plan, resData)
		require.Errorf(t, err, "%d of 4 picked", len(picks))
		assert.Contains(t, err.Error(), "draft incomplete")
	}

	picks = append(picks, gamepkg.DraftChoice{PlayerID: 7, Option: gamepkg.DemandOptionPerformSteps})
	plan, resData := mdPlanWith(&MakeDemandsResolutionData{
		Outcome: makeOutcome, DraftChoices: picks,
	})
	assert.NoError(t, mdHandler{}.CanComplete(plan, resData), "all four options drafted")
}

func TestMDCanComplete_MarredDemandNeedsTheCounter(t *testing.T) {
	plan, resData := mdPlanWith(&MakeDemandsResolutionData{Outcome: marOutcome})
	err := mdHandler{}.CanComplete(plan, resData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counter-demand")

	plan, resData = mdPlanWith(&MakeDemandsResolutionData{
		Outcome: marOutcome, CounterDemandPlaced: true,
	})
	assert.NoError(t, mdHandler{}.CanComplete(plan, resData))
}

// A nil plan.Result must not block a demand that has finished its sub-flow —
// that nil is the normal state, not a missing outcome.
func TestMDCanComplete_IgnoresPlanResult(t *testing.T) {
	plan, resData := mdPlanWith(&MakeDemandsResolutionData{
		Outcome: marOutcome, CounterDemandPlaced: true,
	})
	require.Nil(t, plan.Result, "a resolving plan always has a nil result")
	assert.NoError(t, mdHandler{}.CanComplete(plan, resData))
}

// ── mdDemandBlocksTarget (audit D4) ───────────────────────────────────────────

func TestMDDemandBlocksTarget(t *testing.T) {
	// A resolved demand STILL holds its target's slot. It used not to, which let
	// a second demand land on the same target during the focus turn between two
	// plans on a row — and DemandWinnersForTargetPlan would then honour the older
	// one, silently discarding the second draft.
	for _, status := range []model.PlanStatus{
		model.PlanPending, model.PlanResolving, model.PlanResolved,
	} {
		assert.Truef(t, mdDemandBlocksTarget(&dbgen.Plan{Status: status}),
			"a %s demand holds the slot", status)
	}
	// A cancelled demand never came together, so it releases the slot.
	assert.False(t, mdDemandBlocksTarget(&dbgen.Plan{Status: model.PlanCancelled}))
}

func TestMDAlreadyDemanded(t *testing.T) {
	assert.False(t, mdAlreadyDemanded(nil), "no demands at all")
	assert.False(t, mdAlreadyDemanded([]dbgen.Plan{{Status: model.PlanCancelled}}),
		"only a fallen-through demand")
	assert.True(t, mdAlreadyDemanded([]dbgen.Plan{
		{Status: model.PlanCancelled}, {Status: model.PlanResolved},
	}), "a resolved demand among cancelled ones still blocks")
}
