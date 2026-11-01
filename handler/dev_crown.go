package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// DevCrown handles POST /api/dev/crown.
//
// Seats a monarch at a seeded table without playing the Prologue's title claim,
// by doing what the claim does: a new peer owned by the player, bearing a
// marginalia stamped with the monarch title, and the game's throne_established
// gate tripped. The new peer is broadcast and the crowning posted to the log, so
// open tabs show the crown live. The Propose Decree specs use it to test a table
// with a reigning monarch.
//
// Body: { "game_id": N, "player_id": N }. Mounted only when UNEASY_DEV=1.
func DevCrown(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	type request struct {
		GameID   *int64 `json:"game_id"`
		PlayerID *int64 `json:"player_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.GameID == nil || body.PlayerID == nil {
			respondErr(w, http.StatusBadRequest, "game_id and player_id required")
			return
		}

		ctx := r.Context()
		game, err := s.Q.GetGameByID(ctx, *body.GameID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "game not found")
			return
		}
		player, err := s.Q.GetPlayerByID(ctx, *body.PlayerID)
		if err != nil || player.GameID != game.ID {
			respondErr(w, http.StatusNotFound, "player not found in this game")
			return
		}

		const title = "The Crown"
		var asset dbgen.Asset
		var marg dbgen.Marginalium
		titleID := gamepkg.TitleMonarch
		err = s.InTx(ctx, func(q *dbgen.Queries) error {
			var txErr error
			asset, txErr = q.CreateAsset(ctx, dbgen.CreateAssetParams{
				GameID: game.ID, OwnerID: player.ID, CreatorID: player.ID,
				AssetType: model.AssetPeer, Name: title,
			})
			if txErr != nil {
				return txErr
			}
			marg, txErr = q.CreateTitleMarginalia(ctx, dbgen.CreateTitleMarginaliaParams{
				AssetID: asset.ID, Position: 1, Text: "The Monarch", Title: &titleID,
			})
			if txErr != nil {
				return txErr
			}
			return q.EstablishThrone(ctx, game.ID)
		})
		if err != nil {
			respondInternalErr(w, r, "could not crown the player", err)
			return
		}

		broadcastEvent(manager, game.ID, model.EventAssetCreated,
			model.AssetPayload{Asset: assetWithMarginalia{Asset: asset, Marginalia: []dbgen.Marginalium{marg}}})
		EmitSystemPost(ctx, s.Q, manager, game.ID, "dev.crown", model.SeverityDefault,
			fmt.Sprintf("%s was crowned (dev shortcut).", playerDisplayName(ctx, s.Q, player.ID)),
			logRow(game), nil, nil, map[string]any{"player_id": player.ID})

		respond(w, http.StatusOK, map[string]any{"asset_id": asset.ID})
	}
}
