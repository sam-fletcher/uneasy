import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import SceneDetailsPanel from './SceneDetailsPanel.svelte';

// A destroyed peer stays in the scene's peer list and in the live asset list
// (flagged); it must not render as a live, claimable card.
const peerAsset = (id: number, name: string, is_destroyed: boolean) =>
	({
		id, name, owner_id: 1, asset_type: 'peer', is_main_character: false,
		is_destroyed, is_leveraged: false, marginalia: [],
	}) as never;
const scenePeer = (peer_asset_id: number) =>
	({ peer_asset_id, controller_player_id: null }) as never;

describe('SceneDetailsPanel', () => {
	it('lists a live peer but not a destroyed one', () => {
		const { body } = render(SceneDetailsPanel, {
			props: {
				gameID: 1,
				scene: { id: 1, focus_player_id: 2, location_holding_id: null, time_elapsed: 'hours' } as never,
				peers: [scenePeer(10), scenePeer(11)],
				assets: [peerAsset(10, 'Livia Liveone', false), peerAsset(11, 'Dead Duncan', true)],
				players: [{ id: 1, display_name: 'ravi' }, { id: 2, display_name: 'sol' }] as never,
				currentPlayerID: 1,
				isFocusPlayer: false,
				onSceneEnded: () => {},
			},
		});
		expect(body).toContain('Livia Liveone');
		expect(body).not.toContain('Dead Duncan');
	});
});
