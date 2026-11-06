//go:build integration

// handler/plan_make_choice_no_roll_integration_test.go — POST /make-choice must
// be backed by a settled roll. A plan that has no roll yet has no outcome, so a
// direct call naming "make" or "mar" would commit a result the dice never gave.
// The app never sends such a call (every panel that posts make-choice does so
// after the roll), but the route is public, so the server enforces it for every
// plan at the shared seam (checkChoiceMatchesRoll) instead of leaving each plan
// to remember a ChoiceGate.
//
// The audit this pins is in the PR that added it; the short version: no plan
// reaches make-choice legitimately without a roll. The plans that resolve with
// no roll at all (Make War, Host Festivity, Clandestinely Liaise) and the one
// roll-less path through a rolling plan (Exchange Courtiers' accepted fair
// trade) each finish through their own route, never through make-choice.

package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// allPlanTypes is every plan the registry knows. TestMakeChoice_NoRoll_AllPlans
// fails if the registry grows a plan this list lacks, so a new plan cannot skip
// the audit.
var allPlanTypes = []model.PlanType{
	model.PlanMakeDemands, model.PlanProposeDecree, model.PlanExchangeCourtiers,
	model.PlanMakeWar, model.PlanMakeIntroductions, model.PlanSeekAnswers,
	model.PlanChronicleHistories, model.PlanClandestinelyLiaise, model.PlanSpreadPropaganda,
	model.PlanSpreadRumors, model.PlanProposeDuel, model.PlanHostFestivity,
}

// TestMakeChoice_NoRoll_AllPlans parks a resolving plan of every type with no
// roll and posts make-choice for both outcomes as its preparer. Every one must be
// refused with 409 and leave the plan exactly as it was: still resolving, no
// result, no choice recorded, no asset moved.
func TestMakeChoice_NoRoll_AllPlans(t *testing.T) {
	for pt := range AllHandlers() {
		require.Contains(t, allPlanTypes, pt, "new plan type %s: audit its make-choice with no roll", pt)
	}

	for _, planType := range allPlanTypes {
		for _, outcome := range []string{makeOutcome, marOutcome} {
			t.Run(string(planType)+"/"+outcome, func(t *testing.T) {
				h := newPlanLifecycle(t, 3)
				ctx := context.Background()
				preparerIdx := h.focusPlayerIdx()
				targetIdx := (preparerIdx + 1) % len(h.tg.Players)
				targetPeer := h.seedPeer(targetIdx, "coveted peer")
				_ = h.seedPeer(preparerIdx, "preparer's peer")

				// Category is irrelevant to this route; Power is a valid value.
				plan, err := h.q.CreatePlan(ctx, dbgen.CreatePlanParams{
					GameID:         h.tg.Game.ID,
					PlanType:       planType,
					Category:       model.CategoryPower,
					PreparerID:     h.tg.Players[preparerIdx].ID,
					TargetPlayerID: &h.tg.Players[targetIdx].ID,
					TargetAssetID:  &targetPeer,
					RowNumber:      &h.tg.Game.CurrentRow,
					PreparedAtRow:  h.tg.Game.CurrentRow,
				})
				require.NoError(t, err)
				require.NoError(t, h.q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
					ID: plan.ID, Status: model.PlanResolving,
				}))

				path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"
				code, body := h.post(preparerIdx, path, map[string]any{"result": outcome})
				assert.Equalf(t, http.StatusConflict, code, "%s %s with no roll: %v", planType, outcome, body)

				after, err := h.q.GetPlanByID(ctx, plan.ID)
				require.NoError(t, err)
				assert.Equal(t, model.PlanResolving, after.Status, "plan must still be resolving")
				assert.Nil(t, after.Result, "no result may be stored")
				assert.False(t, loadResolutionData(after.ResolutionData).ChoiceSubmitted,
					"no choice may be recorded")
				peer, err := h.q.GetAssetByID(ctx, targetPeer)
				require.NoError(t, err)
				assert.Equal(t, h.tg.Players[targetIdx].ID, peer.OwnerID, "no asset may move")
			})
		}
	}
}

// TestExchangeCourtiers_MakeChoice_NoRollRefused is the reported hole through the
// real flow: the preparer's make-choice, sent before any roll exists, took the
// targeted peer and resolved the plan. Refused at every roll-less moment —
// before the target's offer, and after it while the preparer has neither
// accepted nor declined.
func TestExchangeCourtiers_MakeChoice_NoRollRefused(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	preparerIdx := h.focusPlayerIdx()
	targetIdx := (preparerIdx + 1) % len(h.tg.Players)
	targetPeer := h.seedPeer(targetIdx, "coveted peer")
	requestedPeer := h.seedPeer(preparerIdx, "requested peer")

	notes := "exchange"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[targetIdx].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &notes,
	})
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))

	assertRefused := func(stage string) {
		t.Helper()
		path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"
		for _, body := range []map[string]any{
			{"result": makeOutcome},
			{"result": makeOutcome, "choices": []string{"legal"}},
			{"result": marOutcome, "choices": []string{"forfeit"}},
		} {
			code, resp := h.post(preparerIdx, path, body)
			require.Equalf(t, http.StatusConflict, code, "%s, %v: %v", stage, body, resp)
		}
		cur, err := h.q.GetPlanByID(ctx, plan.ID)
		require.NoError(t, err)
		require.Equalf(t, model.PlanResolving, cur.Status, "%s: plan must still be resolving", stage)
		peer, err := h.q.GetAssetByID(ctx, targetPeer)
		require.NoError(t, err)
		require.Equalf(t, h.tg.Players[targetIdx].ID, peer.OwnerID, "%s: the peer must not move", stage)
	}

	assertRefused("before the offer")

	code, body := ecOffer(t, h, targetIdx, plan.ID, requestedPeer)
	require.Equalf(t, http.StatusOK, code, "offer: %v", body)
	assertRefused("offer pending")
}

// TestExchangeCourtiers_NoRollPaths_StillWork pins the roll-less ways EC
// legitimately finishes, which the roll requirement must not touch: an accepted
// fair trade resolves the plan with no roll at all (and leaves nothing for
// make-choice to do), and a declined one rolls, after which the ordinary
// roll-backed make-choice goes through.
func TestExchangeCourtiers_NoRollPaths_StillWork(t *testing.T) {
	t.Run("accepted fair trade resolves with no roll", func(t *testing.T) {
		h := newPlanLifecycle(t, 3)
		ctx := context.Background()
		preparerIdx := h.focusPlayerIdx()
		targetIdx := (preparerIdx + 1) % len(h.tg.Players)
		targetPeer := h.seedPeer(targetIdx, "coveted peer")
		requestedPeer := h.seedPeer(preparerIdx, "requested peer")
		notes := "exchange"
		plan := h.prepare(PreparePlanRequest{
			PlanType:         model.PlanExchangeCourtiers,
			TargetPlayerID:   &h.tg.Players[targetIdx].ID,
			TargetAssetID:    &targetPeer,
			PreparationNotes: &notes,
		})
		h.jumpToRow(*plan.RowNumber)
		require.Nil(t, h.resolve(plan.ID))

		code, body := ecOffer(t, h, targetIdx, plan.ID, requestedPeer)
		require.Equalf(t, http.StatusOK, code, "offer: %v", body)
		path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/fair-trade"
		code, body = h.post(preparerIdx, path, map[string]any{"action": "accept"})
		require.Equalf(t, http.StatusOK, code, "accept: %v", body)

		_, err := h.q.GetDiceRollByPlanID(ctx, &plan.ID)
		require.True(t, isNoRows(err), "an accepted fair trade must not create a roll")
		h.requireResolved(plan.ID, makeOutcome)
	})

	t.Run("declined fair trade rolls, then make-choice works", func(t *testing.T) {
		h := newPlanLifecycle(t, 3)
		plan, preparerIdx, _, _, _, rollID, _ := ecToRoll(t, h)
		path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-choice"

		// While the dice are still open the existing refusal stands.
		code, body := h.post(preparerIdx, path, map[string]any{"result": makeOutcome, "choices": []string{"messy"}})
		require.Equalf(t, http.StatusConflict, code, "open roll: %v", body)

		h.forceRoll(rollID, makeOutcome, 15)
		code, body = h.post(preparerIdx, path, map[string]any{"result": makeOutcome, "choices": []string{"messy"}})
		require.Equalf(t, http.StatusOK, code, "settled roll: %v", body)
	})
}
