//go:build integration

// handler/plan_make_demands_rulings_integration_test.go — the owner's
// 2026-11-03 Make Demands rulings: the keep-or-change target option is live only
// on the plans that have a "who", and on a duel or a liaison its aim is settled
// before the first step.

package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
)

// On every plan outside the four with a "who", the option is a plain dud: no
// roll-hold and the route refuses.
func TestMakeDemandsRulings_KeepOrChangeIsADudOffTheFourPlans(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	plan, roll, _, winnerIdx, _ := setupDemandPreRollWindow(t, h, game.DemandOptionKeepOrChangeTarget)
	winnerID := h.tg.Players[winnerIdx].ID

	wp, err := h.q.GetParticipant(ctx, dbgen.GetParticipantParams{RollID: roll.ID, PlayerID: winnerID})
	require.NoError(t, err)
	assert.True(t, wp.IsReady, "a dud winner must not be seeded unready")
	rl, err := h.q.GetDiceRollByID(ctx, roll.ID)
	require.NoError(t, err)
	assert.False(t, rollIsOpen(&rl), "nobody holds a Spread Propaganda roll for a dud option")

	code, body := h.post(winnerIdx, demandRetargetPath(plan.ID), map[string]any{"keep": true})
	assert.Equalf(t, http.StatusConflict, code, "the route refuses on a dud plan: %v", body)
	assert.Contains(t, body["error"], "inert")
}
