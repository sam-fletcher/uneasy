import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import MakeDemandsPanel from './MakeDemandsPanel.svelte';
import type { PlanContext } from './types';

// A demand's difficulty used to be the dice panel's bare figure, and its four options were
// described only once the draft began. Before the dice land everyone now sees how hard
// the roll is and why (the target plan's own difficulty, plus the gap in power), and a
// "What can happen?" that says what each option does against this target.

const ADA = 1; // demands
const BEA = 2; // prepared the target plan
const CYR = 3; // bystander
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

function renderMD(opts: { seat: number; outcome?: 'make' | 'mar' | null; rolled?: boolean }): string {
	const target = {
		id: 20, plan_type: 'seek_answers', status: 'pending', preparer_id: BEA, row_number: 5, resolution_data: null,
	};
	const demand = {
		id: 21, plan_type: 'make_demands', status: 'resolving', preparer_id: ADA, targeted_plan_id: 20,
		row_number: 5, preparation_notes: 'That plan is mine to direct.', resolution_data: null,
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [target, demand] as never, assets: [] as never,
		players: [{ id: ADA, display_name: 'ada' }, { id: BEA, display_name: 'bea' }, { id: CYR, display_name: 'cyr' }] as never,
		rankings: [
			{ id: 1, game_id: 1, category: 'knowledge', player_id: BEA, rank: 2 },
			{ id: 2, game_id: 1, category: 'power', player_id: BEA, rank: 1 },
			{ id: 3, game_id: 1, category: 'power', player_id: ADA, rank: 4 },
		] as never,
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: opts.outcome == null, rollOutcome: opts.outcome ?? null,
		// A demand rolls the moment it starts resolving, at the difficulty the server worked out.
		activeRoll: (opts.rolled === false ? null : { id: 4, plan_id: 21, difficulty: 5, adjusted_difficulty: null, result: null }) as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(MakeDemandsPanel, { props: { ctx, plan: demand as never, mode: 'resolve' } }).body;
}

describe('Make Demands: the difficulty before the dice', () => {
	it('shows everyone how hard the roll is, and why: the target plan\'s difficulty plus the power gap', () => {
		for (const seat of [ADA, BEA, CYR]) {
			const t = textOf(renderMD({ seat }));
			expect(t).toContain('Difficulty 5');
			expect(t).toContain('Seek Answers is difficulty 2; bea outranks ada on power by 3, which adds 3.');
			expect(t).toContain('1 easy');
		}
	});

	it('is the roll\'s own value once it exists, not the client\'s sum', () => {
		// A roll made at 6 shows 6, whatever the rankings add up to.
		const t = textOf(renderMD({ seat: CYR }));
		expect(t).toContain('Difficulty 5');
	});

	it('is gone once the dice have landed', () => {
		for (const o of ['make', 'mar'] as const) expect(textOf(renderMD({ seat: CYR, outcome: o }))).not.toContain('1 easy');
	});

	it('leaves the "Targeting" line where it was, at the top of the step', () => {
		const t = textOf(renderMD({ seat: CYR }));
		expect(t.indexOf('Targeting: Seek Answers by bea')).toBeGreaterThan(-1);
		expect(t.indexOf('Targeting: Seek Answers by bea')).toBeLessThan(t.indexOf('Difficulty 5'));
	});
});

describe('Make Demands: "What can happen?"', () => {
	const toggle = /<button[^>]*aria-expanded="false"[^>]*>[\s\S]*?What can happen\?/;

	it('is a collapsed row for everyone before the dice land', () => {
		for (const seat of [ADA, BEA, CYR]) expect(renderMD({ seat })).toMatch(toggle);
	});

	it('is gone once the dice have landed: the draft\'s cards carry the same descriptions', () => {
		for (const o of ['make', 'mar'] as const) expect(renderMD({ seat: CYR, outcome: o })).not.toContain('What can happen?');
	});
});
