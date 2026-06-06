import { apiFetch } from './client';

export type TimeElapsed =
	| 'moments'
	| 'hours'
	| 'days'
	| 'weeks'
	| 'flashback'
	| 'simultaneous';

export interface Scene {
	id: number;
	game_id: number;
	row_number: number;
	focus_player_id: number;
	location_holding_id: number | null;
	location_custom: string | null;
	time_elapsed: TimeElapsed;
	time_note: string | null;
	prompt: string;
	resolved_plan_id: number | null;
	started_at: string;
	ended_at: string | null;
}

export interface ScenePeerView {
	peer_asset_id: number;
	/** null = unclaimed focus-player peer (a non-focus player can take over). */
	controller_player_id: number | null;
}

export interface SceneResponse {
	scene: Scene | null;
	peers: ScenePeerView[];
}

/**
 * Ephemeral broadcast of the focus player's in-flight scene-setup
 * selections. Mirrors model.SceneSetupDraftPayload. Not persisted; consumed
 * by SceneSetupForm in read-only mode so non-focus players can see what's
 * being chosen as it happens.
 */
export interface SceneSetupDraft {
	player_id: number;
	holding_id: number | null;
	custom_location: string;
	time_elapsed: string;
	time_note: string;
	present_peer_ids: number[];
}

/**
 * Ephemeral broadcast of the focus player's currently-highlighted plan
 * card during the post-scene "prepare a plan" step. Mirrors
 * model.PreparePlanDraftPayload. Not persisted. plan_type is "" when the
 * focus player has nothing selected.
 */
export interface PreparePlanDraft {
	player_id: number;
	plan_type: string;
	/**
	 * Opaque per-plan-type snapshot of the prep form. The shape is owned
	 * by the plan's panel component; consumers cast based on plan_type.
	 * May be undefined/null when only a card is highlighted but no fields
	 * have been touched yet.
	 */
	prep?: Record<string, unknown> | null;
}

export function getActiveScene(gameID: string | number): Promise<SceneResponse> {
	return apiFetch(`/tables/${gameID}/scenes/active`);
}

export function createScene(
	gameID: string | number,
	params: {
		location_holding_id?: number | null;
		location_custom?: string;
		time_elapsed: TimeElapsed;
		time_note?: string;
		present_peer_ids: number[];
	}
): Promise<SceneResponse> {
	return apiFetch(`/tables/${gameID}/scenes`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

export function claimScenePeer(
	gameID: string | number,
	sceneID: number,
	peerAssetID: number
): Promise<{ scene_id: number; peer_asset_id: number; controller_id: number }> {
	return apiFetch(`/tables/${gameID}/scenes/${sceneID}/claim-peer`, {
		method: 'POST',
		body: JSON.stringify({ peer_asset_id: peerAssetID })
	});
}
