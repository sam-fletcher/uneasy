// options.test.ts — the pure views behind Host Festivity's centre of the party.
//
// Two kinds of peer wait there and players see one list: real peers shoved to
// the centre by a disagreement, and drafts introduced at the event who own no
// asset row until claimed (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md, D7). The
// merge and the reverse mapping back to a request are pure, so they're testable
// without a picker.

import { describe, it, expect } from 'vitest';
import type { Asset } from '$lib/api';
import {
	centeredPeerCards, centeredPeerNames, centeredPeerPick, draftPickID, isDraftCard,
	earnedHostMakes, festivityEndable, type FestRes,
} from './options';

const asset = (id: number, name: string, ownerID = 7): Asset => ({
	id, game_id: 1, owner_id: ownerID, creator_id: ownerID,
	asset_type: 'peer', name,
	is_main_character: false, is_leveraged: false, is_destroyed: false,
	created_at: '', destroyed_at: null,
	linked_card_suit: null, linked_card_value: null,
	marginalia: [], secret_count: 0,
});

const draft = (id: string, name: string, marginalia?: string) => ({
	id, name, marginalia, creator_id: 7,
});

function fest(over: Partial<FestRes> = {}): FestRes {
	return {
		guests: [1, 2, 3],
		outcomes: {},
		guestMakes: {},
		guestMars: {},
		hostMakesTaken: [],
		guestRollIDs: {},
		guestIOUs: [],
		hostMarInsists: [],
		pendingHostMars: [],
		acceptDuels: [],
		pendingDuelPlanID: null,
		pendingChallenge: null,
		centeredAssetIDs: [],
		centeredDrafts: [],
		...over,
	};
}

describe('draftPickID', () => {
	it('is negative, so it can never collide with a real asset id', () => {
		for (const id of ['a1b2c3d4', 'ffffffffffffffff', '0', 'd1zz9']) {
			expect(draftPickID(id)).toBeLessThan(0);
		}
	});

	it('is stable for the same draft and distinct across drafts', () => {
		expect(draftPickID('a1b2c3d4')).toBe(draftPickID('a1b2c3d4'));
		expect(draftPickID('a1b2c3d4')).not.toBe(draftPickID('a1b2c3d5'));
	});

	it('does not depend on position, so removing a draft can not re-point a pick', () => {
		const before = fest({ centeredDrafts: [draft('a', 'Ada'), draft('b', 'Bo')] });
		const picked = centeredPeerCards(before, [], 1)[1].id;
		// Ada is claimed mid-selection; Bo shifts from index 1 to index 0.
		const after = fest({ centeredDrafts: [draft('b', 'Bo')] });
		expect(centeredPeerPick(after, picked)).toEqual({ draft_id: 'b' });
	});
});

describe('centeredPeerCards', () => {
	it('merges disagreement peers and drafts into one list, disagreements first', () => {
		const f = fest({
			centeredAssetIDs: [11],
			centeredDrafts: [draft('a', 'Ada', 'keeps a hawk')],
		});
		const cards = centeredPeerCards(f, [asset(11, 'Cousin'), asset(12, 'Elsewhere')], 1);
		expect(cards.map(c => c.name)).toEqual(['Cousin', 'Ada']);
	});

	it('skips destroyed centered assets but keeps every draft', () => {
		const gone = { ...asset(11, 'Cousin'), is_destroyed: true };
		const f = fest({ centeredAssetIDs: [11], centeredDrafts: [draft('a', 'Ada')] });
		expect(centeredPeerCards(f, [gone], 1).map(c => c.name)).toEqual(['Ada']);
	});

	it('renders a draft’s marginalia on its card, and none when it has no note', () => {
		const f = fest({ centeredDrafts: [draft('a', 'Ada', 'keeps a hawk'), draft('b', 'Bo')] });
		const [ada, bo] = centeredPeerCards(f, [], 1);
		expect(ada.marginalia.map(m => m.text)).toEqual(['keeps a hawk']);
		expect(bo.marginalia).toEqual([]);
	});

	it('marks draft cards apart from real ones so the owner label can differ', () => {
		const f = fest({ centeredAssetIDs: [11], centeredDrafts: [draft('a', 'Ada')] });
		const [cousin, ada] = centeredPeerCards(f, [asset(11, 'Cousin')], 1);
		expect(isDraftCard(cousin)).toBe(false);
		expect(isDraftCard(ada)).toBe(true);
		expect(ada.owner_id).toBe(0); // in nobody's retinue — that's the point
	});
});

describe('centeredPeerPick', () => {
	it('sends a real centered peer as asset_id', () => {
		const f = fest({ centeredAssetIDs: [11] });
		expect(centeredPeerPick(f, 11)).toEqual({ asset_id: 11 });
	});

	it('sends a draft as draft_id', () => {
		const f = fest({ centeredDrafts: [draft('a', 'Ada')] });
		expect(centeredPeerPick(f, draftPickID('a'))).toEqual({ draft_id: 'a' });
	});

	it('returns null once the pick has left the centre', () => {
		expect(centeredPeerPick(fest(), 11)).toBeNull();
		expect(centeredPeerPick(fest(), draftPickID('a'))).toBeNull();
	});
});

describe('centeredPeerNames', () => {
	it('names both kinds for the read-only roster', () => {
		const f = fest({ centeredAssetIDs: [11], centeredDrafts: [draft('a', 'Ada')] });
		expect(centeredPeerNames(f, [asset(11, 'Cousin')])).toEqual(['Cousin', 'Ada']);
	});
});

describe('festivityEndable', () => {
	it('does not hold the event open for peers still in the centre', () => {
		// Ending the event is what settles them, so an unclaimed draft must not
		// block the host from winding down.
		const f = fest({
			guests: [1, 2],
			outcomes: { 1: 'host', 2: 'opt_out' },
			hostMakesTaken: ['introduce_peer', 'spread_rumor'],
			centeredDrafts: [draft('a', 'Ada')],
		});
		expect(earnedHostMakes(f, 1)).toBe(2);
		expect(festivityEndable(f, 1)).toBe(true);
	});
});
