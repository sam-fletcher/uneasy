//go:build integration

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

// A plan whose resolution_data no longer decodes used to read as an empty one.
// Routes then judged the request against a plan with no state ("this plan does
// not call for an artifact") or, worse, acted first and saved a mostly-empty
// copy over the blob. Every plan route now refuses up front with a 500 that
// says the state can't be read, and changes nothing — not the blob, not the
// board.
func TestPlanRoute_RefusesAPlanWhoseSavedStateIsUnreadable(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "test propaganda"
	plan := h.prepare(PreparePlanRequest{PlanType: model.PlanSpreadPropaganda, PreparationNotes: &notes})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolveAndCast(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, "make", 0)
	h.requireAutoApplied(plan.ID, "make", "create_artifact")

	// The route would work on a healthy plan: it is the blob, and only the
	// blob, that this test breaks.
	garbage := `{"spread_propaganda":{"artifact_required":"yes"`
	require.NoError(t, h.q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{
		ID: plan.ID, ResolutionData: &garbage,
	}))
	artifactsBefore := countArtifacts(t, h, ctx)

	preparerIdx := h.preparerIdxFor(plan.ID)
	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/create-artifact"
	code, body := h.post(preparerIdx, path, map[string]any{
		"narration": "The old order crumbles.", "name": "The New Orthodoxy", "marginalia": []string{"Born of a lie"},
	})
	require.Equalf(t, http.StatusInternalServerError, code, "an unreadable plan is refused, not read as empty: %v", body)
	assert.Contains(t, body["error"], "saved state could not be read")

	assert.Equal(t, artifactsBefore, countArtifacts(t, h, ctx), "nothing was created before the refusal")
	after, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, after.ResolutionData)
	assert.Equal(t, garbage, *after.ResolutionData, "the stored blob is left exactly as it was")
}

// Saved fields have changed JSON type before (make_mar_choices was a list of
// strings; give_peer_done was a bool). A plan resolved under the old shape must
// still load — it is the unreadable blob that is refused, not the older one.
func TestPlanRoute_StillServesAPlanSavedInAnOlderShape(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()

	notes := "test propaganda"
	plan := h.prepare(PreparePlanRequest{PlanType: model.PlanSpreadPropaganda, PreparationNotes: &notes})

	old := `{"choice_auto_applied":true,"make_mar_choices":["create_artifact"],"spread_propaganda":{"give_peer_done":true}}`
	require.NoError(t, h.q.SetPlanResolutionData(ctx, dbgen.SetPlanResolutionDataParams{
		ID: plan.ID, ResolutionData: &old,
	}))

	code, body := h.get(h.preparerIdxFor(plan.ID), "/api/plans/"+strconv.FormatInt(plan.ID, 10))
	require.Equalf(t, http.StatusOK, code, "an older field shape still loads: %v", body)
}
