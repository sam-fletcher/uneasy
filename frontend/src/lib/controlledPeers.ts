import type { Asset, ScenePeerView } from '$lib/api';

/**
 * The peers a player currently controls in the active scene, as assets.
 * Destroyed peers stay in the scene's peer list and in the live asset list
 * (flagged), so they are filtered here: speaking as one is refused by the
 * server.
 */
export function controlledPeers(
	scenePeers: ScenePeerView[],
	assets: Asset[],
	playerID: number | null,
): Asset[] {
	if (playerID == null) return [];
	return scenePeers
		.filter(p => p.controller_player_id === playerID)
		.map(p => assets.find(a => a.id === p.peer_asset_id))
		.filter((a): a is Asset => a != null && !a.is_destroyed);
}
