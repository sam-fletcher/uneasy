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
// Per the rules, players may only speak as a character during a Scene, and
// only as characters present in that scene under their control.
//
// Rules:
//   - assetID == 0 (unset) → always allowed (speaking as oneself).
//   - A character attribution requires an active scene. The asset must be a
//     scene_peer of the active scene controlled by the caller, OR the caller's
//     main character when the caller is the scene's focus player (the focus
//     player's main character is implicitly present and never recorded as a
//     scene_peer).
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

	// Speaking as a character is only permitted while a scene is active.
	scene, err := loadActiveScene(ctx, q, gameID)
	if err != nil {
		return http.StatusInternalServerError, "could not load active scene"
	}
	if scene == nil {
		return http.StatusBadRequest, "you can only speak as a character during a scene"
	}

	// A peer present in the scene must be controlled by the caller.
	peer, err := q.GetScenePeer(ctx, dbgen.GetScenePeerParams{
		SceneID:     scene.ID,
		PeerAssetID: assetID,
	})
	if err == nil {
		if peer.ControllerPlayerID == nil || *peer.ControllerPlayerID != playerID {
			return http.StatusForbidden, "that character is not yours to speak as"
		}
		return 0, ""
	}

	// The focus player's main character is implicitly present in their scene
	// and is never recorded as a scene_peer, so allow it explicitly.
	if asset.IsMainCharacter && asset.OwnerID == playerID && scene.FocusPlayerID == playerID {
		return 0, ""
	}

	return http.StatusBadRequest, "that character is not in the current scene"
}

// peerWithController pairs a peer asset ID with its assigned controller
// during scene creation (nil = unclaimed focus-player non-MC peer).
type peerWithController struct {
	AssetID    int64
	Controller *int64
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

// validateAndProcessScenePeers validates present peer IDs and returns their
// processed form with assigned controllers. Returns a non-zero statusCode on
// validation failure.
func validateAndProcessScenePeers(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	focusPlayerID int64,
	peerIDs []int64,
) ([]peerWithController, int, string) {
	seen := make(map[int64]bool, len(peerIDs))
	peers := make([]peerWithController, 0, len(peerIDs))

	for _, pid := range peerIDs {
		if seen[pid] {
			continue
		}
		seen[pid] = true

		asset, err := q.GetAssetByID(ctx, pid)
		if err != nil {
			return nil, http.StatusBadRequest, "present peer not found"
		}
		if asset.GameID != gameID {
			return nil, http.StatusBadRequest, "peer is not part of this game"
		}
		if asset.AssetType != model.AssetPeer {
			return nil, http.StatusBadRequest, "present_peer_ids must reference peer assets"
		}
		if asset.IsDestroyed {
			return nil, http.StatusBadRequest, "peer is destroyed"
		}
		if asset.IsMainCharacter && asset.OwnerID == focusPlayerID {
			// The focus player's main character is implicitly present.
			return nil, http.StatusBadRequest,
				"your main character is implicitly present; do not list it"
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

	return peers, 0, ""
}

// validateSceneTiming enforces when a scene may be set and returns the
// follow-scene source — the most-recently resolved plan on the current row,
// or nil for a blank-row / row-start scene. A non-zero statusCode + message
// signals a precondition failure (mirrors validateAndProcessScenePeers).
//
//   - a plan is mid-resolution                → block (resolve it first)
//   - no plan resolved yet, but plans pending → block (resolve the topmost
//     plan before scening, per the per-row loop)
//   - a plan resolved, its follow-scene set   → block (already scened)
//   - a plan resolved, no follow-scene yet    → allow; return that plan
//     (the follow-scene is allowed even while later plans on the row pend)
//   - blank row, nothing pending              → allow; return nil
func validateSceneTiming(
	ctx context.Context,
	q *dbgen.Queries,
	gameID int64,
	currentRow int16,
) (recent *dbgen.Plan, statusCode int, errMsg string) {
	if _, err := q.GetResolvingPlanForGame(ctx, gameID); err == nil {
		return nil, http.StatusConflict, "resolve the active plan before setting a scene"
	}

	if r0, err := q.GetMostRecentResolvedPlanOnRow(ctx, dbgen.GetMostRecentResolvedPlanOnRowParams{
		GameID:    gameID,
		RowNumber: new(currentRow),
	}); err == nil {
		recent = &r0
	}

	if recent == nil {
		pending, err := q.ListPendingPlansByRow(ctx, dbgen.ListPendingPlansByRowParams{
			GameID:    gameID,
			RowNumber: new(currentRow),
		})
		if err != nil {
			return nil, http.StatusInternalServerError, "could not check pending plans"
		}
		if len(pending) > 0 {
			return nil, http.StatusConflict, "resolve pending plans on this row before setting a scene"
		}
		return nil, 0, ""
	}

	if scenes, err := q.ListScenesForRow(ctx, dbgen.ListScenesForRowParams{
		GameID:    gameID,
		RowNumber: currentRow,
	}); err == nil && findFollowScene(scenes, recent.ID) != nil {
		return nil, http.StatusConflict, "the follow-scene for the last resolved plan has already been set"
	}

	return recent, 0, ""
}

// ── HTTP handlers ────────────────────────────────────────────────────────────

// CreateScene handles POST /api/tables/{id}/scenes.
//
// Focus player only; main_event phase only. There must be no active scene
// in the game and no pending or resolving plans on the current row. The
// server fills in `prompt` and `resolved_plan_id` based on the most
// recently resolved plan on this row (if any).
//
//nolint:funlen,gocognit // HTTP handler with extensive validation logic
func CreateScene(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameRow, player, ok := requireFocusPlayer(w, r, s.Q)
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

		// Enforce the rulebook ordering for setting a scene and recover the
		// follow-scene source (the most-recently resolved plan on this row, if
		// any). `recent` is reused below for the prompt + resolved_plan_id.
		recent, statusCode, errMsg := validateSceneTiming(ctx, s.Q, gameRow.ID, gameRow.CurrentRow)
		if statusCode != 0 {
			respondErr(w, statusCode, errMsg)
			return
		}

		// Block if a scene is already active.
		if existing, err := loadActiveScene(ctx, s.Q, gameRow.ID); err != nil {
			respondInternalErr(w, r, "could not check active scene", err)
			return
		} else if existing != nil {
			respondErr(w, http.StatusConflict, "a scene is already active")
			return
		}

		// Validate holding (if used).
		if hasHolding {
			holding, err := s.Q.GetAssetByID(ctx, *body.LocationHoldingID)
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
		peers, statusCode, errMsg := validateAndProcessScenePeers(
			ctx, s.Q, gameRow.ID, player.ID, body.PresentPeerIDs,
		)
		if statusCode != 0 {
			respondErr(w, statusCode, errMsg)
			return
		}

		// Carry the follow-on prompt + resolved plan id from the most recent
		// resolved plan on this row, if any (looked up above as `recent`).
		prompt := ""
		var resolvedPlanID *int64
		if recent != nil {
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
			str := body.LocationCustom
			customLoc = &str
		}
		var timeNote *string
		if tn := strings.TrimSpace(body.TimeNote); tn != "" {
			timeNote = &tn
		}

		var scene dbgen.Scene
		err := s.InTx(ctx, func(q *dbgen.Queries) error {
			sc, cErr := q.CreateScene(ctx, dbgen.CreateSceneParams{
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
			if cErr != nil {
				return errors.New("could not create scene")
			}
			scene = sc
			for _, pw := range peers {
				if iErr := q.InsertScenePeer(ctx, dbgen.InsertScenePeerParams{
					SceneID:            scene.ID,
					PeerAssetID:        pw.AssetID,
					ControllerPlayerID: pw.Controller,
				}); iErr != nil {
					return errors.New("could not record scene peers")
				}
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		resp, err := buildSceneResponse(ctx, s.Q, &scene)
		if err != nil {
			respondInternalErr(w, r, "could not assemble scene response", err)
			return
		}

		// Boundary post + WS broadcast.
		row := scene.RowNumber
		sceneID := scene.ID
		EmitSystemPost(ctx, s.Q, manager, gameRow.ID, "scene.started",
			model.SeverityImportant,
			resolveSceneBannerText(ctx, s.Q, &scene, player.DisplayName),
			&row, nil, &sceneID,
			map[string]any{"scene_id": scene.ID})
		if h, ok := manager.Get(gameRow.ID); ok {
			h.BroadcastEvent(model.EventSceneStarted, model.SceneStartedPayload{
				Scene: scene,
				Peers: resp.Peers,
			})
		}
		broadcastRowState(ctx, s.Q, manager, gameRow.ID)

		respond(w, http.StatusCreated, resp)
	}
}

// timeElapsedLabel formats a TimeElapsed enum value for display in the banner
// and public-record summary. Mirrors the labels used by the frontend.
func timeElapsedLabel(t model.TimeElapsed) string {
	switch t {
	case model.TimeMoments:
		return "Moments later"
	case model.TimeHours:
		return "Hours later"
	case model.TimeDays:
		return "Days later"
	case model.TimeWeeks:
		return "Weeks later"
	case model.TimeFlashback:
		return "Flashback"
	case model.TimeSimultaneous:
		return "Simultaneous"
	default:
		return string(t)
	}
}

// sceneBannerText builds the banner / public-record text for a scene in the
// form "<main character> at <holding>, <time later>" (with an optional
// "— <time note>" suffix). Used both for the scene-start chat boundary post
// and the scene-end public-record summary so the two stay in sync.
//
// holdingName is the resolved location name (looked up by the caller from
// scene.LocationHoldingID, or scene.LocationCustom for free-text scenes).
// mainCharName falls back to the focus player's display name when the focus
// player has no main-character asset set.
func sceneBannerText(mainCharName, holdingName string, scene *dbgen.Scene) string {
	if holdingName == "" {
		holdingName = "an unknown place"
	}
	timeElapsed := timeElapsedLabel(scene.TimeElapsed)
	if scene.TimeNote != nil {
		if note := strings.TrimSpace(*scene.TimeNote); note != "" {
			timeElapsed = note
		}
	}
	return fmt.Sprintf("Scene: %s at %s, %s",
		mainCharName, holdingName, timeElapsed,
	)
}

// resolveSceneBannerText looks up the main-character name and holding name
// for the scene and returns the formatted banner text. Falls back to the
// focus player's display name if no main character is set.
func resolveSceneBannerText(ctx context.Context, q *dbgen.Queries, scene *dbgen.Scene, focusPlayerName string) string {
	mainName := focusPlayerName
	if mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID:  scene.GameID,
		OwnerID: scene.FocusPlayerID,
	}); err == nil {
		mainName = mc.Name
	}
	var holdingName string
	switch {
	case scene.LocationHoldingID != nil:
		if a, err := q.GetAssetByID(ctx, *scene.LocationHoldingID); err == nil {
			holdingName = a.Name
		}
	case scene.LocationCustom != nil:
		holdingName = *scene.LocationCustom
	}
	return sceneBannerText(mainName, holdingName, scene)
}

// GetActiveSceneHandler handles GET /api/tables/{id}/scenes/active.
//
// Returns the active scene + its peer list, or {scene: null} if none.
func GetActiveSceneHandler(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r, s.Q)
		if !ok {
			return
		}
		ctx := r.Context()
		scene, err := loadActiveScene(ctx, s.Q, gameID)
		if err != nil {
			respondInternalErr(w, r, "could not load scene", err)
			return
		}
		if scene == nil {
			respond(w, http.StatusOK, map[string]any{"scene": nil, "peers": []scenePeerView{}})
			return
		}
		resp, err := buildSceneResponse(ctx, s.Q, scene)
		if err != nil {
			respondInternalErr(w, r, "could not load scene peers", err)
			return
		}
		respond(w, http.StatusOK, resp)
	}
}

// ClaimScenePeer handles POST /api/tables/{id}/scenes/{sid}/claim-peer.
//
// A non-focus player claims an unclaimed focus-player peer for the
// remainder of the current scene.
func ClaimScenePeer(s *db.Store, manager *hub.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, player, ok := parseGamePlayer(w, r, s.Q)
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
		scene, err := s.Q.GetSceneByID(ctx, sceneID)
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
		asset, err := s.Q.GetAssetByID(ctx, body.PeerAssetID)
		if err != nil {
			respondErr(w, http.StatusBadRequest, "peer not found")
			return
		}
		if asset.OwnerID != scene.FocusPlayerID {
			respondErr(w, http.StatusForbidden, "only the focus player's peers can be taken over")
			return
		}

		n, err := s.Q.ClaimScenePeer(ctx, dbgen.ClaimScenePeerParams{
			SceneID:            scene.ID,
			PeerAssetID:        body.PeerAssetID,
			ControllerPlayerID: &player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not claim peer", err)
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
