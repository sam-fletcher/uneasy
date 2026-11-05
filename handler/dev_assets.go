package handler

import (
	"encoding/json"
	"net/http"

	"uneasy/db"
	"uneasy/hub"
)

// DevTakeAsset handles POST /api/dev/take-asset.
//
// Moves an asset to a chosen player the way a plan's take effect does
// (takeAssetEffect) and posts the usual asset.taken line, so a test can reach
// "bob owns alice's peer" or "alice lost her main character" without playing a
// plan. There is deliberately no player-facing equivalent: assets change hands
// only through plan resolution, never by a raw request.
//
// Body: { "asset_id": N, "player_id": N }. Mounted only when UNEASY_DEV=1.
func DevTakeAsset(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	type request struct {
		AssetID  *int64 `json:"asset_id"`
		PlayerID *int64 `json:"player_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AssetID == nil || body.PlayerID == nil {
			respondErr(w, http.StatusBadRequest, "asset_id and player_id required")
			return
		}
		ctx := r.Context()
		asset, err := s.Q.GetAssetByID(ctx, *body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		player, err := s.Q.GetPlayerByID(ctx, *body.PlayerID)
		if err != nil || player.GameID != asset.GameID {
			respondErr(w, http.StatusNotFound, "player not found in this game")
			return
		}
		if asset.OwnerID == player.ID {
			respondErr(w, http.StatusConflict, "player already owns this asset")
			return
		}
		oldOwnerID := asset.OwnerID
		updated, err := takeAssetEffect(ctx, s.Q, manager, asset.GameID, asset.ID, oldOwnerID, player.ID)
		if err != nil {
			respondErr(w, http.StatusConflict, err.Error())
			return
		}
		if g, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
			EmitAssetTaken(
				ctx,
				s.Q,
				manager,
				asset.GameID,
				asset,
				oldOwnerID,
				player.ID,
				asset.IsMainCharacter,
				logRow(g),
			)
		}
		// Taking a main character clears its flag, so recompute for the
		// replacement-choice gate.
		broadcastRowState(ctx, s.Q, manager, asset.GameID)
		respond(w, http.StatusOK, map[string]any{"asset": updated})
	}
}

// DevTearMarginalia handles POST /api/dev/tear-marginalia.
//
// Tears one marginalia the way a plan's break effect does (breakMarginalia),
// destroying the asset if it was the last, and posts the marginalia.torn line.
// Like DevTakeAsset it exists so tests can reach a damaged board; players
// cannot tear marginalia by raw request.
//
// Body: { "asset_id": N, "position": 1-4, "player_id": N } — player_id is who
// is credited with the tear. Mounted only when UNEASY_DEV=1.
func DevTearMarginalia(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	type request struct {
		AssetID  *int64 `json:"asset_id"`
		Position *int16 `json:"position"`
		PlayerID *int64 `json:"player_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
			body.AssetID == nil || body.Position == nil || body.PlayerID == nil {
			respondErr(w, http.StatusBadRequest, "asset_id, position and player_id required")
			return
		}
		ctx := r.Context()
		asset, err := s.Q.GetAssetByID(ctx, *body.AssetID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "asset not found")
			return
		}
		player, err := s.Q.GetPlayerByID(ctx, *body.PlayerID)
		if err != nil || player.GameID != asset.GameID {
			respondErr(w, http.StatusNotFound, "player not found in this game")
			return
		}
		existing, _ := s.Q.ListMarginaliaByAsset(ctx, asset.ID)
		m := marginaliaByPosition(existing, *body.Position)
		if m == nil {
			respondErr(w, http.StatusNotFound, "no marginalia at this position")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}
		destroyed, err := breakMarginalia(ctx, s.Q, manager, &asset, m, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not tear marginalia", err)
			return
		}
		if g, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
			EmitMarginaliaTorn(ctx, s.Q, manager, asset.GameID, asset, *m, player.ID, destroyed, logRow(g))
		}
		respond(w, http.StatusOK, map[string]any{"torn": true, "destroyed": destroyed})
	}
}
