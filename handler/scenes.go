package handler

// handler/scenes.go — Main Event scene structure (SCENES_PLAN.md).
//
// A scene records where it takes place (a holding asset OR free text), how
// much in-fiction time has passed, which peers are present, and the
// follow-on prompt from the plan that just resolved (if any). The focus
// player creates the scene; non-focus players may "claim" any focus-player
// peer that wasn't already locked to its owner.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/hub"
	"uneasy/model"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// loadActiveScene returns the active scene for a game, or (nil, nil) if none.
// Errors only on real DB failures.
func loadActiveScene(ctx context.Context, q *dbgen.Queries, gameID int64) (*dbgen.Scene, error) {
	scene, err := q.GetActiveScene(ctx, gameID)
	if err != nil {
		// pgx returns a "no rows" error when there's no active scene; we
		// treat that as a non-error "no active scene" return value.
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, err
	}
	return &scene, nil
}

// validateSpeakingAs returns nil if the (player, asset) attribution is
// permitted given the active scene state, or an HTTP-friendly error message.
//
// Rules:
//   - assetID == 0 (unset) → always allowed (OOC).
//   - Asset must be the player's main character, OR a scene_peer in the
//     active scene whose controller_player_id == playerID.
func validateSpeakingAs(
	ctx context.Context,
	q *dbgen.Queries,
	gameID, playerID, assetID int64,
) (int, string) {
	if assetID == 0 {
		return 0, ""
	}

	asset, err := q.GetAssetByID(ctx, assetID)
	if err != nil {
		return http.StatusBadRequest, "speak-as asset not found"
	}
	if asset.GameID != gameID {
		return http.StatusBadRequest, "speak-as asset is not part of this game"
	}
	if asset.IsDestroyed {
		return http.StatusBadRequest, "speak-as asset is destroyed"
	}

	// Main character of the caller is always allowed, scene or no scene.
	if asset.IsMainCharacter && asset.OwnerID == playerID {
		return 0, ""
	}

	// Otherwise we need an active scene with this peer assigned to the caller.
	scene, err := loadActiveScene(ctx, q, gameID)
	if err != nil {
		return http.StatusInternalServerError, "could not load active scene"
	}
	if scene == nil {
		return http.StatusBadRequest, "no active scene for character attribution"
	}

	peer, err := q.GetScenePeer(ctx, dbgen.GetScenePeerParams{
		SceneID:     scene.ID,
		PeerAssetID: assetID,
	})
	if err != nil {
		return http.StatusBadRequest, "that character is not in the current scene"
	}
	if peer.ControllerPlayerID == nil || *peer.ControllerPlayerID != playerID {
		return http.StatusForbidden, "that character is not yours to speak as"
	}
	return 0, ""
}

// scenePeerView is the JSON shape returned alongside a scene response.
type scenePeerView struct {
	PeerAssetID        int64  `json:"peer_asset_id"`
	ControllerPlayerID *int64 `json:"controller_player_id"`
}

type sceneResponse struct {
	Scene any             `json:"scene"`
	Peers []scenePeerView `json:"peers"`
}

func buildSceneResponse(ctx context.Context, q *dbgen.Queries, scene *dbgen.Scene) (sceneResponse, error) {
	peers, err := q.ListScenePeers(ctx, scene.ID)
	if err != nil {
		return sceneResponse{}, err
	}
	views := make([]scenePeerView, 0, len(peers))
	for _, p := range peers {
		views = append(views, scenePeerView{
			PeerAssetID:        p.PeerAssetID,
			ControllerPlayerID: p.ControllerPlayerID,
		})
	}
	return sceneResponse{Scene: scene, Peers: views}, nil
}

// ── HTTP handlers ────────────────────────────────────────────────────────────

// CreateScene handles POST /api/tables/{id}/scenes.
//
// Focus player only; main_event phase only. There must be no active scene
// in the game and no pending or resolving plans on the current row. The
// server fills in `prompt` and `resolved_plan_id` based on the most
// recently resolved plan on this row (if any).
func CreateScene(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameRow, player, ok := requireFocusPlayer(w, r, q)
		if !ok {
			return
		}
		if gameRow.Phase != model.PhaseMainEvent {
			respondErr(w, http.StatusConflict, "game is not in the main event phase")
			return
		}

		var body struct {
			LocationHoldingID *int64            `json:"location_holding_id"`
			LocationCustom    string            `json:"location_custom"`
			TimeElapsed       model.TimeElapsed `json:"time_elapsed"`
			TimeNote          string            `json:"time_note"`
			PresentPeerIDs    []int64           `json:"present_peer_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Exactly one of holding / custom must be set.
		body.LocationCustom = strings.TrimSpace(body.LocationCustom)
		hasHolding := body.LocationHoldingID != nil
		hasCustom := body.LocationCustom != ""
		if hasHolding == hasCustom {
			respondErr(w, http.StatusBadRequest,
				"exactly one of location_holding_id or location_custom is required")
			return
		}
		if hasCustom && len(body.LocationCustom) > game.MaxCustomLocationLen {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("custom location must be ≤ %d characters", game.MaxCustomLocationLen))
			return
		}

		switch body.TimeElapsed {
		case model.TimeMoments, model.TimeHours, model.TimeDays,
			model.TimeWeeks, model.TimeFlashback, model.TimeSimultaneous:
		default:
			respondErr(w, http.StatusBadRequest, "invalid time_elapsed")
			return
		}

		ctx := r.Context()

		// Block if a plan is currently resolving or any plans are pending on this row.
		if _, err := q.GetResolvingPlanForGame(ctx, gameRow.ID); err == nil {
			respondErr(w, http.StatusConflict, "resolve the active plan before setting a scene")
			return
		}
		pending, err := q.ListPendingPlansByRow(ctx, dbgen.ListPendingPlansByRowParams{
			GameID:    gameRow.ID,
			RowNumber: gameRow.CurrentRow,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check pending plans")
			return
		}
		if len(pending) > 0 {
			respondErr(w, http.StatusConflict, "resolve pending plans on this row before setting a scene")
			return
		}

		// Block if a scene is already active.
		if existing, err := loadActiveScene(ctx, q, gameRow.ID); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not check active scene")
			return
		} else if existing != nil {
			respondErr(w, http.StatusConflict, "a scene is already active")
			return
		}

		// Validate holding (if used).
		if hasHolding {
			holding, err := q.GetAssetByID(ctx, *body.LocationHoldingID)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "holding not found")
				return
			}
			if holding.GameID != gameRow.ID {
				respondErr(w, http.StatusBadRequest, "holding is not part of this game")
				return
			}
			if holding.AssetType != model.AssetHolding {
				respondErr(w, http.StatusBadRequest, "location asset must be a holding")
				return
			}
			if holding.IsDestroyed {
				respondErr(w, http.StatusBadRequest, "holding is destroyed")
				return
			}
		}

		// Validate present peers and pre-compute their controllers.
		focusPlayerID := player.ID
		type peerWithController struct {
			AssetID    int64
			Controller *int64 // nil = unclaimed (focus-player non-MC peer)
		}
		seen := make(map[int64]bool, len(body.PresentPeerIDs))
		peers := make([]peerWithController, 0, len(body.PresentPeerIDs))
		for _, pid := range body.PresentPeerIDs {
			if seen[pid] {
				continue
			}
			seen[pid] = true

			asset, err := q.GetAssetByID(ctx, pid)
			if err != nil {
				respondErr(w, http.StatusBadRequest, "present peer not found")
				return
			}
			if asset.GameID != gameRow.ID {
				respondErr(w, http.StatusBadRequest, "peer is not part of this game")
				return
			}
			if asset.AssetType != model.AssetPeer {
				respondErr(w, http.StatusBadRequest, "present_peer_ids must reference peer assets")
				return
			}
			if asset.IsDestroyed {
				respondErr(w, http.StatusBadRequest, "peer is destroyed")
				return
			}
			if asset.IsMainCharacter && asset.OwnerID == focusPlayerID {
				// The focus player's main character is implicitly present.
				respondErr(w, http.StatusBadRequest,
					"your main character is implicitly present; do not list it")
				return
			}

			var controller *int64
			switch {
			case asset.OwnerID != focusPlayerID:
				owner := asset.OwnerID
				controller = &owner
			case asset.IsMainCharacter:
				// Already filtered above, but keep the rule explicit.
				controller = &focusPlayerID
			default:
				// Focus-player peer that is NOT their main character → unclaimed.
				controller = nil
			}
			peers = append(peers, peerWithController{AssetID: pid, Controller: controller})
		}

		// Look up the prompt + resolved plan id from the most recent
		// resolved plan on this row, if any.
		prompt := ""
		var resolvedPlanID *int64
		if recent, err := q.GetMostRecentResolvedPlanOnRow(ctx, dbgen.GetMostRecentResolvedPlanOnRowParams{
			GameID:    gameRow.ID,
			RowNumber: gameRow.CurrentRow,
		}); err == nil {
			prompt = game.FollowOnPrompt(recent.PlanType)
			id := recent.ID
			resolvedPlanID = &id
		}

		// Insert the scene.
		var holdingID *int64
		var customLoc *string
		if hasHolding {
			holdingID = body.LocationHoldingID
		} else {
			s := body.LocationCustom
			customLoc = &s
		}
		var timeNote *string
		if tn := strings.TrimSpace(body.TimeNote); tn != "" {
			timeNote = &tn
		}

		scene, err := q.CreateScene(ctx, dbgen.CreateSceneParams{
			GameID:            gameRow.ID,
			RowNumber:         gameRow.CurrentRow,
			FocusPlayerID:     player.ID,
			LocationHoldingID: holdingID,
			LocationCustom:    customLoc,
			TimeElapsed:       body.TimeElapsed,
			TimeNote:          timeNote,
			Prompt:            prompt,
			ResolvedPlanID:    resolvedPlanID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create scene")
			return
		}

		for _, pw := range peers {
			if err := q.InsertScenePeer(ctx, dbgen.InsertScenePeerParams{
				SceneID:            scene.ID,
				PeerAssetID:        pw.AssetID,
				ControllerPlayerID: pw.Controller,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not record scene peers")
				return
			}
		}

		resp, err := buildSceneResponse(ctx, q, &scene)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not assemble scene response")
			return
		}

		// Boundary post + WS broadcast.
		row := scene.RowNumber
		EmitBoundary(ctx, q, manager, gameRow.ID, "scene.started",
			summarizeSceneStart(player.DisplayName, &scene),
			&row, nil,
			map[string]any{"scene_id": scene.ID})
		if h, ok := manager.Get(gameRow.ID); ok {
			h.BroadcastEvent(model.EventSceneStarted, model.SceneStartedPayload{
				Scene: scene,
				Peers: resp.Peers,
			})
		}

		respond(w, http.StatusCreated, resp)
	}
}

// summarizeSceneStart builds the boundary-post body for a new scene.
func summarizeSceneStart(focusName string, scene *dbgen.Scene) string {
	var loc string
	switch {
	case scene.LocationCustom != nil:
		loc = *scene.LocationCustom
	case scene.LocationHoldingID != nil:
		loc = "a holding"
	default:
		loc = "somewhere"
	}
	return fmt.Sprintf("%s sets a scene at %s.", focusName, loc)
}

// GetActiveSceneHandler handles GET /api/tables/{id}/scenes/active.
//
// Returns the active scene + its peer list, or {scene: null} if none.
func GetActiveSceneHandler(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		ctx := r.Context()
		scene, err := loadActiveScene(ctx, q, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load scene")
			return
		}
		if scene == nil {
			respond(w, http.StatusOK, map[string]any{"scene": nil, "peers": []scenePeerView{}})
			return
		}
		resp, err := buildSceneResponse(ctx, q, scene)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load scene peers")
			return
		}
		respond(w, http.StatusOK, resp)
	}
}

// ClaimScenePeer handles POST /api/tables/{id}/scenes/{sid}/claim-peer.
//
// A non-focus player claims an unclaimed focus-player peer for the
// remainder of the current scene.
func ClaimScenePeer(q *dbgen.Queries, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, q)
		if !ok {
			return
		}
		sceneID, err := strconv.ParseInt(chi.URLParam(r, "sid"), 10, 64)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "invalid scene id")
			return
		}

		var body struct {
			PeerAssetID int64 `json:"peer_asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		scene, err := q.GetSceneByID(ctx, sceneID)
		if err != nil {
			respondErr(w, http.StatusNotFound, "scene not found")
			return
		}
		if scene.GameID != gameID {
			respondErr(w, http.StatusNotFound, "scene not found")
			return
		}
		if scene.EndedAt.Valid {
			respondErr(w, http.StatusConflict, "scene has ended")
			return
		}
		if scene.FocusPlayerID == player.ID {
			respondErr(w, http.StatusForbidden, "the focus player cannot claim peers")
			return
		}

		// The peer must belong to the focus player and be in the scene.
		asset, err := q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "peer not found")
			return
		}
		if asset.OwnerID != scene.FocusPlayerID {
			respondErr(w, http.StatusForbidden, "only the focus player's peers can be taken over")
			return
		}

		n, err := q.ClaimScenePeer(ctx, dbgen.ClaimScenePeerParams{
			SceneID:            scene.ID,
			PeerAssetID:        body.PeerAssetID,
			ControllerPlayerID: &player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not claim peer")
			return
		}
		if n == 0 {
			// Either not in this scene or already claimed.
			respondErr(w, http.StatusConflict, "peer is not available to take over")
			return
		}

		if h, ok := manager.Get(gameID); ok {
			h.BroadcastEvent(model.EventScenePeerClaimed, model.ScenePeerClaimedPayload{
				SceneID:      scene.ID,
				PeerAssetID:  body.PeerAssetID,
				ControllerID: player.ID,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"scene_id":      scene.ID,
			"peer_asset_id": body.PeerAssetID,
			"controller_id": player.ID,
		})
	}
}
