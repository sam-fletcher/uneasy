import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ProposeDecreePanel from './ProposeDecreePanel.svelte';
import type { PlanContext } from './types';

// Propose Decree showed no difficulty until the dice panel, and nothing said what a
// make or a mar of a law would do. Before the dice land (the council meeting, and the
// roll itself) everyone now sees the difficulty (the preparer's power rank) and a
// "What can happen?" from the book's make and mar.

const PREPARER = 1;
const OTHER = 2;
const BYSTANDER = 3;
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

function renderPD(opts: { seat: number; phase: 'council' | 'rolling' | 'passed'; outcome?: 'make' | 'mar' | null }): string {
	const pd: Record<string, unknown> = {
		signatory_id: PREPARER, signatory_player_ids: [PREPARER, OTHER], debate_started: true, law_text: 'No private armies.',
	};
	if (opts.phase === 'passed') pd.outcome = opts.outcome ?? 'make';
	const plan = {
		id: 9, plan_type: 'propose_decree', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'No private armies.', resolution_data: JSON.stringify({ propose_decree: pd }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: [] as never,
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }, { id: BYSTANDER, display_name: 'mei' }] as never,
		rankings: [
			{ id: 1, game_id: 1, category: 'power', player_id: PREPARER, rank: 3 },
			{ id: 2, game_id: 1, category: 'power', player_id: OTHER, rank: 2 },
			{ id: 3, game_id: 1, category: 'power', player_id: BYSTANDER, rank: 4 },
		] as never,
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: opts.phase === 'rolling', rollOutcome: opts.phase === 'passed' ? (opts.outcome ?? 'make') : null,
		activeRoll: (opts.phase === 'rolling' ? { id: 4, plan_id: 9, difficulty: 3, adjusted_difficulty: null, result: null } : null) as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(ProposeDecreePanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Propose Decree: the difficulty before the dice', () => {
	it.each(['council', 'rolling'] as const)('shows everyone the preparer\'s power rank in the %s step', (phase) => {
		for (const seat of [PREPARER, OTHER, BYSTANDER]) {
			const t = textOf(renderPD({ seat, phase }));
			expect(t).toContain('Difficulty 3');
			expect(t).toContain('ravi’s rank on the power track: 3');
		}
	});

	it('is gone once the dice have landed and the law is being written', () => {
		for (const o of ['make', 'mar'] as const) {
			expect(textOf(renderPD({ seat: BYSTANDER, phase: 'passed', outcome: o }))).not.toContain('1 easy');
		}
	});
});

describe('Propose Decree: "What can happen?"', () => {
	const toggle = /<button[^>]*aria-expanded="false"[^>]*>[\s\S]*?What can happen\?/;

	it.each(['council', 'rolling'] as const)('is a collapsed row for everyone in the %s step', (phase) => {
		for (const seat of [PREPARER, OTHER, BYSTANDER]) expect(renderPD({ seat, phase })).toMatch(toggle);
	});

	it('is gone once the dice have landed: the law-writing steps say what is happening', () => {
		for (const o of ['make', 'mar'] as const) expect(renderPD({ seat: OTHER, phase: 'passed', outcome: o })).not.toContain('What can happen?');
	});
});
