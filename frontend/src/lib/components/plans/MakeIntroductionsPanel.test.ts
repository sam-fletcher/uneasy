import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import MakeIntroductionsPanel from './MakeIntroductionsPanel.svelte';
import type { PlanContext } from './types';

// A Make Demands perform_steps win hands the whole resolution to the winner and
// locks the preparer out (the server 403s them). The arrival form must follow
// the winner, or a winner-driven plan shows no way forward.

const PREPARER = 1;
const WINNER = 2;

function renderFor(currentPlayerID: number): string {
	const target = {
		id: 9, plan_type: 'make_introductions', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x',
		resolution_data: JSON.stringify({
			make_introductions: {
				peer_count: 1, make_pending: true,
				drafts: [{ id: 'd1', name: 'Lady Vane' }],
			},
			make_mar_choices: [{ option: 'peers_arrive', player_id: PREPARER }],
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
		rollActive: false, rollOutcome: 'make', activeRoll: null,
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
