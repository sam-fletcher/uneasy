import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import MakeIntroductionsPanel from './MakeIntroductionsPanel.svelte';
import type { PlanContext } from './types';

// A Make Demands perform_steps win hands the whole resolution to the winner and
// locks the preparer out (the server 403s them). The arrival form must follow
// the winner, or a winner-driven plan shows no way forward.

const PREPARER = 1;
const WINNER = 2;

type Branch = { rollOutcome: 'make' | 'mar' | null; mi: Record<string, unknown>; choices?: unknown[] };

const MAKE: Branch = {
	rollOutcome: 'make',
	mi: { peer_count: 1, make_pending: true, drafts: [{ id: 'd1', name: 'Lady Vane' }] },
	choices: [{ option: 'peers_arrive', player_id: PREPARER }],
};
const NAMING: Branch = { rollOutcome: null, mi: { peer_count: 1 } };
const MAR: Branch = {
	rollOutcome: 'mar',
	mi: { peer_count: 1, mar_pending: true, drafts: [{ id: 'd1', name: 'Lady Vane' }] },
};

function renderFor(currentPlayerID: number, branch: Branch = MAKE): string {
	const target = {
		id: 9, plan_type: 'make_introductions', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x',
		resolution_data: JSON.stringify({
			make_introductions: branch.mi,
			make_mar_choices: branch.choices ?? [],
		}),
	};
	const demand = {
		id: 12, plan_type: 'make_demands', status: 'resolved', result: 'make',
		preparer_id: WINNER, targeted_plan_id: 9,
		resolution_data: JSON.stringify({
			make_demands: { draft_choices: [{ option: 'perform_steps', player_id: WINNER }] },
		}),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [target, demand] as never,
		assets: [],
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: WINNER, display_name: 'sol' }] as never,
		rankings: [], currentPlayerID, isFocusPlayer: false,
		rollActive: false, rollOutcome: branch.rollOutcome, activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(MakeIntroductionsPanel, { props: { ctx, plan: target as never, mode: 'resolve' } }).body;
}

describe('MakeIntroductionsPanel with a perform_steps winner', () => {
	it('gives the arrival form to the winner', () => {
		expect(renderFor(WINNER)).toContain('They arrive');
	});

	it('shows the preparer a wait on the winner, not a form', () => {
		const body = renderFor(PREPARER);
		expect(body).not.toContain('They arrive');
		expect(body).toContain('sol is welcoming the new peers');
	});
});

describe('MakeIntroductionsPanel pre-roll naming with a perform_steps winner', () => {
	it('keeps the naming form with the preparer', () => {
		expect(renderFor(PREPARER, NAMING)).toContain('Name peers &amp; roll');
	});

	it('shows the winner a wait on the preparer, not the form', () => {
		const body = renderFor(WINNER, NAMING);
		expect(body).not.toContain('Name peers');
		expect(body).toContain('ravi is naming the new peers');
	});
});

describe('MakeIntroductionsPanel mar fates with a perform_steps winner', () => {
	it('gives the resolve form to the winner', () => {
		expect(renderFor(WINNER, MAR)).toContain('Resolve peer');
	});

	it('shows the preparer a wait on the winner', () => {
		const body = renderFor(PREPARER, MAR);
		expect(body).not.toContain('Resolve peer');
		expect(body).toContain('sol is resolving the marred introductions');
	});
});
