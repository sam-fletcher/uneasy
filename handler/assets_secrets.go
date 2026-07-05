package handler

import (
	"encoding/json"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

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
		text, ok := textField(w, "text", body.Text, maxNarrativeLen)
		if !ok {
			return
		}
		body.Text = text
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
