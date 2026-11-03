import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import SpreadPropagandaPanel from './SpreadPropagandaPanel.svelte';
import type { PlanContext } from './types';

// The server records a mar committed with nothing picked as choice_submitted, with
// make_mar_choices empty. The panel must read that as "choices made" (done-state comes
// from the server), not show the picker again.

const PREPARER = 1;

function renderMar(resolution: Record<string, unknown>): string {
	const plan = {
		id: 9, plan_type: 'spread_propaganda', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x', resolution_data: JSON.stringify(resolution),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never,
		assets: [],
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: 2, display_name: 'sol' }] as never,
		rankings: [], currentPlayerID: PREPARER, isFocusPlayer: false,
		rollActive: false, rollOutcome: 'mar',
		activeRoll: { id: 4, plan_id: 9, status: 'resolved' } as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(SpreadPropagandaPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Spread Propaganda mar after a zero-pick commit', () => {
	it('shows the picker while nothing is committed', () => {
		expect(renderMar({ spread_propaganda: { pre_roll_done: true } })).toContain('Apply choices');
	});

	it('moves on to the follow-up steps, not the picker', () => {
		const body = renderMar({ spread_propaganda: { pre_roll_done: true }, choice_submitted: true });
		expect(body).not.toContain('Apply choices');
		expect(body).toContain('Complete plan');
	});
});
