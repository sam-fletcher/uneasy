import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { activeWarPlans, warGateActive, warGatePlans } from './warGate';
import type { Plan, RowState, WarStateResponse } from '$lib/api';

// Minimal builders; only the fields the helpers read matter.
function plan(id: number, over: Partial<Plan> = {}): Plan {
	return { id, plan_type: 'make_war', status: 'resolved', row_number: 2, ...over } as Plan;
}
function war(originPlanID: number, over: Partial<WarStateResponse> = {}): WarStateResponse {
	return {
		war_id: originPlanID * 10,
		origin_plan_id: originPlanID,
		status: 'active',
		outstanding_costs: [],
		open_claims: [],
		...over,
	} as WarStateResponse;
}
const owes = [{ payer_id: 1, opponent_id: 2 }];
const rs = (kind: string): RowState => ({ kind }) as RowState;

describe('warGateActive', () => {
	it('is true for the two war gates the war panel can discharge', () => {
		expect(warGateActive(rs('await_battle_cost'))).toBe(true);
		expect(warGateActive(rs('await_surrender_claim'))).toBe(true);
	});

	it('is false for every other kind, and while the row state is loading', () => {
		for (const kind of ['scene_setting', 'plan_resolving', 'await_delay_reveal', 'await_dice_roll']) {
			expect(warGateActive(rs(kind))).toBe(false);
		}
		expect(warGateActive(null)).toBe(false);
		expect(warGateActive(undefined)).toBe(false);
	});
});

describe('warGatePlans', () => {
	// The bug this guards: the cost of battle falls on rows AFTER the war's own,
	// when the Make War plan is long since 'resolved'. A routing that depended on
	// the plan still resolving (as the plan registry does) shows nothing here.
	it('routes a RESOLVED plan whose war owes a cost — the original blank play area', () => {
		const out = warGatePlans(rs('await_battle_cost'), [war(7, { outstanding_costs: owes })], [plan(7)]);
		expect(out.map(p => p.id)).toEqual([7]);
	});

	it('routes a surrender claim, which carries no war_id on the row state', () => {
		const claims = [{ id: 1, surrendered_id: 1, claimant_id: 2 }];
		const out = warGatePlans(rs('await_surrender_claim'), [war(7, { open_claims: claims })], [plan(7)]);
		expect(out.map(p => p.id)).toEqual([7]);
	});

	it('surfaces every war that owes something, not just the one the row state names', () => {
		const wars = [war(7, { outstanding_costs: owes }), war(8, { outstanding_costs: owes })];
		const out = warGatePlans(rs('await_battle_cost'), wars, [plan(7), plan(8)]);
		expect(out.map(p => p.id)).toEqual([7, 8]);
	});

	it('skips a war with nothing outstanding, and an ended war', () => {
		const wars = [
			war(7),
			war(8, { status: 'ended', outstanding_costs: owes }),
			war(9, { outstanding_costs: owes }),
		];
		const out = warGatePlans(rs('await_battle_cost'), wars, [plan(7), plan(8), plan(9)]);
		expect(out.map(p => p.id)).toEqual([9]);
	});

	it('returns nothing when no war gate is open', () => {
		const wars = [war(7, { outstanding_costs: owes })];
		expect(warGatePlans(rs('scene_setting'), wars, [plan(7)])).toEqual([]);
		expect(warGatePlans(null, wars, [plan(7)])).toEqual([]);
	});

	it('returns nothing while the war list is still loading, rather than a placeholder', () => {
		expect(warGatePlans(rs('await_battle_cost'), [], [plan(7)])).toEqual([]);
	});

	it('drops a war whose origin plan is not in the plan list', () => {
		expect(warGatePlans(rs('await_battle_cost'), [war(7, { outstanding_costs: owes })], [])).toEqual([]);
	});
});

describe('activeWarPlans', () => {
	it('keeps a war that is under way, resolved or still pending', () => {
		const plans = [plan(7), plan(8, { status: 'pending', row_number: 5 })];
		expect(activeWarPlans(plans, [war(7), war(8)]).map(p => p.id)).toEqual([7, 8]);
	});

	// "Active Wars (1)" over an empty sheet: listWars only returns ACTIVE wars,
	// so a finished war has no entry — and the old filter, which looked for an
	// entry marked 'ended', could never match it.
	it('drops a finished war, which is simply absent from the active-war list', () => {
		expect(activeWarPlans([plan(7)], [])).toEqual([]);
	});

	it('drops a cancelled declaration and a plan still in its delay reveal', () => {
		const plans = [plan(7, { status: 'cancelled' }), plan(8, { row_number: null })];
		expect(activeWarPlans(plans, [war(7), war(8)])).toEqual([]);
	});

	it('ignores other plan types', () => {
		expect(activeWarPlans([plan(7, { plan_type: 'seek_answers' })], [war(7)])).toEqual([]);
	});
});

// The pure functions above only protect the play area if the view still calls
// them. MainEventView is too heavy to render without a browser and its war list
// arrives through an async fetch, so this guards the wiring at the source: the
// takeover branch must exist, render the Make War panel, and sit AHEAD of the
// scene branches — the server's precedence puts the war gate above them.
describe('MainEventView war-gate wiring', () => {
	const src = readFileSync(join(__dirname, 'components/phases/MainEventView.svelte'), 'utf8');

	it('renders the Make War panel from a war-gate branch', () => {
		const branch = src.indexOf('{:else if warGateTakeover}');
		expect(branch, 'the {:else if warGateTakeover} branch is gone').toBeGreaterThan(-1);
		const body = src.slice(branch, src.indexOf('{:else if', branch + 10));
		expect(body).toContain('planType="make_war"');
		expect(body).toContain('warGateWarPlans');
	});

	it('puts the war gate ahead of the scene branches', () => {
		const gate = src.indexOf('{:else if warGateTakeover}');
		expect(gate).toBeGreaterThan(-1);
		expect(gate).toBeLessThan(src.indexOf("{:else if activeScene && activeScene.kind === 'turn'}"));
		expect(gate).toBeLessThan(src.indexOf("{:else if rowState?.kind === 'scene_setting'"));
	});

	it('derives the takeover from the shared helpers', () => {
		expect(src).toMatch(/warGateActive\(rowState\)/);
		expect(src).toMatch(/warGatePlans\(rowState, wars, plans\)/);
		expect(src).toMatch(/activeWarPlans\(plans, wars\)/);
	});
});
