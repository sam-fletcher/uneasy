import { describe, it, expect } from 'vitest';
import type { Player } from '$lib/api';
import type { WaitingOnState } from './waitingOn';
import { stepHeading } from './stepHeading';

// The heading is the one place the game says, in words, what it asks of whom.
// "Your move — …" when the viewer is among the people it waits on, "Waiting on
// otto — …" when it is someone else. It reads the same derivation as the clocks
// on the player row (lib/waitingOn.ts), so these tests pin the grammar only.

const players = [
	{ id: 1, display_name: 'Ash' },
	{ id: 2, display_name: 'otto' },
	{ id: 3, display_name: 'Wren' },
	{ id: 4, display_name: 'Mira' },
	{ id: 5, display_name: 'Tamsin' },
] as Player[];

const on = (ids: number[], stepLabel?: string): WaitingOnState => ({
	waitees: ids.map(playerID => ({ kind: 'player' as const, playerID })),
	stepLabel,
});

describe('stepHeading', () => {
	it('is the viewer’s own when the game waits on them', () => {
		const h = stepHeading(on([1], 'aid or interfere'), 1, players);
		expect(h).toEqual({ yours: true, text: 'Your move — aid or interfere' });
	});

	it('names the one other player the game waits on', () => {
		const h = stepHeading(on([2], 'the roll'), 1, players);
		expect(h).toEqual({ yours: false, text: 'Waiting on otto — the roll' });
	});

	it('lists two or three others, with "and" before the last', () => {
		expect(stepHeading(on([2, 3], 'difficulty vote'), 1, players)?.text)
			.toBe('Waiting on otto and Wren — difficulty vote');
		expect(stepHeading(on([2, 3, 4], 'everyone chooses'), 1, players)?.text)
			.toBe('Waiting on otto, Wren and Mira — everyone chooses');
	});

	it('counts instead of listing past three, so the heading stays a line or two', () => {
		const h = stepHeading(on([2, 3, 4, 5], 'how will the game end?'), 1, players);
		expect(h?.text).toBe('Waiting on 4 players — how will the game end?');
	});

	it('is the viewer’s and names no one else when others are owed too', () => {
		// The clocks on the other chips say who else is being waited on.
		const h = stepHeading(on([2, 1, 3], 'how will the game end?'), 1, players);
		expect(h).toEqual({ yours: true, text: 'Your move — how will the game end?' });
	});

	it('treats "everyone" as including the viewer', () => {
		const h = stepHeading({ waitees: [{ kind: 'everyone' }], stepLabel: 'vote' }, 1, players);
		expect(h?.yours).toBe(true);
	});

	it('says a label waitee as it is written (the lobby’s "1 more player to join")', () => {
		const h = stepHeading(
			{ waitees: [{ kind: 'label', text: '1 more player to join' }], stepLabel: 'gathering players' },
			1,
			players,
		);
		expect(h?.text).toBe('Waiting on 1 more player to join — gathering players');
	});

	// A label waitee is free text, not a player: it is never counted toward the
	// three-name cap, or two of them would read "2 players".
	it('does not count a label waitee as a player', () => {
		const three = [
			{ kind: 'player' as const, playerID: 2 },
			{ kind: 'player' as const, playerID: 3 },
			{ kind: 'player' as const, playerID: 4 },
		];
		const h = stepHeading(
			{ waitees: [...three, { kind: 'label', text: '1 more player to join' }], stepLabel: 'gathering players' },
			1,
			players,
		);
		expect(h?.text).toBe('Waiting on otto, Wren, Mira and 1 more player to join — gathering players');
	});

	it('drops the dash when there is no step label', () => {
		expect(stepHeading(on([1]), 1, players)?.text).toBe('Your move');
		expect(stepHeading(on([2]), 1, players)?.text).toBe('Waiting on otto');
	});

	it('is nothing when the game waits on no one', () => {
		expect(stepHeading({ waitees: [] }, 1, players)).toBeNull();
	});

	it('is never "yours" for a viewer who is not seated', () => {
		expect(stepHeading(on([2], 'the roll'), null, players)?.yours).toBe(false);
	});

	it('says "someone" for a player it can’t find, rather than a blank', () => {
		expect(stepHeading(on([99], 'the roll'), 1, players)?.text).toBe('Waiting on someone — the roll');
	});
});

// ── Every state the old bar covered has a heading ────────────────────────────
// The Waiting On bar was drawn by the page and always showed. The heading is drawn
// by whichever surface holds the open step, with the page as the default, so what
// must hold here is that the WORDS exist for every state: a kind with no actor is
// the only one allowed to say nothing. The record is typed, so a row state added
// to the server without a line here fails the type check, not a code review.

import type { RowStateKind } from '$lib/api';
import { mainEventWaitingOn } from './waitingOn';

describe('every row state the bar covered has a heading', () => {
	// true: names whoever the server says must act. false: nobody is owed anything.
	const HEADED: Record<RowStateKind, boolean> = {
		phase_not_main_event: false,
		await_dice_roll: true,
		await_endgame_vote: true,
		await_surrender_claim: true,
		await_battle_cost: true,
		await_main_character_choice: true,
		await_delay_reveal: true,
		plan_resolving: true,
		plan_pending: true,
		await_demand_counter: true,
		await_demand_draft_pick: true,
		await_demand_leverage: true,
		await_demand_retarget: true,
		await_plan_reaim: true,
		await_festivity_guest_turn: true,
		await_festivity_challenge_response: true,
		await_duel_staking: true,
		await_duel_bout: true,
		await_take_consent: true,
		await_question_answer: true,
		liaise_resolving: true,
		await_courtier_response: true,
		await_introductions_marginalia: true,
		await_chronicle_choices: true,
		await_counter_propaganda: true,
		finale_row_complete: false,
		scene_active: true,
		post_scene_action: true,
		scene_setting: true,
	};

	for (const [kind, headed] of Object.entries(HEADED) as [RowStateKind, boolean][]) {
		it(`${kind} ${headed ? 'is headed, with a step in it' : 'says nothing, because nobody is owed anything'}`, () => {
			const state = mainEventWaitingOn({
				rowState: { kind, acting_player_ids: [2] },
				activeRoll: null,
				maxRefresh: 1,
				currentPlayerID: 1,
			});
			const h = stepHeading(state, 1, players);
			if (headed) {
				expect(h, `${kind} has no heading`).not.toBeNull();
				// "Waiting on otto — <a step>": a lead and a phrase, never a bare lead.
				expect(h!.text).toMatch(/^Waiting on otto — \S/);
			} else {
				expect(h).toBeNull();
			}
		});
	}
});
