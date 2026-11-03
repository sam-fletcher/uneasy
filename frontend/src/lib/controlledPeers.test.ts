import { describe, expect, it } from 'vitest';
import { controlledPeers } from './controlledPeers';

const asset = (id: number, is_destroyed = false) => ({ id, is_destroyed }) as never;
const sp = (peer_asset_id: number, controller_player_id: number | null) =>
	({ peer_asset_id, controller_player_id }) as never;

describe('controlledPeers', () => {
	it('returns my live peers and drops a destroyed one still listed in the scene', () => {
		const out = controlledPeers([sp(1, 7), sp(2, 7), sp(3, 8)], [asset(1), asset(2, true), asset(3)], 7);
		expect(out.map(a => a.id)).toEqual([1]);
	});
	it('is empty with no player', () => {
		expect(controlledPeers([sp(1, 7)], [asset(1)], null)).toEqual([]);
	});
});
