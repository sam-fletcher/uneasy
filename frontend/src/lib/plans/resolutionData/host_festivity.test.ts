// host_festivity.test.ts — which plan the play area shows while a festivity's
// duel is running. Both are `resolving`; only the duel is being played.

import { describe, it, expect } from 'vitest';
import type { Plan } from '$lib/api';
import { activeResolvingPlan } from './host_festivity';

const plan = (id: number, plan_type: string, status: string, resolution_data?: object): Plan =>
	({
		id, plan_type, status,
		resolution_data: resolution_data ? JSON.stringify(resolution_data) : null,
	}) as unknown as Plan;

describe('activeResolvingPlan', () => {
	it('is the one resolving plan when there is just one', () => {
		const plans = [plan(1, 'host_festivity', 'resolving'), plan(2, 'propose_duel', 'pending')];
		expect(activeResolvingPlan(plans)?.id).toBe(1);
	});

	it('is null when nothing is resolving', () => {
		expect(activeResolvingPlan([plan(1, 'host_festivity', 'resolved')])).toBeNull();
	});

	it('picks the nested duel over the festivity beneath it, whatever the list order', () => {
		const festivity = plan(1, 'host_festivity', 'resolving', { festivity: { pending_duel_plan_id: 2 } });
		const duel = plan(2, 'propose_duel', 'resolving');
		expect(activeResolvingPlan([festivity, duel])?.id).toBe(2);
		expect(activeResolvingPlan([duel, festivity])?.id).toBe(2);
	});

	it('returns to the festivity once the duel has resolved', () => {
		const festivity = plan(1, 'host_festivity', 'resolving', { festivity: { pending_duel_plan_id: 2 } });
		const duel = plan(2, 'propose_duel', 'resolved');
		expect(activeResolvingPlan([festivity, duel])?.id).toBe(1);
	});

	it('does not suspend a festivity for an unrelated resolving plan', () => {
		const festivity = plan(1, 'host_festivity', 'resolving', { festivity: { pending_duel_plan_id: 9 } });
		expect(activeResolvingPlan([festivity, plan(3, 'propose_duel', 'resolving')])?.id).toBe(1);
	});
});
