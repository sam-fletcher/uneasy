// make_introductions.test.ts — the draft-peer views the Make Introductions
// panel derives its whole resolve flow from. Peers named pre-roll are drafts
// with no asset behind them (adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md D4), so
// "who still owes an arrival?" is a pure question over resolution_data.

import { describe, it, expect } from 'vitest';
import {
	miDrafts, miHasArrived, miPendingArrivals,
	type MakeIntroductionsResolutionData,
} from './make_introductions';

const draft = (id: string, name: string) => ({ id, name, creator_id: 1 });

describe('miDrafts', () => {
	it('lists a normal plan’s named peers in naming order', () => {
		const mi: MakeIntroductionsResolutionData = {
			peer_count: 2,
			drafts: [draft('a', 'Ada'), draft('b', 'Bo')],
		};
		expect(miDrafts(mi).map(d => d.name)).toEqual(['Ada', 'Bo']);
	});

	it('reads a synthetic delayed-arrival plan’s single traveller instead', () => {
		const mi: MakeIntroductionsResolutionData = {
			delayed_arrival: true,
			delayed_draft: draft('t', 'Traveller'),
			// A synthetic plan carries no `drafts`; if it somehow did, the
			// traveller still wins — that is the plan's whole content.
			drafts: [draft('a', 'Ada')],
		};
		expect(miDrafts(mi).map(d => d.name)).toEqual(['Traveller']);
	});

	it('is empty for a delayed-arrival plan whose draft is missing', () => {
		expect(miDrafts({ delayed_arrival: true })).toEqual([]);
		expect(miDrafts({})).toEqual([]);
	});
});

describe('miHasArrived', () => {
	it('is true only once the draft has materialized into an asset', () => {
		const mi: MakeIntroductionsResolutionData = {
			drafts: [draft('a', 'Ada'), draft('b', 'Bo')],
			arrivals: [{ draft_id: 'a', asset_id: 7 }],
		};
		expect(miHasArrived(mi, 'a')).toBe(true);
		expect(miHasArrived(mi, 'b')).toBe(false);
		expect(miHasArrived({}, 'a')).toBe(false);
	});
});

describe('miPendingArrivals', () => {
	it('is empty before the roll resolves — nobody has arrived to describe yet', () => {
		expect(miPendingArrivals({ peer_count: 2, drafts: [draft('a', 'Ada')] })).toEqual([]);
	});

	it('lists every undescribed peer on the make path, in naming order', () => {
		const mi: MakeIntroductionsResolutionData = {
			make_pending: true,
			drafts: [draft('a', 'Ada'), draft('b', 'Bo'), draft('c', 'Cy')],
			arrivals: [{ draft_id: 'b', asset_id: 7 }],
		};
		expect(miPendingArrivals(mi).map(d => d.name)).toEqual(['Ada', 'Cy']);
	});

	it('empties once every draft has arrived, which is what unlocks Complete', () => {
		const mi: MakeIntroductionsResolutionData = {
			make_pending: true,
			drafts: [draft('a', 'Ada')],
			arrivals: [{ draft_id: 'a', asset_id: 7 }],
		};
		expect(miPendingArrivals(mi)).toEqual([]);
	});

	it('asks for the traveller on a delayed-arrival plan, then stops', () => {
		const travelling: MakeIntroductionsResolutionData = {
			delayed_arrival: true,
			delayed_draft: draft('t', 'Traveller'),
		};
		expect(miPendingArrivals(travelling).map(d => d.name)).toEqual(['Traveller']);
		expect(miPendingArrivals({ ...travelling, arrivals: [{ draft_id: 't', asset_id: 9 }] }))
			.toEqual([]);
	});

	it('stays empty on the mar path — those peers arrive through their outcomes', () => {
		const mi: MakeIntroductionsResolutionData = {
			mar_pending: true,
			drafts: [draft('a', 'Ada')],
			mar_outcomes: [{ draft_id: 'a', outcome: 'broken_arrival', done: false }],
		};
		expect(miPendingArrivals(mi)).toEqual([]);
	});
});
