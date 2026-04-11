package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	appMiddleware "uneasy/middleware"
	"uneasy/model"
)

// ── Shared helpers ────────────────────────────────────────────────────────────

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
	player := appMiddleware.PlayerFromContext(r.Context())
	if player == nil || player.GameID != asset.GameID {
		respondErr(w, http.StatusForbidden, "not a member of this table")
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
func ListAssets(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}

		assets, err := q.ListAssetsByGame(r.Context(), gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load assets")
			return
		}

		result := make([]assetWithMarginalia, 0, len(assets))
		for _, a := range assets {
			marginalia, _ := q.ListMarginaliaByAsset(r.Context(), a.ID)
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
// Creates an asset and optional initial marginalia in one call.
// Body: { asset_type, name, is_main_character?, marginalia?: ["text",...] }
func CreateAsset(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r)
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

		if len(body.Marginalia) > 4 {
			respondErr(w, http.StatusBadRequest, "at most 4 marginalia")
			return
		}

		ctx := r.Context()

		// If setting as main character, clear existing main character first.
		if body.IsMainCharacter {
			if err := q.ClearMainCharacter(ctx, dbgen.ClearMainCharacterParams{
				OwnerID: player.ID,
				GameID:  gameID,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not clear main character")
				return
			}
		}

		asset, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          gameID,
			OwnerID:         player.ID,
			CreatorID:       player.ID,
			AssetType:       assetType,
			Name:            body.Name,
			IsMainCharacter: body.IsMainCharacter,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create asset")
			return
		}

		// Create any initial marginalia.
		marginalia := make([]dbgen.Marginalium, 0, len(body.Marginalia))
		for i, text := range body.Marginalia {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
				AssetID:  asset.ID,
				Position: int16(i + 1),
				Text:     text,
			})
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not create marginalia")
				return
			}
			marginalia = append(marginalia, m)
		}

		result := assetWithMarginalia{Asset: asset, Marginalia: marginalia}

		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventAssetCreated, model.AssetPayload{Asset: result})
		}

		respond(w, http.StatusCreated, map[string]any{"asset": result})
	}
}

// UpdateAsset handles PUT /api/assets/{assetId}.
//
// Owner can update the asset name and/or main-character flag.
// Body: { name?, is_main_character? }
func UpdateAsset(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetOwner(w, r, q)
		if !ok {
			return
		}

		var body struct {
			Name            *string `json:"name"`
			IsMainCharacter *bool   `json:"is_main_character"`
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
			if err := q.UpdateAssetName(ctx, dbgen.UpdateAssetNameParams{
				ID:   asset.ID,
				Name: name,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not update name")
				return
			}
			asset.Name = name
		}

		if body.IsMainCharacter != nil {
			if *body.IsMainCharacter {
				// Only peers can be main characters.
				if asset.AssetType != model.AssetPeer {
					respondErr(w, http.StatusBadRequest, "only peer assets can be the main character")
					return
				}
				if err := q.ClearMainCharacter(ctx, dbgen.ClearMainCharacterParams{
					OwnerID: player.ID,
					GameID:  asset.GameID,
				}); err != nil {
					respondErr(w, http.StatusInternalServerError, "could not clear main character")
					return
				}
			}
			if err := q.SetMainCharacter(ctx, dbgen.SetMainCharacterParams{
				ID:              asset.ID,
				IsMainCharacter: *body.IsMainCharacter,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not update main character")
				return
			}
			asset.IsMainCharacter = *body.IsMainCharacter
		}

		enriched, err := loadAssetEnriched(r, q, asset.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not reload asset")
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetUpdated, model.AssetPayload{Asset: enriched})
		}

		respond(w, http.StatusOK, map[string]any{"asset": enriched})
	}
}

// ── Marginalia handlers ───────────────────────────────────────────────────────

// AddMarginalia handles POST /api/assets/{assetId}/marginalia.
//
// Owner adds a marginalia note to their asset (max 4 total).
// Body: { text }
func AddMarginalia(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, _, ok := requireAssetOwner(w, r, q)
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

		existing, err := q.ListMarginaliaByAsset(ctx, asset.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check marginalia")
			return
		}
		if int64(len(existing)) >= 4 {
			respondErr(w, http.StatusBadRequest, "asset already has 4 marginalia")
			return
		}

		// Find next available position (1-4), skipping occupied slots.
		occupied := map[int16]bool{}
		for _, m := range existing {
			occupied[m.Position] = true
		}
		var nextPos int16
		for p := int16(1); p <= 4; p++ {
			if !occupied[p] {
				nextPos = p
				break
			}
		}

		m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID:  asset.ID,
			Position: nextPos,
			Text:     body.Text,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not add marginalia")
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaAdded, model.MarginaliaPayload{
				AssetID:    asset.ID,
				Marginalia: m,
			})
		}

		respond(w, http.StatusCreated, map[string]any{"marginalia": m})
	}
}

// UpdateMarginalia handles PUT /api/assets/{assetId}/marginalia/{pos}.
//
// Owner updates the text of a marginalia at a given position.
// Body: { text }
func UpdateMarginalia(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, _, ok := requireAssetOwner(w, r, q)
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

		existing, _ := q.ListMarginaliaByAsset(ctx, asset.ID)
		m := marginaliaByPosition(existing, int16(pos))
		if m == nil {
			respondErr(w, http.StatusNotFound, "no marginalia at this position")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		if err := q.UpdateMarginaliaText(ctx, dbgen.UpdateMarginaliaTextParams{
			ID:   m.ID,
			Text: body.Text,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not update marginalia")
			return
		}
		m.Text = body.Text

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaUpdated, model.MarginaliaPayload{
				AssetID:    asset.ID,
				Marginalia: *m,
			})
		}

		respond(w, http.StatusOK, map[string]any{"marginalia": m})
	}
}

// TearMarginalia handles DELETE /api/assets/{assetId}/marginalia/{pos}.
//
// Any game member can tear (break) a marginalia. If all 4 are torn the asset
// is destroyed.
func TearMarginalia(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetAccess(w, r, q)
		if !ok {
			return
		}

		pos, err := strconv.ParseInt(chi.URLParam(r, "pos"), 10, 16)
		if err != nil || pos < 1 || pos > 4 {
			respondErr(w, http.StatusBadRequest, "invalid position: must be 1–4")
			return
		}

		ctx := r.Context()

		existing, _ := q.ListMarginaliaByAsset(ctx, asset.ID)
		m := marginaliaByPosition(existing, int16(pos))
		if m == nil {
			respondErr(w, http.StatusNotFound, "no marginalia at this position")
			return
		}
		if m.IsTorn {
			respondErr(w, http.StatusConflict, "marginalia is already torn")
			return
		}

		if err := q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
			ID:       m.ID,
			TornByID: &player.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not tear marginalia")
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventMarginaliaTorn, model.MarginaliaTornPayload{
				AssetID:  asset.ID,
				Position: int16(pos),
				TornByID: player.ID,
			})
		}

		// Check if all marginalia are now torn → destroy the asset.
		intact, _ := q.CountIntactMarginalia(ctx, asset.ID)
		if intact == 0 {
			totalCount, _ := q.CountMarginalia(ctx, asset.ID)
			if totalCount > 0 {
				_ = q.DestroyAsset(ctx, asset.ID)
				if h, ok := manager.Get(asset.GameID); ok {
					h.BroadcastEvent(model.EventAssetDestroyed, model.AssetIDPayload{
						AssetID: asset.ID,
					})
				}
				respond(w, http.StatusOK, map[string]any{"torn": true, "destroyed": true})
				return
			}
		}

		respond(w, http.StatusOK, map[string]any{"torn": true, "destroyed": false})
	}
}

// ── Leverage / Refresh / Take ─────────────────────────────────────────────────

// LeverageAsset handles POST /api/assets/{assetId}/leverage.
//
// Owner marks an asset as leveraged (committed to a dice roll).
func LeverageAsset(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetOwner(w, r, q)
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

		if err := q.SetAssetLeveraged(r.Context(), dbgen.SetAssetLeveragedParams{
			ID:          asset.ID,
			IsLeveraged: true,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not leverage asset")
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetLeveraged, model.AssetIDPayload{
				AssetID:  asset.ID,
				PlayerID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{"leveraged": true})
	}
}

// RefreshAsset handles POST /api/assets/{assetId}/refresh.
//
// Owner un-leverages an asset.
func RefreshAsset(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, _, ok := requireAssetOwner(w, r, q)
		if !ok {
			return
		}
		if !asset.IsLeveraged {
			respondErr(w, http.StatusConflict, "asset is not leveraged")
			return
		}

		if err := q.SetAssetLeveraged(r.Context(), dbgen.SetAssetLeveragedParams{
			ID:          asset.ID,
			IsLeveraged: false,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not refresh asset")
			return
		}

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetRefreshed, model.AssetIDPayload{
				AssetID: asset.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{"leveraged": false})
	}
}

// TakeAsset handles POST /api/assets/{assetId}/take.
//
// Any game member can take an asset from another player (used during plan
// resolution). Grants the caller visibility on all existing secrets.
func TakeAsset(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetAccess(w, r, q)
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

		if err := q.TransferAsset(ctx, dbgen.TransferAssetParams{
			ID:      asset.ID,
			OwnerID: player.ID,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not take asset")
			return
		}

		// Grant the new owner visibility on all existing secrets.
		_ = q.GrantSecretVisibilityForAsset(ctx, dbgen.GrantSecretVisibilityForAssetParams{
			AssetID:  asset.ID,
			PlayerID: player.ID,
		})

		asset.OwnerID = player.ID
		enriched, _ := loadAssetEnriched(r, q, asset.ID)

		if h, ok := manager.Get(asset.GameID); ok {
			h.BroadcastEvent(model.EventAssetTaken, model.AssetTakenPayload{
				Asset:      enriched,
				OldOwnerID: oldOwnerID,
				NewOwnerID: player.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{"asset": enriched})
	}
}

// ── Secrets handlers ──────────────────────────────────────────────────────────

// WriteSecret handles POST /api/assets/{assetId}/secrets.
//
// Any game member can write a secret on an asset's underside.
// Body: { text }
func WriteSecret(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetAccess(w, r, q)
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

		secret, err := q.CreateSecret(r.Context(), dbgen.CreateSecretParams{
			AssetID:  asset.ID,
			AuthorID: player.ID,
			Text:     body.Text,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not write secret")
			return
		}

		respond(w, http.StatusCreated, map[string]any{"secret": secret})
	}
}

// GetSecrets handles GET /api/assets/{assetId}/secrets.
//
// Returns secrets the caller is allowed to see: those they authored, or those
// they've been explicitly granted visibility on.
func GetSecrets(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, player, ok := requireAssetAccess(w, r, q)
		if !ok {
			return
		}

		secrets, err := q.ListVisibleSecrets(r.Context(), dbgen.ListVisibleSecretsParams{
			AssetID:  asset.ID,
			PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load secrets")
			return
		}
		if secrets == nil {
			secrets = []dbgen.Secret{}
		}

		respond(w, http.StatusOK, map[string]any{"secrets": secrets})
	}
}
