package handler

// plan_name_asset.go — shared "name-asset" plan route.
//
// A few plans create an asset as a mechanical consequence of their make step
// (Propose Decree's resource, Spread Propaganda's artifact). The rules call for
// the player to author that asset, but the asset must exist the moment the make
// resolves so downstream mechanics have something to point at. We square this
// by creating the asset with a neutral placeholder name and then letting the
// PREPARER name it via this route.
//
// It is a plan-scoped, preparer-gated rename — distinct from the owner-gated
// PUT /api/assets/:id route — because the namer (preparer) is not necessarily
// the owner (e.g. a decree resource owned by the signatory). Naming is optional
// and does not gate plan completion; the placeholder simply stands until named.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// maxAssetNameLen bounds a player-authored asset name (runes).
const maxAssetNameLen = 120

// Placeholder names used until the preparer authors the real one.
const (
	lawResourceNameDefault        = "[Resource produced by the new law]"
	propagandaArtifactNameDefault = "[Artifact produced by propaganda]"
)

// nameCreatedPlanAsset renames the single asset a plan created during its make
// step. assetIDOf returns the stored asset id (nil if none created yet);
// markNamed flips the plan's "named" flag in resolution data. The response is
// written here in all paths.
func nameCreatedPlanAsset(
	w http.ResponseWriter,
	r *http.Request,
	deps *PlanDeps,
	wantType model.PlanType,
	assetIDOf func(*ResolutionData) *int64,
	markNamed func(*ResolutionData),
) {
	plan, player, ok := requirePlanAccess(w, r, deps.Q)
	if !ok {
		return
	}
	if plan.PlanType != wantType {
		respondErr(w, http.StatusBadRequest, "name-asset is not valid for this plan")
		return
	}
	if plan.Status != model.PlanResolving {
		respondErr(w, http.StatusConflict, "plan is not in resolving status")
		return
	}
	if player.ID != plan.PreparerID {
		respondErr(w, http.StatusForbidden, "only the preparer can name this asset")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		respondErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if len([]rune(name)) > maxAssetNameLen {
		respondErr(w, http.StatusBadRequest,
			fmt.Sprintf("name must be at most %d characters", maxAssetNameLen))
		return
	}

	ctx := r.Context()
	resData := loadResolutionData(plan.ResolutionData)
	assetIDPtr := assetIDOf(&resData)
	if assetIDPtr == nil {
		respondErr(w, http.StatusConflict, "no asset has been created for this plan yet")
		return
	}
	assetID := *assetIDPtr

	before, _ := deps.Q.GetAssetByID(ctx, assetID)
	if err := deps.Q.UpdateAssetName(ctx, dbgen.UpdateAssetNameParams{ID: assetID, Name: name}); err != nil {
		respondInternalErr(w, r, "could not rename asset", err)
		return
	}
	// Authoring the placeholder-named asset is a rename in the action log, so the
	// final name is reconstructable (asset.created logged the placeholder).
	if g, gErr := deps.Q.GetGameByID(ctx, plan.GameID); gErr == nil {
		EmitAssetRenamed(ctx, deps.Q, deps.Manager, plan.GameID, before, before.Name, name, player.ID, g.CurrentRow)
	}
	markNamed(&resData)
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		respondInternalErr(w, r, "could not save naming", err)
		return
	}

	enriched, err := loadAssetEnriched(r, deps.Q, assetID)
	if err != nil {
		respondInternalErr(w, r, "could not reload asset", err)
		return
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventAssetUpdated,
		model.AssetPayload{Asset: enriched})

	respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "asset": enriched})
}
