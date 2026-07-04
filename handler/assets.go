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
	// SecretCount is the total number of secrets on the asset (existence),
	// public to every player. The content stays gated by secret_visibility;
	// clients derive the "known to me" count from the visible-secrets list and
	// treat the remainder (SecretCount − known) as hidden. Newly-created assets
	// have none, so the zero value is correct wherever it isn't set.
	SecretCount int64 `json:"secret_count"`
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
	// Total secret count (existence) — public; tolerate errors as zero.
	secretCount, _ := q.CountSecretsByAsset(r.Context(), assetID)
	return assetWithMarginalia{Asset: asset, Marginalia: marginalia, SecretCount: secretCount}, nil
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

// requireOneMarginalia trims and filters raw marginalia strings, requiring
// exactly one non-empty entry to remain. Every player-authored asset-creation
// route shares this validator so the error wording stays uniform.
func requireOneMarginalia(raw []string) (string, error) {
	var found []string
	for _, m := range raw {
		if m = strings.TrimSpace(m); m != "" {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		return "", errors.New("a new asset needs a name and one marginalia")
	}
	return found[0], nil
}

// createAssetWithFirstMarginalia creates an asset and its first marginalia
// (position 1) atomically. Takes q so callers choose their own transaction —
// plan routes have deps.Store.InTx (PlanDeps embeds *db.Store); use it where
// the route isn't already inside one.
func createAssetWithFirstMarginalia(
	ctx context.Context, q *dbgen.Queries, params dbgen.CreateAssetParams, margText string,
) (dbgen.Asset, []dbgen.Marginalium, error) {
	asset, err := q.CreateAsset(ctx, params)
	if err != nil {
		return dbgen.Asset{}, nil, fmt.Errorf("could not create asset: %w", err)
	}
	m, err := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
		AssetID:  asset.ID,
		Position: 1,
		Text:     margText,
	})
	if err != nil {
		return dbgen.Asset{}, nil, fmt.Errorf("could not create marginalia: %w", err)
	}
	return asset, []dbgen.Marginalium{m}, nil
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

		// Display path only: include destroyed assets so the retinue can
		// render them as read-only "tombstone" cards. Gameplay logic must
		// never use this query — it stays on the filtered ListAssetsByGame /
		// ListAssetsByOwner so destroyed assets never leak into mechanics.
		assets, err := s.Q.ListAllAssetsByGame(r.Context(), gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load assets", err)
			return
		}

		// Total secret count per asset (existence), in one query. Content stays
		// gated by secret_visibility; only the count is public.
		secretCounts, _ := s.Q.CountSecretsByGame(r.Context(), gameID)
		secretCountByAsset := make(map[int64]int64, len(secretCounts))
		for _, c := range secretCounts {
			secretCountByAsset[c.AssetID] = c.SecretCount
		}

		result := make([]assetWithMarginalia, 0, len(assets))
		for _, a := range assets {
			marginalia, _ := s.Q.ListMarginaliaByAsset(r.Context(), a.ID)
			if marginalia == nil {
				marginalia = []dbgen.Marginalium{}
			}
			result = append(result, assetWithMarginalia{
				Asset:       a,
				Marginalia:  marginalia,
				SecretCount: secretCountByAsset[a.ID],
			})
		}

		respond(w, http.StatusOK, map[string]any{"assets": result})
	}
}

// CreateAsset handles POST /api/tables/{id}/assets.
//
// Creates an asset and its one required initial marginalia in one call.
// Always owned by the caller. Plan-gained assets (e.g. Make Introductions
// peers) go through their plan handler's own peer-creation route, which
// routes ownership through AssetRecipientForPlan so demand keep_assets
// winners are honored.
// Body: { asset_type, name, is_main_character?, marginalia: ["text"] }
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

		margText, err := requireOneMarginalia(body.Marginalia)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()

		var asset dbgen.Asset
		var marginalia []dbgen.Marginalium
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			if body.IsMainCharacter {
				if cErr := q.ClearMainCharacter(ctx, dbgen.ClearMainCharacterParams{
					OwnerID: player.ID,
					GameID:  gameID,
				}); cErr != nil {
					return errors.New("could not clear main character")
				}
			}

			var caErr error
			asset, marginalia, caErr = createAssetWithFirstMarginalia(ctx, q, dbgen.CreateAssetParams{
				GameID:          gameID,
				OwnerID:         player.ID,
				CreatorID:       player.ID,
				AssetType:       assetType,
				Name:            body.Name,
				IsMainCharacter: body.IsMainCharacter,
			}, margText)
			return caErr
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
			EmitAssetCreated(ctx, s.Q, manager, gameID, asset, marginalia, &g.CurrentRow)
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
// at an untorn marginalia on the old MC. If the old MC has no untorn
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
			if *body.IsMainCharacter {
				// A stale scene_peers row from the old main character (if the
				// player was a scene participant) would otherwise silently reject
				// the new one as "not in the current scene" — see
				// repointScenePeerToNewMainCharacter.
				repointScenePeerToNewMainCharacter(ctx, s.Q, manager, asset.GameID, player.ID, asset.ID)
			}
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

		// A main-character change can satisfy a pending replacement-choice gate
		// (the player who lost their MC just promoted a peer). Recompute so the
		// gate clears for the whole table.
		if body.IsMainCharacter != nil {
			broadcastRowState(r.Context(), s.Q, manager, asset.GameID)
		}

		respond(w, http.StatusOK, map[string]any{"asset": enriched})
	}
}

// ReplaceMainCharacterWithNewPeer handles
// POST /api/tables/{id}/replace-main-character.
//
// The "no peers left" escape hatch for the replacement-main-character gate
// (RowStateAwaitMainCharacterChoice): when a player has lost their main
// character and has no remaining peer to promote, they conscript a brand new
// peer to be their main character. The custom-rule cost is that ALL of their
// assets — including the new peer — become leveraged.
//
// Guarded to that exact situation: rejected if the caller still has a main
// character (nothing to replace) or still owns a non-destroyed peer (they must
// promote it via PUT /assets/{id} instead, which is free).
// Body: { name, marginalia: ["text", ...] }  (>= 1 marginalia, per the
// asset-creation rule).
func ReplaceMainCharacterWithNewPeer(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		name, margs, ok := parseConscriptBody(w, r)
		if !ok {
			return
		}

		ctx := r.Context()
		owned, err := s.Q.ListAssetsByOwner(ctx, player.ID)
		if err != nil {
			respondInternalErr(w, r, "could not list owner assets", err)
			return
		}
		if !ensureOwesConscription(w, owned, gameID) {
			return
		}

		asset, marginalia, err := createConscriptedMainCharacter(ctx, s, gameID, player.ID, name, margs)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		broadcastConscription(r, s.Q, manager, gameID, player.ID, asset, marginalia, owned)
		// See UpdateAsset's promotion path — a stale scene_peers row from the
		// destroyed main character would otherwise silently reject the new one.
		repointScenePeerToNewMainCharacter(ctx, s.Q, manager, gameID, player.ID, asset.ID)
		// Clears the replacement-choice gate now that the player has an MC again.
		broadcastRowState(ctx, s.Q, manager, gameID)

		enriched, _ := loadAssetEnriched(r, s.Q, asset.ID)
		respond(w, http.StatusCreated, map[string]any{"asset": enriched})
	}
}

// parseConscriptBody decodes and validates the conscript request body, writing
// the error response itself on failure. Requires a name and at least one
// marginalia (per the asset-creation rule), capped at maxMarginalia. Extra
// marginalia beyond the first can always be added from the retinue afterward.
func parseConscriptBody(w http.ResponseWriter, r *http.Request) (string, []string, bool) {
	var body struct {
		Name       string   `json:"name"`
		Marginalia []string `json:"marginalia"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid JSON")
		return "", nil, false
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		respondErr(w, http.StatusBadRequest, "name is required")
		return "", nil, false
	}
	margs := make([]string, 0, len(body.Marginalia))
	for _, m := range body.Marginalia {
		if m = strings.TrimSpace(m); m != "" {
			margs = append(margs, m)
		}
	}
	if len(margs) < 1 {
		respondErr(w, http.StatusBadRequest, "a new character needs a name and one marginalia")
		return "", nil, false
	}
	if len(margs) > maxMarginalia {
		respondErr(w, http.StatusBadRequest, fmt.Sprintf("at most %d marginalia", maxMarginalia))
		return "", nil, false
	}
	return name, margs, true
}

// ensureOwesConscription guards the conscript route to its exact situation: the
// caller has no main character (nothing to replace ⇒ reject) and no
// non-destroyed peer (a peer ⇒ they must promote it, which is free). Writes the
// 409 response itself; returns false when the route does not apply.
func ensureOwesConscription(w http.ResponseWriter, owned []dbgen.Asset, gameID int64) bool {
	for i := range owned {
		a := &owned[i]
		if a.GameID != gameID || a.IsDestroyed {
			continue
		}
		if a.IsMainCharacter {
			respondErr(w, http.StatusConflict, "you already have a main character")
			return false
		}
		if a.AssetType == model.AssetPeer {
			respondErr(w, http.StatusConflict, "promote one of your existing peers instead")
			return false
		}
	}
	return true
}

// createConscriptedMainCharacter creates the new peer (flagged main character)
// with its marginalia and applies the custom-rule cost — every asset the player
// owns becomes leveraged — atomically in one transaction.
func createConscriptedMainCharacter(
	ctx context.Context, s *db.Store, gameID, ownerID int64, name string, margs []string,
) (dbgen.Asset, []dbgen.Marginalium, error) {
	var asset dbgen.Asset
	var marginalia []dbgen.Marginalium
	err := s.InTx(ctx, func(q *dbgen.Queries) error {
		var caErr error
		asset, caErr = q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          gameID,
			OwnerID:         ownerID,
			CreatorID:       ownerID,
			AssetType:       model.AssetPeer,
			Name:            name,
			IsMainCharacter: true,
		})
		if caErr != nil {
			return errors.New("could not create main character")
		}
		marginalia = make([]dbgen.Marginalium, 0, len(margs))
		for i, text := range margs {
			m, mErr := q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
				AssetID: asset.ID, Position: int16(i + 1), Text: text,
			})
			if mErr != nil {
				return errors.New("could not create marginalia")
			}
			marginalia = append(marginalia, m)
		}
		if lErr := q.LeverageAllPlayerAssets(ctx, dbgen.LeverageAllPlayerAssetsParams{
			OwnerID: ownerID, GameID: gameID,
		}); lErr != nil {
			return errors.New("could not leverage assets")
		}
		return nil
	})
	return asset, marginalia, err
}

// broadcastConscription announces the new main character, re-pushes the player's
// now-leveraged assets (a full per-asset reload keeps the merge-not-replace
// contract — see the asset.taken payload note), and logs the conscription.
// `owned` is the pre-conscription asset list (their existing assets).
func broadcastConscription(
	r *http.Request, q *dbgen.Queries, manager *hub.Manager,
	gameID, ownerID int64, asset dbgen.Asset, marginalia []dbgen.Marginalium, owned []dbgen.Asset,
) {
	ctx := r.Context()
	if h, ok := manager.Get(gameID); ok {
		if e, err := loadAssetEnriched(r, q, asset.ID); err == nil {
			h.BroadcastEvent(model.EventAssetCreated, model.AssetPayload{Asset: e})
		}
		for i := range owned {
			a := &owned[i]
			if a.GameID != gameID || a.IsDestroyed {
				continue
			}
			if e, err := loadAssetEnriched(r, q, a.ID); err == nil {
				h.BroadcastEvent(model.EventAssetUpdated, model.AssetPayload{Asset: e})
			}
		}
	}
	if g, err := q.GetGameByID(ctx, gameID); err == nil {
		EmitAssetCreated(ctx, q, manager, gameID, asset, marginalia, &g.CurrentRow)
		EmitMainCharacterConscripted(ctx, q, manager, gameID, asset, ownerID, g.CurrentRow)
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
			EmitAssetTaken(ctx, s.Q, manager, asset.GameID, *asset, oldOwnerID, player.ID, &g.CurrentRow)
		}

		// Taking a main character clears its flag (TransferAsset), leaving the
		// previous owner without one. Recompute so the table surfaces the
		// replacement-choice gate (RowStateAwaitMainCharacterChoice) live.
		broadcastRowState(ctx, s.Q, manager, asset.GameID)

		respond(w, http.StatusOK, map[string]any{"asset": enriched})
	}
}
