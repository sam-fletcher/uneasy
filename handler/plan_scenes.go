package handler

// handler/plan_scenes.go — plan-scene lifecycle (adr/CHAT_OVERHAUL_PLAN.md
// Phase 5). Gives roleplay-heavy plan resolutions (Host Festivity, Propose
// Decree, Chronicle Histories, Clandestinely Liaise) the same scene-
// container treatment as turn-scenes, minus the location/time setup step,
// so validateSpeakingAs (scenes.go) stops blocking in-character speech
// during exactly the moments the rules call for roleplay.
//
// Lifecycle: opened from kickoffPlanResolution immediately after
// EmitPlanResolving fires (before OnResolve runs — see PlanSceneStager's
// doc comment on why participant computation can't depend on OnResolve's
// side effects), closed from EmitPlanResolved regardless of which of its
// several call sites fired (make, mar, cancelled all close it) — a single
// choke point covers every plan type without touching each call site.

import (
	"context"
	"fmt"

	dbgen "uneasy/db/gen"
	"uneasy/hub"
	"uneasy/model"
)

// maybeOpenPlanScene opens a plan-scene for plan if its handler opts into
// PlanSceneStager. No-op for the other eight plan types, and best-effort for
// the four that do: a failure here degrades to "can't speak in character
// during this resolution" (the pre-Phase-5 status quo), not a broken
// resolution — mirroring EmitPlanResolving's own best-effort posture right
// next to this call.
func maybeOpenPlanScene(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan *dbgen.Plan) {
	h, ok := GetHandler(plan.PlanType)
	if !ok {
		return
	}
	stager, ok := h.(PlanSceneStager)
	if !ok {
		return
	}
	if plan.RowNumber == nil {
		return
	}
	// Scenes are exclusive (scenes_one_active_per_game) and mutually exclusive
	// with plan resolution by construction — validateSceneTiming blocks a new
	// turn-scene while a plan resolves, and the follow-scene gate ensures any
	// prior turn-scene has already ended before the next plan's auto-kickoff.
	// This check is defense-in-depth: skip rather than error if one is
	// somehow still open.
	if existing, err := loadActiveScene(ctx, q, plan.GameID); err != nil || existing != nil {
		return
	}
	participantIDs, err := stager.PlanSceneParticipants(ctx, q, plan)
	if err != nil || len(participantIDs) == 0 {
		return
	}

	scene, err := q.CreatePlanScene(ctx, dbgen.CreatePlanSceneParams{
		GameID:        plan.GameID,
		RowNumber:     *plan.RowNumber,
		FocusPlayerID: plan.PreparerID,
		PlanID:        &plan.ID,
	})
	if err != nil {
		return
	}

	names := insertPlanScenePeers(ctx, q, scene.ID, plan.GameID, participantIDs)

	resp, err := buildSceneResponse(ctx, q, &scene)
	if err != nil {
		return
	}

	row := scene.RowNumber
	sceneID := scene.ID
	planID := plan.ID
	EmitSystemPost(ctx, q, manager, plan.GameID, "scene.started",
		model.SeverityImportant,
		fmt.Sprintf("%s — the scene opens.", planLabel(plan.PlanType)),
		&row, &planID, &sceneID,
		map[string]any{
			"scene_id":        scene.ID,
			"kind":            "plan",
			"focus_player_id": scene.FocusPlayerID,
			"plan_id":         plan.ID,
			"participants":    names,
		})
	if hb, ok := manager.Get(plan.GameID); ok {
		hb.BroadcastEvent(model.EventSceneStarted, model.SceneStartedPayload{
			Scene: scene,
			Peers: resp.Peers,
		})
	}
}

// insertPlanScenePeers inserts one scene_peers row per participant's main
// character (controller = the participant themselves — unlike a turn-scene,
// a plan-scene records the focus player/preparer's own MC explicitly too;
// see the kind guard in validateSpeakingAs), deduping participantIDs and
// skipping (not failing) a player who has no main character right now — a
// rare transient state (AwaitMainCharacterChoice) that shouldn't block the
// rest of the scene from opening. Returns the resolved character names, for
// the scene.started system_data's "participants" list.
func insertPlanScenePeers(
	ctx context.Context, q *dbgen.Queries, sceneID, gameID int64, participantIDs []int64,
) []string {
	seen := make(map[int64]bool, len(participantIDs))
	names := make([]string, 0, len(participantIDs))
	for _, playerID := range participantIDs {
		if seen[playerID] {
			continue
		}
		seen[playerID] = true
		mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
			GameID: gameID, OwnerID: playerID,
		})
		if err != nil {
			continue
		}
		if err := q.InsertScenePeer(ctx, dbgen.InsertScenePeerParams{
			SceneID: sceneID, PeerAssetID: mc.ID, ControllerPlayerID: &playerID,
		}); err != nil {
			continue
		}
		names = append(names, mc.Name)
	}
	return names
}

// AddPlanSceneParticipant adds a dynamically-joining player to the game's
// active plan-scene (e.g. Propose Decree's join-council) — a no-op if the
// scene has already closed, the player already has a peer row, or they have
// no main character right now. Re-broadcasts a full EventSceneStarted
// snapshot (scene + refreshed peer list) rather than EventScenePeerClaimed,
// since this is a brand-new peer row, not a claim on an existing unclaimed
// one — the client's SceneStarted handler already does an unconditional
// full replace, so this doubles as the "trigger an active-scene refetch"
// the plan calls for, without adding a new WS event or client handler.
func AddPlanSceneParticipant(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager, scene *dbgen.Scene, playerID int64,
) {
	if scene == nil || scene.Kind != model.SceneKindPlan || scene.EndedAt.Valid {
		return
	}
	mc, err := q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
		GameID: scene.GameID, OwnerID: playerID,
	})
	if err != nil {
		return
	}
	if _, err := q.GetScenePeer(ctx, dbgen.GetScenePeerParams{
		SceneID: scene.ID, PeerAssetID: mc.ID,
	}); err == nil {
		return // already a peer
	}
	if err := q.InsertScenePeer(ctx, dbgen.InsertScenePeerParams{
		SceneID: scene.ID, PeerAssetID: mc.ID, ControllerPlayerID: &playerID,
	}); err != nil {
		return
	}
	resp, err := buildSceneResponse(ctx, q, scene)
	if err != nil {
		return
	}
	if hb, ok := manager.Get(scene.GameID); ok {
		hb.BroadcastEvent(model.EventSceneStarted, model.SceneStartedPayload{
			Scene: *scene,
			Peers: resp.Peers,
		})
	}
}

// closePlanSceneIfAny ends plan's active plan-scene, if any, and posts
// scene.ended. Called from EmitPlanResolved so every result (make, mar,
// cancelled) and every one of its several call sites across the plan
// handlers close the scene uniformly — a cancelled plan can't leave one
// dangling. No-op for the eight plan types that never open one, and for a
// plan-scene that isn't this plan's (defensive; scenes are exclusive so
// this should never actually happen).
func closePlanSceneIfAny(ctx context.Context, q *dbgen.Queries, manager *hub.Manager, plan dbgen.Plan) {
	scene, err := loadActiveScene(ctx, q, plan.GameID)
	if err != nil || scene == nil {
		return
	}
	if scene.Kind != model.SceneKindPlan || scene.PlanID == nil || *scene.PlanID != plan.ID {
		return
	}
	if err := q.EndScene(ctx, scene.ID); err != nil {
		return
	}

	row := scene.RowNumber
	sceneID := scene.ID
	planID := plan.ID
	EmitSystemPost(ctx, q, manager, plan.GameID, "scene.ended",
		model.SeverityImportant,
		fmt.Sprintf("%s — the scene ends.", planLabel(plan.PlanType)),
		&row, &planID, &sceneID,
		map[string]any{"row_number": row, "plan_id": plan.ID, "scene_id": scene.ID})

	if hb, ok := manager.Get(plan.GameID); ok {
		hb.BroadcastEvent(model.EventSceneEnded, model.SceneEndedPayload{
			RowNumber: scene.RowNumber,
			PlayerID:  plan.PreparerID,
			SceneID:   scene.ID,
		})
	}
}

// repointScenePeerToNewMainCharacter updates the active scene's peer row for
// playerID (if any) to point at their newly-promoted/conscripted main
// character. A main-character swap (promotion or the no-peers-left
// conscription escape hatch) doesn't change WHO controls a scene_peers row,
// only WHICH asset it references — without this, a participant who loses
// and replaces their main character mid-scene would find their new MC
// silently rejected as "not in the current scene" (the old row still points
// at the old, now-destroyed asset). Applies uniformly to whichever scene
// (turn or plan) is currently active; a no-op if none is, or the player has
// no peer row in it (they weren't a participant/present peer).
func repointScenePeerToNewMainCharacter(
	ctx context.Context, q *dbgen.Queries, manager *hub.Manager, gameID, playerID, newAssetID int64,
) {
	scene, err := loadActiveScene(ctx, q, gameID)
	if err != nil || scene == nil {
		return
	}
	peer, err := q.GetScenePeerByOwner(ctx, dbgen.GetScenePeerByOwnerParams{
		SceneID: scene.ID, OwnerID: playerID,
	})
	if err != nil || peer.PeerAssetID == newAssetID {
		return
	}
	if _, err := q.UpdateScenePeerAsset(ctx, dbgen.UpdateScenePeerAssetParams{
		SceneID:        scene.ID,
		OldPeerAssetID: peer.PeerAssetID,
		NewPeerAssetID: newAssetID,
	}); err != nil {
		return
	}
	resp, err := buildSceneResponse(ctx, q, scene)
	if err != nil {
		return
	}
	if hb, ok := manager.Get(gameID); ok {
		hb.BroadcastEvent(model.EventSceneStarted, model.SceneStartedPayload{
			Scene: *scene,
			Peers: resp.Peers,
		})
	}
}
