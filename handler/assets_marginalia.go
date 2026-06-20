package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// ── Marginalia handlers ───────────────────────────────────────────────────────

// AddMarginalia handles POST /api/assets/{assetId}/marginalia.
//
// Owner adds a marginalia note to their asset (max 4 total).
// Body: { text }
func AddMarginalia(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetOwner(w, r, s.Q)
		if !ok {
			return
		}

		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Text = strings.TrimSpace(body.Text)
		if body.Text == "" {
			respondErr(w, http.StatusBadRequest, "text is required")
			return
		}

		ctx := r.Context()

		existing, err := s.Q.ListMarginaliaByAsset(ctx, asset.ID)
		if err != nil {
			respondInternalErr(w, r, "could not check marginalia", err)
			return
		}
		if int64(len(existing)) >= maxMarginalia {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("asset already has %d marginalia", maxMarginalia))
			return
		}

		// Find next available position (1-4), skipping occupied slots.
		occupied := map[int16]bool{}
		for _, m := range existing {
			occupied[m.Position] = true
		}
		var nextPos int16
		for p := int16(1); p <= maxMarginalia; p++ {
			if !occupied[p] {
				nextPos = p
				break
			}
		}

		m, err := s.Q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID:  asset.ID,
			Position: nextPos,
			Text:     body.Text,
		})
		if err != nil {
			respondInternalErr(w, r, "could not add marginalia", err)
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaAdded, model.MarginaliaPayload{
				AssetID:    asset.ID,
				Marginalia: m,
			})
		}
		if g, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
			EmitMarginaliaAdded(ctx, s.Q, manager, asset.GameID, *asset, m, player.ID, &g.CurrentRow)
		}

		respond(w, http.StatusCreated, map[string]any{"marginalia": m})
	}
}

// UpdateMarginalia handles PUT /api/assets/{assetId}/marginalia/{pos}.
//
// Owner updates the text of a marginalia at a given position.
// Body: { text }
func UpdateMarginalia(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetOwner(w, r, s.Q)
		if !ok {
			return
		}

		pos, err := strconv.ParseInt(chi.URLParam(r, "pos"), 10, 16)
		if err != nil || pos < 1 || pos > 4 {
			respondErr(w, http.StatusBadRequest, "invalid position: must be 1–4")
			return
		}

		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Text = strings.TrimSpace(body.Text)
		if body.Text == "" {
			respondErr(w, http.StatusBadRequest, "text is required")
			return
		}

		ctx := r.Context()

		existing, _ := s.Q.ListMarginaliaByAsset(ctx, asset.ID)
		m := marginaliaByPosition(existing, int16(pos))
		if m == nil {
			respondErr(w, http.StatusNotFound, "no marginalia at this position")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		if err := s.Q.UpdateMarginaliaText(ctx, dbgen.UpdateMarginaliaTextParams{
			ID:   m.ID,
			Text: body.Text,
		}); err != nil {
			respondInternalErr(w, r, "could not update marginalia", err)
			return
		}
		m.Text = body.Text

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaUpdated, model.MarginaliaPayload{
				AssetID:    asset.ID,
				Marginalia: *m,
			})
		}
		if g, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
			EmitMarginaliaEdited(ctx, s.Q, manager, asset.GameID, *asset, *m, body.Text, player.ID, g.CurrentRow)
		}

		respond(w, http.StatusOK, map[string]any{"marginalia": m})
	}
}

// TearMarginalia handles DELETE /api/assets/{assetId}/marginalia/{pos}.
//
// Any game member can tear (break) a marginalia. If all 4 are torn the asset
// is destroyed.
func TearMarginalia(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetAccess(w, r, s.Q)
		if !ok {
			return
		}

		pos, err := strconv.ParseInt(chi.URLParam(r, "pos"), 10, 16)
		if err != nil || pos < 1 || pos > 4 {
			respondErr(w, http.StatusBadRequest, "invalid position: must be 1–4")
			return
		}

		ctx := r.Context()

		existing, _ := s.Q.ListMarginaliaByAsset(ctx, asset.ID)
		m := marginaliaByPosition(existing, int16(pos))
		if m == nil {
			respondErr(w, http.StatusNotFound, "no marginalia at this position")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		if _, err := s.Q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
			ID:       m.ID,
			TornByID: &player.ID,
		}); err != nil {
			respondInternalErr(w, r, "could not tear marginalia", err)
			return
		}

		// Snapshot model: tearing the asset reveals its current secrets to the
		// tearing player. The grant is idempotent (no-op if already visible).
		_ = s.Q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
			AssetID:  asset.ID,
			PlayerID: player.ID,
		})

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
				AssetID:  asset.ID,
				Position: int16(pos),
				TornByID: player.ID,
			})
			h.BroadcastEvent(model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
				AssetID:  asset.ID,
				PlayerID: player.ID,
			})
		}
		// Check if that was the last intact marginalia → destroy the asset.
		// DestroyIfAllMarginaliaTorn composes the "no intact remain" check
		// and the flip into a single SQL statement; rows=1 means the tear
		// just completed the destruction. Computed before the torn post so its
		// "how has the asset changed?" prompt is suppressed on a destroy.
		destroyedRows, _ := s.Q.DestroyIfAllMarginaliaTorn(ctx, asset.ID)
		destroyed := destroyedRows > 0

		if g, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
			EmitMarginaliaTorn(ctx, s.Q, manager, asset.GameID, *asset, *m, player.ID, destroyed, g.CurrentRow)
		}

		if destroyed {
			if h, ok := manager.Get(asset.GameID); ok {
				h.BroadcastEvent(model.EventAssetDestroyed, model.AssetIDPayload{
					AssetID: asset.ID,
				})
			}
			if game, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
				EmitAssetDestroyed(ctx, s.Q, manager, asset.GameID, *asset, game.CurrentRow)
			}
			respond(w, http.StatusOK, map[string]any{"torn": true, "destroyed": true})
			return
		}

		respond(w, http.StatusOK, map[string]any{"torn": true, "destroyed": false})
	}
}
