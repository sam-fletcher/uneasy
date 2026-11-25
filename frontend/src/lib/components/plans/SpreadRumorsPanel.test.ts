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
		rankings: [], currentPlayerID: 1, focusPlayerID: null, isFocusPlayer: false,
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
		// The target asset is a fact fixed at preparation: the preparation line says it ("about
		// Target Artifact"). The consent screen below it is about the main character instead.
		const consent = body.slice(body.indexOf('Consent needed'));
		expect(consent).not.toContain('Target Artifact');
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
		rankings: [], currentPlayerID: 2, focusPlayerID: null, isFocusPlayer: false,
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

// The difficulty used to show only in the dice panel's line. It is the meter now, for
// everyone, until the dice land, and it says which of the plan's two readings it is:
// a rumor about a main character is as hard as its owner's esteem STATUS (6 minus rank),
// any other asset as hard as the preparer's own esteem RANK. The two run opposite ways,
// which is what the number alone could not tell a player.
describe('SpreadRumorsPanel difficulty, before the dice', () => {
	const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

	function renderPreRoll(opts: {
		seat: number; targetIsMainCharacter: boolean; outcome?: 'make' | 'mar' | null;
		/** This plan's choices are committed and another plan's roll is the one open on the table. */
		committedWithOtherRoll?: boolean;
	}): string {
		const plan = {
			id: 9, plan_type: 'spread_rumors', status: 'resolving', preparer_id: 1,
			target_asset_id: 20, preparation_notes: 'a rumor',
			resolution_data: JSON.stringify({
				spread_rumors: { pre_roll_done: false },
				...(opts.committedWithOtherRoll ? { choice_submitted: true } : {}),
			}),
		} as never;
		const ctx = {
			gameID: 1, currentRow: 5, endingMode: null, plans: [plan],
			assets: [asset(1, 1, 'Preparer Main', true), asset(20, 2, 'Target', opts.targetIsMainCharacter)],
			players: [{ id: 1, display_name: 'ravi' }, { id: 2, display_name: 'sol' }, { id: 3, display_name: 'mei' }] as never,
			rankings: [
				{ id: 1, game_id: 1, category: 'esteem', player_id: 1, rank: 3 },
				{ id: 2, game_id: 1, category: 'esteem', player_id: 2, rank: 2 },
			] as never,
			currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
			rollActive: false, rollOutcome: opts.outcome ?? null,
			activeRoll: (opts.committedWithOtherRoll ? { id: 8, plan_id: 77, outcome: null, difficulty: 2 } : null) as never,
			onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
			readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		} as PlanContext;
		return render(SpreadRumorsPanel, { props: { ctx, plan, mode: 'resolve' } }).body;
	}

	it('does not come back when another plan\'s roll opens after this plan\'s dice have landed', () => {
		const t = textOf(renderPreRoll({ seat: 3, targetIsMainCharacter: true, committedWithOtherRoll: true }));
		expect(t).not.toContain('1 easy');
	});

	it('about a main character: the owner\'s esteem status, for the preparer and for everyone else', () => {
		for (const seat of [1, 2, 3]) {
			const t = textOf(renderPreRoll({ seat, targetIsMainCharacter: true }));
			expect(t).toContain('Difficulty 4');
			expect(t).toContain('main character');
			expect(t).toContain('sol’s status on the esteem track: 6 minus rank 2 is 4');
		}
	});

	it('about any other asset: the preparer\'s own esteem rank', () => {
		for (const seat of [1, 3]) {
			const t = textOf(renderPreRoll({ seat, targetIsMainCharacter: false }));
			expect(t).toContain('Difficulty 3');
			expect(t).toContain('not a main character');
			expect(t).toContain('ravi, 3');
		}
	});

	it('is gone once the dice have landed', () => {
		expect(textOf(renderPreRoll({ seat: 3, targetIsMainCharacter: true, outcome: 'make' }))).not.toContain('1 easy');
	});
});
