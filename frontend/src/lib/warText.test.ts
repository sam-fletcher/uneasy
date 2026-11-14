import { describe, expect, it } from 'vitest';
import type { Plan, Player, WarStateResponse } from '$lib/api';
import {
	entryOpponents, joinChoices, nameList, sideLeader, warLede, warLeaders, warName, warStatusWord,
} from './warText';

// The war band and the war's panel say the same few things in many places, so
// the wording lives in warText.ts and is pinned here.

const players = [
	{ id: 1, display_name: 'otto' },
	{ id: 2, display_name: 'Wren' },
	{ id: 3, display_name: 'Mira' },
	{ id: 4, display_name: 'Ash' },
] as Player[];

const plan = (over: Partial<Plan> = {}, enemies: number[] = [2]) => ({
	id: 9,
	plan_type: 'make_war',
	preparer_id: 1,
	status: 'resolved',
	row_number: 4,
	resolution_data: JSON.stringify({ make_war: { enemy_player_ids: enemies } }),
	...over,
}) as Plan;

const part = (player_id: number, side: 1 | 2, over: Record<string, unknown> = {}) => ({
	player_id, side, joined_at_row: 1, surrendered_at_row: null, entry_payment_complete: true, ...over,
});
const war = (participants: ReturnType<typeof part>[], over: Partial<WarStateResponse> = {}) => ({
	war_id: 1, origin_plan_id: 9, status: 'active', started_at_row: 4, ended_at_row: null,
	end_reason: null, current_row: 5, participants, battle_costs: [], outstanding_costs: [],
	open_proposal: null, open_claims: [], ...over,
}) as unknown as WarStateResponse;

describe('a war is named for its declarer and the first player they named', () => {
	it('"otto’s war on Wren"', () => {
		expect(warName(plan(), players)).toBe('otto’s war on Wren');
	});

	it('adds "and others" only when the declarer named more than one', () => {
		expect(warName(plan({}, [2, 3]), players)).toBe('otto’s war on Wren and others');
		expect(warName(plan({}, [2]), players)).not.toContain('others');
	});

	it('names the FIRST player named, whatever the seat order', () => {
		expect(warName(plan({}, [3, 2]), players)).toBe('otto’s war on Mira and others');
	});

	it('still reads when the plan does not say who was named', () => {
		expect(warName(plan({ resolution_data: null }), players)).toBe('otto’s war');
	});
});

describe('the sides are named after their leaders', () => {
	it('side 1 is the declarer and side 2 the first enemy', () => {
		const l = warLeaders(plan(), players);
		expect(sideLeader(1, l)).toBe('otto');
		expect(sideLeader(2, l)).toBe('Wren');
	});
});

describe('where the war stands', () => {
	it('a pending plan is planned and a resolved one is under way', () => {
		expect(warStatusWord(plan({ status: 'pending' }))).toBe('planned');
		expect(warStatusWord(plan({ status: 'resolved' }))).toBe('under way');
		expect(warStatusWord(plan({ status: 'resolving' }))).toBe('under way');
	});

	it('the lede says when it began, or when it is planned for, and what being in it costs', () => {
		expect(warLede(plan(), war([]))).toBe(
			'Under way since row 4. Everyone in it pays the cost of battle each time the record advances.');
		expect(warLede(plan({ status: 'pending', row_number: 7 }), war([]))).toBe(
			'Planned for row 7. Once it begins, everyone in it pays the cost of battle each time the record advances.');
		// It began on its plan's row, not on the row it was declared (the war record's).
		expect(warLede(plan({ row_number: 6 }), war([], { started_at_row: 2 }))).toMatch(/^Under way since row 6\./);
		// Before the war state arrives the lede still says something true.
		expect(warLede(plan(), null)).toMatch(/^Under way since row 4\./);
		expect(warLede(plan({ row_number: null }), null)).toMatch(/^Under way\. /);
	});
});

describe('the entry cost', () => {
	it('is owed to every opposing participant who is fully in and has not surrendered', () => {
		const w = war([
			part(1, 1), part(2, 2), part(3, 2, { surrendered_at_row: 5 }),
			part(4, 2, { entry_payment_complete: false }),
		]);
		expect(entryOpponents(w, 1)).toEqual([2]);
		// From side 2, the opponents are side 1's.
		expect(entryOpponents(w, 2)).toEqual([1]);
	});

	it('lists names in words', () => {
		expect(nameList(['Wren'])).toBe('Wren');
		expect(nameList(['Wren', 'Mira'])).toBe('Wren and Mira');
		expect(nameList(['Wren', 'Mira', 'Ash'])).toBe('Wren, Mira and Ash');
	});
});

describe('the three choices a bystander has', () => {
	const w = war([part(1, 1), part(2, 2)]);

	it('are named after the leaders, with a button that names the commitment', () => {
		const c = joinChoices(w, plan(), players, true);
		expect(c.map(x => x.label)).toEqual(['Join otto', 'Join Wren', 'Stay out']);
		expect(c.map(x => x.button)).toEqual(['Join otto’s side', 'Join Wren’s side', 'Stay out']);
		expect(c.map(x => x.side)).toEqual([1, 2, null]);
	});

	it('never say "Side 1" or "Side 2"', () => {
		for (const x of joinChoices(w, plan(), players, true)) {
			expect(`${x.label} ${x.button} ${x.cost}`).not.toMatch(/Side \d/);
		}
	});

	it('price the entry in the opponents already in, per side, once the war has begun', () => {
		const [join1, join2] = joinChoices(w, plan(), players, true);
		expect(join1.cost).toContain('now Wren: break one of your assets, or leverage two');
		expect(join2.cost).toContain('now otto: break one of your assets, or leverage two');
		for (const x of [join1, join2]) expect(x.cost).toContain('pay the cost of battle each time the record advances');
	});

	it('name every opponent when there are several, and skip one who has surrendered', () => {
		const many = war([part(1, 1), part(2, 2), part(3, 2), part(4, 2, { surrendered_at_row: 5 })]);
		expect(joinChoices(many, plan(), players, true)[0].cost).toContain('now Wren and Mira:');
	});

	it('say joining is free while the delay is still being voted', () => {
		const [join1] = joinChoices(w, plan({ row_number: null }), players, false);
		expect(join1.cost).toContain('free while the war’s delay is still being voted');
		expect(join1.cost).not.toContain('entry payment');
		// ...and once the vote has closed, a war that is only planned already charges.
		const [planned] = joinChoices(w, plan({ status: 'pending', row_number: 7 }), players, true);
		expect(planned.cost).toContain('entry payment');
		expect(planned.cost).not.toContain('free');
	});

	it('say staying out is only a note, and that the war can still be joined', () => {
		const stay = joinChoices(w, plan(), players, true)[2];
		expect(stay.cost).toBe('Staying out tells the table you won’t join. You can still join later, while the war lasts.');
	});
});
