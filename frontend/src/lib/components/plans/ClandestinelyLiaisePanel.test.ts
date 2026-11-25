import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ClandestinelyLiaisePanel from './ClandestinelyLiaisePanel.svelte';
import type { PlanContext } from './types';

// A liaison rolls nothing, so it has no make or mar to preview; its "What can happen"
// is the options of its two choices (the things we share, and meeting again). It is the
// two players' reference before they choose, so the bystander, who is deliberately given
// nothing to read here, still has nothing, and it goes once the options are on screen.

const PREPARER = 1;
const PARTNER = 2;
const BYSTANDER = 3;

function renderLiaise(seat: number, phase: string): string {
	const plan = {
		id: 9, plan_type: 'clandestinely_liaise', status: 'resolving', preparer_id: PREPARER, row_number: 5,
		preparation_notes: 'a quiet word', resolution_data: JSON.stringify({ liaise: { phase, partner_id: PARTNER } }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: [] as never,
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: PARTNER, display_name: 'sol' }, { id: BYSTANDER, display_name: 'mei' }] as never,
		rankings: [], currentPlayerID: seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: false, rollOutcome: null, activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(ClandestinelyLiaisePanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Clandestinely Liaise: "What can happen"', () => {
	const toggle = /<button[^>]*aria-expanded="false"[^>]*>[\s\S]*?What can happen/;

	it.each(['together_at_last', 'secrets_we_keep'])('is a collapsed row for both players in %s', (phase) => {
		for (const seat of [PREPARER, PARTNER]) expect(renderLiaise(seat, phase)).toMatch(toggle);
	});

	it.each(['together_at_last', 'secrets_we_keep', 'things_we_share'])('gives the bystander nothing to read in %s', (phase) => {
		expect(renderLiaise(BYSTANDER, phase)).not.toContain('What can happen');
	});

	it.each(['things_we_share', 'when_will_i_see_you_again'])('is not repeated in %s, where the options themselves are on screen', (phase) => {
		for (const seat of [PREPARER, PARTNER]) expect(renderLiaise(seat, phase)).not.toContain('What can happen');
	});
});
