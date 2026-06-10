package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── Shared helpers ────────────────────────────────────────────────────────────

const maxMarginalia = 4

// assetWithMarginalia is the enriched response type for asset API calls.
// It embeds the base asset and adds the marginalia slice inline.
type assetWithMarginalia struct {
	dbgen.Asset

	Marginalia []dbgen.Marginalium `json:"marginalia"`
}

// loadAssetEnriched fetches an asset and its marginalia in two queries.
func loadAssetEnriched(r *http.Request, q *dbgen.Queries, assetID int64) (assetWithMarginalia, error) {
	asset, err := q.GetAssetByID(r.Context(), assetID)
	if err != nil {
		return assetWithMarginalia{}, err
	}
	marginalia, err := q.ListMarginaliaByAsset(r.Context(), assetID)
	if err != nil || marginalia == nil {
		marginalia = []dbgen.Marginalium{}
	}
	return assetWithMarginalia{Asset: asset, Marginalia: marginalia}, nil
}

// requireAssetAccess validates the assetId URL param, loads the asset, and
// confirms the caller is a member of that game. Returns the asset and player.
func requireAssetAccess(w http.ResponseWriter, r *http.Request, q *dbgen.Queries) (*dbgen.Asset, *dbgen.Player, bool) {
	assetID, err := strconv.ParseInt(chi.URLParam(r, "assetId"), 10, 64)
	if err != nil {
		respondErr(w, http.StatusBadRequest, "invalid asset id")
		return nil, nil, false
	}
	asset, err := q.GetAssetByID(r.Context(), assetID)
	if err != nil {
		respondErr(w, http.StatusNotFound, "asset not found")
		return nil, nil, false
	}
	player, ok := requirePlayerInGame(w, r, q, asset.GameID)
	if !ok {
		return nil, nil, false
	}
	return &asset, player, true
}

// requireAssetOwner is like requireAssetAccess but also checks ownership.
func requireAssetOwner(w http.ResponseWriter, r *http.Request, q *dbgen.Queries) (*dbgen.Asset, *dbgen.Player, bool) {
	asset, player, ok := requireAssetAccess(w, r, q)
	if !ok {
		return nil, nil, false
	}
	if asset.OwnerID != player.ID {
		respondErr(w, http.StatusForbidden, "you do not own this asset")
		return nil, nil, false
	}
	return asset, player, true
}

// marginaliaByPosition scans a marginalia list and returns the entry at the
// given position, or nil if not found.
func marginaliaByPosition(list []dbgen.Marginalium, pos int16) *dbgen.Marginalium {
	for i := range list {
		if list[i].Position == pos {
			return &list[i]
		}
	}
	return nil
}

// ── Asset handlers ────────────────────────────────────────────────────────────

// ListAssets handles GET /api/tables/{id}/assets.
//
// Returns all non-destroyed assets in the game, each with their marginalia.
func ListAssets(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		assets, err := s.Q.ListAssetsByGame(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load assets", err)
			return
		}

		result := make([]assetWithMarginalia, 0, len(assets))
		for _, a := range assets {
			marginalia, _ := s.Q.ListMarginaliaByAsset(r.Context(), a.ID)
			if marginalia == nil {
				marginalia = []dbgen.Marginalium{}
			}
			result = append(result, assetWithMarginalia{Asset: a, Marginalia: marginalia})
		}

		respond(w, http.StatusOK, map[string]any{"assets": result})
	}
}

// CreateAsset handles POST /api/tables/{id}/assets.
//
// Creates an asset and optional initial marginalia in one call. Always
// owned by the caller. Plan-gained assets (e.g. Make Introductions peers)
// go through their plan handler's own peer-creation route, which routes
// ownership through AssetRecipientForPlan so demand keep_assets
// winners are honored.
// Body: { asset_type, name, is_main_character?, marginalia?: ["text",...] }
func CreateAsset(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		var body struct {
			AssetType       string   `json:"asset_type"`
			Name            string   `json:"name"`
			IsMainCharacter bool     `json:"is_main_character"`
			Marginalia      []string `json:"marginalia"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			respondErr(w, http.StatusBadRequest, "name is required")
			return
		}

		assetType := model.AssetType(body.AssetType)
		switch assetType {
		case model.AssetPeer, model.AssetHolding, model.AssetArtifact, model.AssetResource:
			// valid
		default:
			respondErr(w, http.StatusBadRequest, "invalid asset_type")
			return
		}

		if len(body.Marginalia) > maxMarginalia {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("at most %d marginalia", maxMarginalia))
			return
		}

		ctx := r.Context()

		var asset dbgen.Asset
		var marginalia []dbgen.Marginalium
		err := s.InTx(ctx, func(q *dbgen.Queries) error {
			if body.IsMainCharacter {
				if cErr := q.ClearMainCharacter(ctx, dbgen.ClearMainCharacterParams{
					OwnerID: player.ID,
					GameID:  gameID,
				}); cErr != nil {
					return errors.New("could not clear main character")
				}
			}

			var caErr error
			asset, caErr = q.CreateAsset(ctx, dbgen.CreateAssetParams{
				GameID:          gameID,
				OwnerID:         player.ID,
				CreatorID:       player.ID,
				AssetType:       assetType,
				Name:            body.Name,
				IsMainCharacter: body.IsMainCharacter,
			})
			if caErr != nil {
				return errors.New("could not create asset")
			}

			marginalia = make([]dbgen.Marginalium, 0, len(body.Marginalia))
			for i, text := range body.Marginalia {
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				m, mErr := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
					AssetID:  asset.ID,
					Position: int16(i + 1),
					Text:     text,
				})
				if mErr != nil {
					return errors.New("could not create marginalia")
				}
				marginalia = append(marginalia, m)
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		result := assetWithMarginalia{Asset: asset, Marginalia: marginalia}

		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventAssetCreated, model.AssetPayload{Asset: result})
		}
		if g, err := s.Q.GetGameByID(ctx, gameID); err == nil {
			EmitAssetCreated(ctx, s.Q, manager, gameID, asset, marginalia, g.CurrentRow)
		}

		respond(w, http.StatusCreated, map[string]any{"asset": result})
	}
}

// UpdateAsset handles PUT /api/assets/{assetId}.
//
// Owner can update the asset name and/or main-character flag.
// Body: { name?, is_main_character?, tear_position? }
//
// When promoting a peer to main character and an existing main character
// already exists for this player, the rules require tearing one of the
// existing MC's marginalia. Callers must pass `tear_position` (1–4) pointing
// at an untorn marginalium on the old MC. If the old MC has no untorn
// marginalia (e.g. all 4 already torn), the swap proceeds without tearing.
func UpdateAsset(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetOwner(w, r, s.Q)
		if !ok {
			return
		}

		var body struct {
			Name            *string `json:"name"`
			IsMainCharacter *bool   `json:"is_main_character"`
			TearPosition    *int16  `json:"tear_position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		if body.Name != nil {
			name := strings.TrimSpace(*body.Name)
			if name == "" {
				respondErr(w, http.StatusBadRequest, "name cannot be empty")
				return
			}
			oldName := asset.Name
			err := s.Q.UpdateAssetName(ctx, dbgen.UpdateAssetNameParams{
				ID:   asset.ID,
				Name: name,
			})
			if err != nil {
				respondInternalErr(w, r, "could not update name", err)
				return
			}
			asset.Name = name
			if name != oldName {
				if g, gErr := s.Q.GetGameByID(ctx, asset.GameID); gErr == nil {
					EmitAssetRenamed(ctx, s.Q, manager, asset.GameID, *asset, oldName, name, player.ID, g.CurrentRow)
				}
			}
		}

		if body.IsMainCharacter != nil {
			if !applyMainCharacterChange(ctx, w, r, s.Q, manager, asset, player,
				*body.IsMainCharacter, body.TearPosition) {
				return
			}
			asset.IsMainCharacter = *body.IsMainCharacter
			if g, gErr := s.Q.GetGameByID(ctx, asset.GameID); gErr == nil {
				EmitMainCharacterChanged(
					ctx,
					s.Q,
					manager,
					asset.GameID,
					*asset,
					*body.IsMainCharacter,
					player.ID,
					g.CurrentRow,
				)
			}
		}

		enriched, err := loadAssetEnriched(r, s.Q, asset.ID)
		if err != nil {
			respondInternalErr(w, r, "could not reload asset", err)
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetUpdated, model.AssetPayload{Asset: enriched})
		}

		respond(w, http.StatusOK, map[string]any{"asset": enriched})
	}
}

// tearOldMainCharacterMarginalia tears the marginalium the MC swap requires
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
		EmitMarginaliaTorn(ctx, q, manager, oldMC.GameID, *oldMC, *target, player.ID, g.CurrentRow)
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
			EmitMarginaliaAdded(ctx, s.Q, manager, asset.GameID, *asset, m, player.ID, g.CurrentRow)
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
		if g, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
			EmitMarginaliaTorn(ctx, s.Q, manager, asset.GameID, *asset, *m, player.ID, g.CurrentRow)
		}

		// Check if that was the last intact marginalium → destroy the asset.
		// DestroyIfAllMarginaliaTorn composes the "no intact remain" check
		// and the flip into a single SQL statement; rows=1 means the tear
		// just completed the destruction.
		destroyedRows, _ := s.Q.DestroyIfAllMarginaliaTorn(ctx, asset.ID)
		if destroyedRows > 0 {
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

// ── Leverage / Refresh / Take ─────────────────────────────────────────────────

// LeverageAsset handles POST /api/assets/{assetId}/leverage.
//
// Owner marks an asset as leveraged (committed to a dice roll).
func LeverageAsset(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetOwner(w, r, s.Q)
		if !ok {
			return
		}
		if asset.IsLeveraged {
			respondErr(w, http.StatusConflict, "asset is already leveraged")
			return
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "asset is destroyed")
			return
		}

		if err := s.Q.SetAssetLeveraged(r.Context(), dbgen.SetAssetLeveragedParams{
			ID:          asset.ID,
			IsLeveraged: true,
		}); err != nil {
			respondInternalErr(w, r, "could not leverage asset", err)
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetLeveraged, model.AssetIDPayload{
				AssetID:  asset.ID,
				PlayerID: player.ID,
			})
		}
		if game, err := s.Q.GetGameByID(r.Context(), asset.GameID); err == nil {
			EmitAssetLeveraged(r.Context(), s.Q, manager, asset.GameID, *asset, game.CurrentRow)
		}

		respond(w, http.StatusOK, map[string]any{"leveraged": true})
	}
}

// RefreshAsset handles POST /api/assets/{assetId}/refresh.
//
// Owner un-leverages an asset.
func RefreshAsset(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, _, ok := requireAssetOwner(w, r, s.Q)
		if !ok {
			return
		}
		if !asset.IsLeveraged {
			respondErr(w, http.StatusConflict, "asset is not leveraged")
			return
		}

		if err := s.Q.SetAssetLeveraged(r.Context(), dbgen.SetAssetLeveragedParams{
			ID:          asset.ID,
			IsLeveraged: false,
		}); err != nil {
			respondInternalErr(w, r, "could not refresh asset", err)
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetRefreshed, model.AssetIDPayload{
				AssetID: asset.ID,
			})
		}
		if game, err := s.Q.GetGameByID(r.Context(), asset.GameID); err == nil {
			EmitAssetRefreshed(r.Context(), s.Q, manager, asset.GameID, *asset, game.CurrentRow)
		}

		respond(w, http.StatusOK, map[string]any{"leveraged": false})
	}
}

// TakeAsset handles POST /api/assets/{assetId}/take.
//
// Any game member can take an asset from another player (used during plan
// resolution). Grants the caller visibility on all existing secrets.
func TakeAsset(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetAccess(w, r, s.Q)
		if !ok {
			return
		}
		if asset.OwnerID == player.ID {
			respondErr(w, http.StatusConflict, "you already own this asset")
			return
		}
		if asset.IsDestroyed {
			respondErr(w, http.StatusConflict, "asset is destroyed")
			return
		}

		ctx := r.Context()
		oldOwnerID := asset.OwnerID

		if err := s.Q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID:      asset.ID,
			OwnerID: player.ID,
		}); err != nil {
			respondInternalErr(w, r, "could not take asset", err)
			return
		}

		// Grant the new owner visibility on all existing secrets.
		_ = s.Q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
			AssetID:  asset.ID,
			PlayerID: player.ID,
		})

		asset.OwnerID = player.ID
		enriched, _ := loadAssetEnriched(r, s.Q, asset.ID)

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
				Asset:      enriched,
				OldOwnerID: oldOwnerID,
				NewOwnerID: player.ID,
			})
			h.BroadcastEvent(model.EventSecretVisibilityGrant, model.SecretVisibilityGrantPayload{
				AssetID:  asset.ID,
				PlayerID: player.ID,
			})
		}
		if g, err := s.Q.GetGameByID(ctx, asset.GameID); err == nil {
			EmitAssetTaken(ctx, s.Q, manager, asset.GameID, *asset, oldOwnerID, player.ID, g.CurrentRow)
		}

		respond(w, http.StatusOK, map[string]any{"asset": enriched})
	}
}

// ── Secrets handlers ──────────────────────────────────────────────────────────

// WriteSecret handles POST /api/assets/{assetId}/secrets.
//
// Only the asset owner can write a secret on its underside (per the rules:
// "choose one of your assets that's helping you keep the secret"). Visibility
// follows a snapshot model — see SECRETS_RULES.md.
// Body: { text }
func WriteSecret(s *db.Store, manager *hub.Manager) http.HandlerFunc {
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

		secret, err := s.Q.CreateSecret(r.Context(), dbgen.CreateSecretParams{
			AssetID:  asset.ID,
			AuthorID: player.ID,
			Text:     body.Text,
		})
		if err != nil {
			respondInternalErr(w, r, "could not write secret", err)
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventSecretCreated, model.SecretCreatedPayload{
				AssetID:  asset.ID,
				AuthorID: player.ID,
			})
		}

		respond(w, http.StatusCreated, map[string]any{"secret": secret})
	}
}

// GetSecrets handles GET /api/assets/{assetId}/secrets.
//
// Returns secrets the caller is allowed to see: those they authored, or those
// they've been explicitly granted visibility on.
func GetSecrets(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetAccess(w, r, s.Q)
		if !ok {
			return
		}

		secrets, err := s.Q.ListVisibleSecrets(r.Context(), dbgen.ListVisibleSecretsParams{
			AssetID:  asset.ID,
			PlayerID: player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not load secrets", err)
			return
		}
		if secrets == nil {
			secrets = []dbgen.Secret{}
		}

		respond(w, http.StatusOK, map[string]any{"secrets": secrets})
	}
}

// ListVisibleSecretsForGame handles GET /api/tables/{id}/secrets/visible.
//
// Returns every secret in the game that the caller can see. Used by the
// retinue UI to display per-asset secret counts without N requests.
func ListVisibleSecretsForGame(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}

		secrets, err := s.Q.ListVisibleSecretsByGame(r.Context(), dbgen.ListVisibleSecretsByGameParams{
			GameID:   gameID,
			PlayerID: player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not load secrets", err)
			return
		}
		if secrets == nil {
			secrets = []dbgen.Secret{}
		}

		respond(w, http.StatusOK, map[string]any{"secrets": secrets})
	}
}
