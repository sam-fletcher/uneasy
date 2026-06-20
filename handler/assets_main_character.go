package handler

// handler/assets_main_character.go — Promoting and demoting a peer asset
// to/from main character. The pure rule decision lives in
// game.DecideMainCharacterChange; these helpers load inputs, run it, and
// apply the resulting tears, destroys, flag flips, and broadcasts.

import (
	"context"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// tearOldMainCharacterMarginalia tears the marginalia the MC swap requires
// (and destroys the old MC if that was its last intact one), broadcasting and
// logging each step. Split out of tearAndReplaceOldMainCharacter to keep the
// nesting shallow. Returns false on error.
func tearOldMainCharacterMarginalia(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	oldMC *dbgen.Asset,
	oldMargs []dbgen.Marginalium,
	player *dbgen.Player,
	decision game.MCDecision,
) bool {
	target := marginaliaByPosition(oldMargs, decision.TearPosition)
	if _, err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID:       target.ID,
		TornByID: &player.ID,
	}); err != nil {
		respondInternalErr(w, r, "could not tear marginalia", err)
		return false
	}
	if h, ok := manager.Get(oldMC.GameID); ok {
		h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
			AssetID:  oldMC.ID,
			Position: decision.TearPosition,
			TornByID: player.ID,
		})
	}
	if g, gErr := q.GetGameByID(ctx, oldMC.GameID); gErr == nil {
		EmitMarginaliaTorn(
			ctx,
			q,
			manager,
			oldMC.GameID,
			*oldMC,
			*target,
			player.ID,
			decision.DestroysOldMC,
			g.CurrentRow,
		)
	}

	if !decision.DestroysOldMC {
		return true
	}
	if err := q.DestroyAsset(ctx, oldMC.ID); err != nil {
		respondInternalErr(w, r, "could not destroy old main character", err)
		return false
	}
	if h, ok := manager.Get(oldMC.GameID); ok {
		h.BroadcastEvent(model.EventAssetDestroyed, model.AssetIDPayload{AssetID: oldMC.ID})
	}
	if g, gErr := q.GetGameByID(ctx, oldMC.GameID); gErr == nil {
		EmitAssetDestroyed(ctx, q, manager, oldMC.GameID, *oldMC, g.CurrentRow)
	}
	oldMC.IsDestroyed = true
	return true
}

// tearAndReplaceOldMainCharacter handles replacing an existing main character
// with a new one. It performs any necessary tearing, broadcasts events, and
// clears the old MC's flag. Returns false on error.
func tearAndReplaceOldMainCharacter(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	oldMC *dbgen.Asset,
	oldMargs []dbgen.Marginalium,
	player *dbgen.Player,
	asset *dbgen.Asset,
	decision game.MCDecision,
) bool {
	if decision.NeedsTear {
		if !tearOldMainCharacterMarginalia(ctx, w, r, q, manager, oldMC, oldMargs, player, decision) {
			return false
		}
	}

	// Clear the old MC's flag. AssetDestroyed already removes it from
	// frontend state, so AssetUpdated is only needed when not destroyed.
	err := q.SetMainCharacter(ctx, dbgen.SetMainCharacterParams{
		ID:              oldMC.ID,
		IsMainCharacter: false,
	})
	if err != nil {
		respondInternalErr(w, r, "could not clear old main character", err)
		return false
	}
	if !oldMC.IsDestroyed {
		if e, err := loadAssetEnriched(r, q, oldMC.ID); err == nil {
			if h, ok := manager.Get(asset.GameID); ok {
				h.BroadcastEvent(model.EventAssetUpdated, model.AssetPayload{Asset: e})
			}
		}
	}
	return true
}

// applyMainCharacterChange handles promoting/demoting a peer to/from main
// character. Rule logic (validation, tear-required-or-not, destroy-on-tear)
// lives in game.DecideMainCharacterChange; this function loads the inputs,
// runs the decision, and applies the resulting writes + broadcasts.
func applyMainCharacterChange(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	q *dbgen.Queries,
	manager *hub.Manager,
	asset *dbgen.Asset,
	player *dbgen.Player,
	isMainCharacter bool,
	tearPosition *int16,
) bool {
	if !isMainCharacter {
		// Demote — no rule check, no tear.
		if err := q.SetMainCharacter(ctx, dbgen.SetMainCharacterParams{
			ID:              asset.ID,
			IsMainCharacter: false,
		}); err != nil {
			respondInternalErr(w, r, "could not update main character", err)
			return false
		}
		return true
	}

	// Find existing MC (if any, other than the asset being promoted).
	owned, err := q.ListAssetsByOwner(ctx, player.ID)
	if err != nil {
		respondInternalErr(w, r, "could not list owner assets", err)
		return false
	}
	var oldMC *dbgen.Asset
	for i := range owned {
		a := &owned[i]
		if a.GameID == asset.GameID && a.IsMainCharacter && a.ID != asset.ID && !a.IsDestroyed {
			oldMC = a
			break
		}
	}
	var oldMargs []dbgen.Marginalium
	if oldMC != nil {
		oldMargs, err = q.ListMarginaliaByAsset(ctx, oldMC.ID)
		if err != nil {
			respondInternalErr(w, r, "could not load old main character marginalia", err)
			return false
		}
	}

	// Map storage rows → decoupled domain views for the pure decision.
	var targetView *game.AssetView
	if asset != nil {
		targetView = &game.AssetView{AssetType: asset.AssetType}
	}
	var oldMCView *game.AssetView
	if oldMC != nil {
		oldMCView = &game.AssetView{AssetType: oldMC.AssetType}
	}
	margViews := make([]game.MarginaliumView, len(oldMargs))
	for i := range oldMargs {
		margViews[i] = game.MarginaliumView{Position: oldMargs[i].Position, IsTorn: oldMargs[i].IsTorn}
	}

	decision, derr := game.DecideMainCharacterChange(targetView, oldMCView, margViews, tearPosition)
	if derr != nil {
		respondErr(w, derr.Code, derr.Message)
		return false
	}

	if oldMC != nil {
		if !tearAndReplaceOldMainCharacter(ctx, w, r, q, manager,
			oldMC, oldMargs, player, asset, decision) {
			return false
		}
	}

	err = q.SetMainCharacter(ctx, dbgen.SetMainCharacterParams{
		ID:              asset.ID,
		IsMainCharacter: true,
	})
	if err != nil {
		respondInternalErr(w, r, "could not update main character", err)
		return false
	}
	return true
}
