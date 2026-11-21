import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import PlanDifficulty from './PlanDifficulty.svelte';

// The meter every plan draws before its dice land. What the number is lives in
// lib/plans/difficulty.test.ts; this pins that the wrapper hands it the table's state
// and draws it for a viewer who is not the preparer (it is the same for everyone).

const ctx = (over: Record<string, unknown> = {}) => ({
	plans: [], assets: [], activeRoll: null,
	players: [{ id: 1, display_name: 'ada' }, { id: 2, display_name: 'bea' }],
	rankings: [{ id: 1, game_id: 1, category: 'knowledge', player_id: 1, rank: 3 }],
	currentPlayerID: 2,
	...over,
}) as never;

const plan = (over: Record<string, unknown> = {}) => ({
	id: 10, plan_type: 'seek_answers', status: 'resolving', preparer_id: 1,
	resolution_data: null, ...over,
}) as never;

const text = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

describe('PlanDifficulty', () => {
	it('draws the number and where it came from, to a viewer who is not the preparer', () => {
		const t = text(render(PlanDifficulty, { props: { plan: plan(), ctx: ctx() } }).body);
		expect(t).toContain('Difficulty 3');
		expect(t).toContain('ada’s rank on the knowledge track: 3');
		expect(t).toContain('1 easy');
		expect(t).toContain('6 hard');
	});

	it('names no viewer: nothing in it says "you" or "your"', () => {
		const t = text(render(PlanDifficulty, { props: { plan: plan(), ctx: ctx({ currentPlayerID: 1 }) } }).body);
		expect(t).not.toMatch(/\byou(r)?\b/i);
	});

	it('takes its headline from the plan when the panel says so', () => {
		const t = text(render(PlanDifficulty, { props: { plan: plan(), ctx: ctx(), headline: 'Starting difficulty' } }).body);
		expect(t).toContain('Starting difficulty 3');
	});

	it('counts the artifacts being invoked for Chronicle Histories', () => {
		const p = plan({ plan_type: 'chronicle_histories' });
		const t = text(render(PlanDifficulty, { props: { plan: p, ctx: ctx(), invokedCount: 5 } }).body);
		expect(t).toContain('Difficulty 5');
	});

	it('draws nothing for a plan that never rolls', () => {
		const body = render(PlanDifficulty, { props: { plan: plan({ plan_type: 'make_war' }), ctx: ctx() } }).body;
		expect(text(body)).toBe('');
	});

	it('draws nothing while the ranking it reads is not on this client', () => {
		const body = render(PlanDifficulty, { props: { plan: plan(), ctx: ctx({ rankings: [] }) } }).body;
		expect(text(body)).toBe('');
	});

	it('shows the roll\'s own difficulty once the plan has rolled', () => {
		const roll = { id: 4, plan_id: 10, difficulty: 5, adjusted_difficulty: null };
		const t = text(render(PlanDifficulty, { props: { plan: plan(), ctx: ctx({ activeRoll: roll }) } }).body);
		expect(t).toContain('Difficulty 5');
	});
});
