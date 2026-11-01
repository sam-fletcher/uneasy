import { describe, it, expect } from 'vitest';
import type { Asset, Marginalium, Plan, Player } from '$lib/api';
import {
	parseResolutionData,
	choicesCommitted,
	demandWinnersFromPlan,
	activeDemandAgainst,
	resolutionActorID,
	playerName,
	assetName,
	intactMarginalia,
	assetsWithIntactMarginalia,
	breakableAssets,
	breaksAvailable,
	playersExcept,
	ownerUnleveragedAssets,
	ownerIntactAssets,
	finaleSlotSpent,
	demandOptionInertReason,
	draftOptionViews,
	pickAfterTap,
	DEMAND_OPTIONS,
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
		title: null,
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
		linked_card_suit: null,
		linked_card_value: null,
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
		is_finale_bonus: false,
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

	it('keeps only non-destroyed assets that still have an untorn marginalia', () => {
		const result = assetsWithIntactMarginalia([withMarg, allTorn, destroyed]);
		expect(result.map(a => a.id)).toEqual([1]);
	});

	it('narrows to a single owner when ownerID is given', () => {
		const result = assetsWithIntactMarginalia([withMarg, otherOwner], 2);
		expect(result.map(a => a.id)).toEqual([1]);
	});

	it('excludes a blank asset — it has no marginalia line to pick', () => {
		const blank = asset({ id: 5, owner_id: 2, marginalia: [] });
		expect(assetsWithIntactMarginalia([withMarg, blank]).map(a => a.id)).toEqual([1]);
	});
});

describe('breakableAssets', () => {
	const withMarg = asset({ id: 1, owner_id: 2, marginalia: [marg({ id: 1, asset_id: 1 })] });
	const blank = asset({ id: 2, owner_id: 2, marginalia: [] });
	const allTorn = asset({ id: 3, owner_id: 2, marginalia: [marg({ id: 3, asset_id: 3, is_torn: true })] });
	const destroyed = asset({ id: 4, owner_id: 2, is_destroyed: true, marginalia: [marg({ id: 4, asset_id: 4 })] });
	const otherOwner = asset({ id: 5, owner_id: 9, marginalia: [] });

	it('admits both an asset with intact marginalia and a blank one', () => {
		const result = breakableAssets([withMarg, blank, allTorn, destroyed]);
		expect(result.map(a => a.id)).toEqual([1, 2]);
	});

	it('excludes an all-torn (but undestroyed) asset — nothing left to break', () => {
		expect(breakableAssets([allTorn])).toEqual([]);
	});

	it('narrows to a single owner when ownerID is given', () => {
		expect(breakableAssets([blank, otherOwner], 2).map(a => a.id)).toEqual([2]);
	});
});

describe('breaksAvailable', () => {
	it('counts one break per untorn marginalia', () => {
		const a = asset({
			id: 1,
			marginalia: [
				marg({ id: 1, asset_id: 1 }),
				marg({ id: 2, asset_id: 1 }),
				marg({ id: 3, asset_id: 1, is_torn: true }),
			],
		});
		expect(breaksAvailable(a)).toBe(2);
	});

	it('gives a blank asset exactly one break — it destroys the asset', () => {
		expect(breaksAvailable(asset({ id: 1, marginalia: [] }))).toBe(1);
	});

	it('gives an all-torn asset none', () => {
		expect(breaksAvailable(asset({ id: 1, marginalia: [marg({ id: 1, asset_id: 1, is_torn: true })] }))).toBe(0);
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

// The slot is derived from plans, never from a flag on the player, so this
// mirrors the server's CountFinaleBonusPlans clause for clause — including the
// `status != cancelled` filter, which is vacuous today (a bonus plan cannot
// fall through) but is the behaviour we would want if it ever could.
describe('finaleSlotSpent', () => {
	it('is false for a null player', () => {
		expect(finaleSlotSpent([plan({ preparer_id: 2, is_finale_bonus: true })], null)).toBe(false);
	});
	it('is false when the player holds no bonus plan', () => {
		expect(finaleSlotSpent([plan({ preparer_id: 2 })], 2)).toBe(false);
	});
	it('is true when the player holds one', () => {
		expect(finaleSlotSpent([plan({ preparer_id: 2, is_finale_bonus: true })], 2)).toBe(true);
	});
	it('ignores another player’s bonus plan', () => {
		expect(finaleSlotSpent([plan({ preparer_id: 9, is_finale_bonus: true })], 2)).toBe(false);
	});
	it('ignores a bonus plan that fell through', () => {
		const plans = [plan({ preparer_id: 2, is_finale_bonus: true, status: 'cancelled' })];
		expect(finaleSlotSpent(plans, 2)).toBe(false);
	});
});

// The draft picker shows an option that would do nothing against the target as
// greyed out with the reason, instead of letting the drafter spend a pick on it.
describe('demandOptionInertReason', () => {
	it('control_leverage is inert on the three targets with no roll of the preparer’s own', () => {
		for (const t of ['host_festivity', 'propose_duel', 'clandestinely_liaise'] as const) {
			expect(demandOptionInertReason('control_leverage', t)).toMatch(/no dice roll/);
		}
		expect(demandOptionInertReason('control_leverage', 'spread_rumors')).toBeNull();
	});
	it('keep_or_change_target is live only where the plan is aimed at something', () => {
		for (const t of ['exchange_courtiers', 'spread_rumors', 'propose_duel', 'clandestinely_liaise'] as const) {
			expect(demandOptionInertReason('keep_or_change_target', t)).toBeNull();
		}
		expect(demandOptionInertReason('keep_or_change_target', 'propose_decree')).toMatch(/Propose Decree/);
	});
	it('keep_assets is inert where the preparer gains nothing', () => {
		expect(demandOptionInertReason('keep_assets', 'seek_answers')).toMatch(/no assets/);
		expect(demandOptionInertReason('keep_assets', 'chronicle_histories')).toMatch(/no assets/);
		expect(demandOptionInertReason('keep_assets', 'make_introductions')).toBeNull();
	});
	it('perform_steps is never inert', () => {
		expect(demandOptionInertReason('perform_steps', 'seek_answers')).toBeNull();
	});
});

describe('draftOptionViews', () => {
	it('disables the inert options while a live one remains', () => {
		const v = draftOptionViews([...DEMAND_OPTIONS], 'host_festivity');
		const by = Object.fromEntries(v.map(x => [x.option, x]));
		expect(by.control_leverage.disabled).toBe(true);
		expect(by.keep_or_change_target.disabled).toBe(true);
		expect(by.keep_assets.disabled).toBe(false);
		expect(by.perform_steps.disabled).toBe(false);
		expect(by.control_leverage.reason).not.toBeNull();
	});
	it('never blocks a pick when everything left is inert', () => {
		const v = draftOptionViews(['control_leverage', 'keep_or_change_target'], 'host_festivity');
		expect(v.every(x => !x.disabled)).toBe(true);
		expect(v.every(x => x.reason != null)).toBe(true);
	});
	it('disables nothing against a target where all four do something', () => {
		expect(draftOptionViews([...DEMAND_OPTIONS], 'spread_rumors').some(x => x.disabled)).toBe(false);
	});
});

describe('pickAfterTap', () => {
	const live = { option: 'perform_steps', reason: null, disabled: false } as const;
	const blocked = { option: 'keep_assets', reason: 'x', disabled: true } as const;
	it('takes a live card', () => {
		expect(pickAfterTap(null, live)).toBe('perform_steps');
	});
	it('ignores a blocked card, keeping the pick already made', () => {
		expect(pickAfterTap('perform_steps', blocked)).toBe('perform_steps');
		expect(pickAfterTap(null, blocked)).toBeNull();
	});
});

describe('choicesCommitted', () => {
	it('reads a zero-pick commit from the marker, not from make_mar_choices', () => {
		expect(choicesCommitted({})).toBe(false);
		expect(choicesCommitted({ choice_submitted: true })).toBe(true);
		expect(choicesCommitted({ choice_auto_applied: true })).toBe(true);
	});
	it('still reads a row stored before the markers existed', () => {
		expect(choicesCommitted({ make_mar_choices: [{ option: 'legal' }] as never })).toBe(true);
	});
});

// ── resolutionActorID ────────────────────────────────────────────────────────

describe('resolutionActorID', () => {
	const target = plan({ id: 100, plan_type: 'make_introductions', preparer_id: 1 });
	const demand = (winner: number) => plan({
		id: 5, plan_type: 'make_demands', targeted_plan_id: 100, status: 'resolved', result: 'make',
		resolution_data: JSON.stringify({ make_demands: { draft_choices: [{ option: 'perform_steps', player_id: winner }] } }),
	});

	it('is the preparer when no demand is made against the plan', () => {
		expect(resolutionActorID(target, [target])).toBe(1);
	});

	it('is the perform_steps winner once a demand has been made', () => {
		expect(resolutionActorID(target, [target, demand(3)])).toBe(3);
	});
});
