import { describe, it, expect } from 'vitest';
import type { Player } from '$lib/api';
import type { WaitingOnState } from './waitingOn';
import { stepHeading } from './stepHeading';
import { readHeading } from './stepHeadingTestKit';

// The heading is the one place the game says, in words, what it asks of whom: a
// who-line ("Your move · also alice", "Waiting on otto and Wren") over the step alone
// ("Aid or interfere"). It reads the same derivation as the clocks on the player row
// (lib/waitingOn.ts), so these tests pin the grammar only.

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
const named = (h: ReturnType<typeof stepHeading>) =>
	h?.who.map(w => ('player' in w ? w.player.display_name : w.text));

describe('stepHeading', () => {
	it('is the viewer’s own when the game waits on them, and the step stands alone', () => {
		const h = stepHeading(on([1], 'aid or interfere'), 1, players);
		expect(h).toMatchObject({ yours: true, label: 'Aid or interfere', who: [], more: false });
		expect(readHeading(h)).toBe('Your move. Aid or interfere');
	});

	it('names the one other player the game waits on', () => {
		const h = stepHeading(on([2], 'the roll'), 1, players);
		expect(h).toMatchObject({ yours: false, label: 'The roll', more: false });
		expect(named(h)).toEqual(['otto']);
		expect(readHeading(h)).toBe('Waiting on otto. The roll');
	});

	it('names two others, with "and" before the last', () => {
		const h = stepHeading(on([2, 3], 'difficulty vote'), 1, players);
		expect(named(h)).toEqual(['otto', 'Wren']);
		expect(readHeading(h)).toBe('Waiting on otto and Wren. Difficulty vote');
	});

	it('names two and then "and others" at exactly three owed, so the line stays short', () => {
		const h = stepHeading(on([2, 3, 4], 'how will the game end?'), 1, players);
		expect(named(h)).toEqual(['otto', 'Wren']);
		expect(h?.more).toBe(true);
		expect(readHeading(h)).toBe('Waiting on otto, Wren and others. How will the game end?');
	});

	it('does the same past three', () => {
		const h = stepHeading(on([2, 3, 4, 5], 'how will the game end?'), 1, players);
		expect(named(h)).toEqual(['otto', 'Wren']);
		expect(readHeading(h)).toBe('Waiting on otto, Wren and others. How will the game end?');
	});

	it('is the viewer’s and names the others owed beside them', () => {
		const h = stepHeading(on([2, 1], 'the delay reveal'), 1, players);
		expect(h).toMatchObject({ yours: true, more: false });
		expect(named(h)).toEqual(['otto']);
		expect(readHeading(h)).toBe('Your move · also otto. The delay reveal');
	});

	it('never names the viewer on their own line, and caps the others at two', () => {
		const three = stepHeading(on([2, 1, 3, 4], 'how will the game end?'), 1, players);
		expect(named(three)).toEqual(['otto', 'Wren']);
		expect(three).toMatchObject({ yours: true, more: true });
		expect(readHeading(three)).toBe('Your move · also otto, Wren and others. How will the game end?');
		// Exactly two others stay named in full.
		expect(readHeading(stepHeading(on([2, 1, 3], 'the roll'), 1, players))).toBe('Your move · also otto and Wren. The roll');
	});

	it('treats "everyone" as including the viewer', () => {
		const h = stepHeading({ waitees: [{ kind: 'everyone' }], stepLabel: 'vote' }, 1, players);
		expect(h?.yours).toBe(true);
		expect(h?.who).toEqual([]);
	});

	it('says a label waitee as it is written (the lobby’s "1 more player to join")', () => {
		const h = stepHeading(
			{ waitees: [{ kind: 'label', text: '1 more player to join' }], stepLabel: 'gathering players' },
			1,
			players,
		);
		expect(readHeading(h)).toBe('Waiting on 1 more player to join. Gathering players');
	});

	// A label waitee is free text, not a player: it is never counted toward the cap, or
	// a lobby's waiting players would read "and others" for a count that is not players.
	it('does not count a label waitee as a player', () => {
		const h = stepHeading(
			{ waitees: [...on([2, 3]).waitees, { kind: 'label', text: '1 more player to join' }], stepLabel: 'gathering players' },
			1,
			players,
		);
		expect(h).toMatchObject({ more: false });
		expect(readHeading(h)).toBe('Waiting on otto, Wren and 1 more player to join. Gathering players');
	});

	it('is only the who-line when there is no step label', () => {
		expect(readHeading(stepHeading(on([1]), 1, players))).toBe('Your move');
		expect(readHeading(stepHeading(on([2]), 1, players))).toBe('Waiting on otto');
	});

	it('is nothing when the game waits on no one', () => {
		expect(stepHeading({ waitees: [] }, 1, players)).toBeNull();
	});

	it('is never "yours" for a viewer who is not seated', () => {
		expect(stepHeading(on([2], 'the roll'), null, players)?.yours).toBe(false);
	});

	it('says "someone" for a player it can’t find, rather than a blank', () => {
		expect(readHeading(stepHeading(on([99], 'the roll'), 1, players))).toBe('Waiting on someone. The roll');
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
				// "Waiting on otto. <A step>": a who-line and a step, never a bare who-line.
				expect(readHeading(h)).toMatch(/^Waiting on otto\. \S/);
			} else {
				expect(h).toBeNull();
			}
		});
	}
});
