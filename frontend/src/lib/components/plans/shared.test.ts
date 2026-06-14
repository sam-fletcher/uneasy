import { describe, it, expect } from 'vitest';
import type { Asset, Marginalium, Plan, Player } from '$lib/api';
import {
	parseResolutionData,
	demandWinnersFromPlan,
	activeDemandAgainst,
	playerName,
	assetName,
	intactMarginalia,
	assetsWithIntactMarginalia,
	playersExcept,
	ownerUnleveragedAssets,
	ownerIntactAssets,
} from './shared';
import { parseSpreadRumorsData } from '$lib/plans/resolutionData/spread_rumors';

// ── Fixtures ───────────────────────────────────────────────────────────────

function marg(overrides: Partial<Marginalium> = {}): Marginalium {
	return {
		id: 1,
		asset_id: 1,
		position: 0,
		text: 'note',
		is_torn: false,
		torn_at: null,
		torn_by_id: null,
		...overrides,
	};
}

function asset(overrides: Partial<Asset> = {}): Asset {
	return {
		id: 1,
		game_id: 1,
		owner_id: 1,
		creator_id: 1,
		asset_type: 'peer',
		name: 'Asset',
		is_main_character: false,
		is_leveraged: false,
		is_destroyed: false,
		created_at: '2026-01-01T00:00:00Z',
		destroyed_at: null,
		marginalia: [],
		secret_count: 0,
		...overrides,
	};
}

function plan(overrides: Partial<Plan> = {}): Plan {
	return {
		id: 1,
		game_id: 1,
		plan_type: 'make_demands',
		category: 'power',
		preparer_id: 1,
		target_player_id: null,
		target_asset_id: null,
		row_number: 1,
		row_order: 0,
		prepared_at_row: 1,
		status: 'resolved',
		result: 'make',
		resolved_at: '2026-01-01T00:00:00Z',
		preparation_notes: null,
		resolution_data: null,
		targeted_plan_id: null,
		...overrides,
	};
}

function player(id: number, name: string): Player {
	return {
		id,
		game_id: 1,
		account_id: id,
		display_name: name,
		joined_at: '2026-01-01T00:00:00Z',
		is_facilitator: false,
		token_color: null,
		seat_order: null,
	};
}

// ── parseResolutionData ──────────────────────────────────────────────────────

describe('parseResolutionData', () => {
	it('returns {} for null plan or null resolution_data', () => {
		expect(parseResolutionData(null)).toEqual({});
		expect(parseResolutionData(undefined)).toEqual({});
		expect(parseResolutionData(plan({ resolution_data: null }))).toEqual({});
	});
	it('returns {} for malformed JSON rather than throwing', () => {
		expect(parseResolutionData(plan({ resolution_data: '{not json' }))).toEqual({});
	});
	it('parses well-formed JSON', () => {
		const rd = JSON.stringify({ spread_rumors: { source_hidden: true } });
		expect(parseResolutionData(plan({ resolution_data: rd }))).toEqual({
			spread_rumors: { source_hidden: true },
		});
	});
});

// ── parseSpreadRumorsData (take-asset consent) ───────────────────────────────

describe('parseSpreadRumorsData', () => {
	it('returns {} when there is no spread_rumors data', () => {
		expect(parseSpreadRumorsData(plan())).toEqual({});
	});
	it('surfaces an open take-asset consent request', () => {
		const rd = JSON.stringify({
			spread_rumors: {
				pending_take_consent: {
					choices: ['take_asset', 'leverage_target'],
					result: 'make',
					asset_ids: [7, 9],
					victim_id: 2,
					requested_by: 1,
				},
			},
		});
		const sr = parseSpreadRumorsData(plan({ resolution_data: rd }));
		expect(sr.pending_take_consent?.victim_id).toBe(2);
		expect(sr.pending_take_consent?.asset_ids).toEqual([7, 9]);
	});
	it('surfaces the denied flag once the owner refuses', () => {
		const rd = JSON.stringify({ spread_rumors: { take_asset_denied: true } });
		expect(parseSpreadRumorsData(plan({ resolution_data: rd })).take_asset_denied).toBe(true);
	});
});

// ── demandWinnersFromPlan ────────────────────────────────────────────────────

describe('demandWinnersFromPlan', () => {
	it('returns an empty map when there are no draft choices', () => {
		expect(demandWinnersFromPlan(plan())).toEqual({});
	});
	it('maps each known option to its winning player', () => {
		const rd = JSON.stringify({
			make_demands: {
				draft_choices: [
					{ player_id: 7, option: 'control_leverage' },
					{ player_id: 9, option: 'keep_assets' },
				],
			},
		});
		expect(demandWinnersFromPlan(plan({ resolution_data: rd }))).toEqual({
			control_leverage: 7,
			keep_assets: 9,
		});
	});
	it('ignores unknown option keys', () => {
		const rd = JSON.stringify({
			make_demands: { draft_choices: [{ player_id: 3, option: 'bogus' }] },
		});
		expect(demandWinnersFromPlan(plan({ resolution_data: rd }))).toEqual({});
	});
	it('lets a later pick overwrite an earlier one for the same option', () => {
		const rd = JSON.stringify({
			make_demands: {
				draft_choices: [
					{ player_id: 1, option: 'perform_steps' },
					{ player_id: 2, option: 'perform_steps' },
				],
			},
		});
		expect(demandWinnersFromPlan(plan({ resolution_data: rd }))).toEqual({
			perform_steps: 2,
		});
	});
});

// ── activeDemandAgainst ──────────────────────────────────────────────────────

describe('activeDemandAgainst', () => {
	const target = plan({ id: 100, plan_type: 'propose_decree' });

	it('finds a resolved+made demand targeting the plan', () => {
		const demand = plan({
			id: 5,
			plan_type: 'make_demands',
			targeted_plan_id: 100,
			status: 'resolved',
			result: 'make',
		});
		expect(activeDemandAgainst(target, [demand, target])?.id).toBe(5);
	});

	it('ignores a marred demand', () => {
		const demand = plan({
			id: 5,
			plan_type: 'make_demands',
			targeted_plan_id: 100,
			status: 'resolved',
			result: 'mar',
		});
		expect(activeDemandAgainst(target, [demand])).toBeNull();
	});

	it('ignores a still-pending demand', () => {
		const demand = plan({
			id: 5,
			plan_type: 'make_demands',
			targeted_plan_id: 100,
			status: 'pending',
			result: null,
		});
		expect(activeDemandAgainst(target, [demand])).toBeNull();
	});

	it('ignores a demand aimed at a different plan', () => {
		const demand = plan({
			id: 5,
			plan_type: 'make_demands',
			targeted_plan_id: 999,
			status: 'resolved',
			result: 'make',
		});
		expect(activeDemandAgainst(target, [demand])).toBeNull();
	});
});

// ── name lookups ─────────────────────────────────────────────────────────────

describe('playerName / assetName', () => {
	it('returns "?" for null ids', () => {
		expect(playerName([player(1, 'Alice')], null)).toBe('?');
		expect(assetName([asset({ id: 1, name: 'Crown' })], null)).toBe('?');
	});
	it('returns "?" when the id is not found', () => {
		expect(playerName([player(1, 'Alice')], 2)).toBe('?');
		expect(assetName([asset({ id: 1, name: 'Crown' })], 2)).toBe('?');
	});
	it('returns the matching name', () => {
		expect(playerName([player(1, 'Alice')], 1)).toBe('Alice');
		expect(assetName([asset({ id: 1, name: 'Crown' })], 1)).toBe('Crown');
	});
});

// ── marginalia helpers ───────────────────────────────────────────────────────

describe('intactMarginalia', () => {
	it('returns [] for a null owner', () => {
		expect(intactMarginalia([asset()], null)).toEqual([]);
	});

	it('collects untorn marginalia from a owner non-destroyed assets, with parent info', () => {
		const a = asset({
			id: 4,
			owner_id: 2,
			name: 'Diary',
			marginalia: [
				marg({ id: 10, asset_id: 4, text: 'kept' }),
				marg({ id: 11, asset_id: 4, text: 'gone', is_torn: true }),
			],
		});
		const result = intactMarginalia([a], 2);
		expect(result).toHaveLength(1);
		expect(result[0]).toMatchObject({ id: 10, text: 'kept', assetName: 'Diary', assetID: 4 });
	});

	it('skips destroyed assets and other owners', () => {
		const destroyed = asset({ id: 1, owner_id: 2, is_destroyed: true, marginalia: [marg()] });
		const otherOwner = asset({ id: 2, owner_id: 3, marginalia: [marg({ id: 20, asset_id: 2 })] });
		expect(intactMarginalia([destroyed, otherOwner], 2)).toEqual([]);
	});
});

describe('assetsWithIntactMarginalia', () => {
	const withMarg = asset({ id: 1, owner_id: 2, marginalia: [marg({ id: 1, asset_id: 1 })] });
	const allTorn = asset({ id: 2, owner_id: 2, marginalia: [marg({ id: 2, asset_id: 2, is_torn: true })] });
	const destroyed = asset({ id: 3, owner_id: 2, is_destroyed: true, marginalia: [marg({ id: 3, asset_id: 3 })] });
	const otherOwner = asset({ id: 4, owner_id: 9, marginalia: [marg({ id: 4, asset_id: 4 })] });

	it('keeps only non-destroyed assets that still have an untorn marginalium', () => {
		const result = assetsWithIntactMarginalia([withMarg, allTorn, destroyed]);
		expect(result.map(a => a.id)).toEqual([1]);
	});

	it('narrows to a single owner when ownerID is given', () => {
		const result = assetsWithIntactMarginalia([withMarg, otherOwner], 2);
		expect(result.map(a => a.id)).toEqual([1]);
	});
});

// ── owner asset filters ──────────────────────────────────────────────────────

describe('playersExcept', () => {
	const players = [player(1, 'A'), player(2, 'B'), player(3, 'C')];
	it('drops the excluded player', () => {
		expect(playersExcept(players, 2).map(p => p.id)).toEqual([1, 3]);
	});
	it('returns everyone when exclude is null', () => {
		expect(playersExcept(players, null).map(p => p.id)).toEqual([1, 2, 3]);
	});
});

describe('ownerUnleveragedAssets', () => {
	it('returns [] for a null owner', () => {
		expect(ownerUnleveragedAssets([asset()], null)).toEqual([]);
	});
	it('keeps only intact, un-leveraged assets of the owner', () => {
		const assets = [
			asset({ id: 1, owner_id: 2 }),
			asset({ id: 2, owner_id: 2, is_leveraged: true }),
			asset({ id: 3, owner_id: 2, is_destroyed: true }),
			asset({ id: 4, owner_id: 9 }),
		];
		expect(ownerUnleveragedAssets(assets, 2).map(a => a.id)).toEqual([1]);
	});
});

describe('ownerIntactAssets', () => {
	it('returns [] for a null owner', () => {
		expect(ownerIntactAssets([asset()], null)).toEqual([]);
	});
	it('keeps intact assets of the owner regardless of leverage', () => {
		const assets = [
			asset({ id: 1, owner_id: 2 }),
			asset({ id: 2, owner_id: 2, is_leveraged: true }),
			asset({ id: 3, owner_id: 2, is_destroyed: true }),
			asset({ id: 4, owner_id: 9 }),
		];
		expect(ownerIntactAssets(assets, 2).map(a => a.id)).toEqual([1, 2]);
	});
});
