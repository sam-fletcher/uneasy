import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import SpreadRumorsPanel from './SpreadRumorsPanel.svelte';
import type { PlanContext } from './types';

// On a mar the rumor that spreads is about the preparer, so break and leverage
// land on the preparer's main character. The preparer, who is the victim of a
// take, must be told that — not that the plan's target asset (owned by the
// player making the demand) is hit.

const asset = (id: number, owner_id: number, name: string, is_main_character = false) =>
	({ id, owner_id, name, is_main_character, is_destroyed: false, is_leveraged: false }) as never;

function renderAsPreparer(): string {
	const resolution = {
		spread_rumors: {
			pre_roll_done: true,
			rumor_text: 'A debt was never paid.',
			counter_rumor_text: 'The Duke hides a bastard.',
			pending_take_consent: {
				choices: ['take_asset', 'break_target', 'leverage_target'],
				result: 'mar', asset_ids: [3], victim_id: 1, requested_by: 2,
			},
		},
	};
	const plan = {
		id: 9, plan_type: 'spread_rumors', status: 'resolving', preparer_id: 1,
		target_asset_id: 20, preparation_notes: 'x', resolution_data: JSON.stringify(resolution),
	} as never;
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan],
		assets: [
			asset(1, 1, 'Preparer Main', true), asset(3, 1, 'Preparer Artifact'),
			asset(20, 2, 'Target Artifact'),
		],
		players: [{ id: 1, display_name: 'ravi' }, { id: 2, display_name: 'sol' }] as never,
		rankings: [], currentPlayerID: 1, isFocusPlayer: false,
		rollActive: false, rollOutcome: 'mar', activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(SpreadRumorsPanel, { props: { ctx, plan, mode: 'resolve' } }).body;
}

describe('SpreadRumorsPanel consent screen on a mar', () => {
	it('names the preparer’s main character, not the target asset', () => {
		const body = renderAsPreparer();
		expect(body).toContain('Consent needed');
		expect(body).toContain('also hits');
		expect(body).toContain('Preparer Main');
		expect(body).not.toContain('Target Artifact');
	});
});

// A mar committed with nothing picked is recorded as choice_submitted with an empty
// make_mar_choices; the panel must read it as "choices made" and not offer the picker again.
function renderZeroPick(committed: boolean): string {
	const resolution = {
		spread_rumors: { pre_roll_done: true, rumor_text: 'A debt was never paid.', counter_rumor_text: 'x' },
		...(committed ? { choice_submitted: true } : {}),
	};
	const plan = {
		id: 9, plan_type: 'spread_rumors', status: 'resolving', preparer_id: 1,
		target_asset_id: 20, preparation_notes: 'x', resolution_data: JSON.stringify(resolution),
	} as never;
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan],
		assets: [asset(1, 1, 'Preparer Main', true), asset(20, 2, 'Target Artifact')],
		players: [{ id: 1, display_name: 'ravi' }, { id: 2, display_name: 'sol' }] as never,
		rankings: [], currentPlayerID: 2, isFocusPlayer: false,
		rollActive: false, rollOutcome: 'mar',
		activeRoll: { id: 4, plan_id: 9, status: 'resolved' } as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(SpreadRumorsPanel, { props: { ctx, plan, mode: 'resolve' } }).body;
}

describe('SpreadRumorsPanel mar after a zero-pick commit', () => {
	it('shows the picker while nothing is committed', () => {
		expect(renderZeroPick(false)).toContain('Apply choices');
	});

	it('does not offer the picker again', () => {
		expect(renderZeroPick(true)).not.toContain('Apply choices');
	});
});
