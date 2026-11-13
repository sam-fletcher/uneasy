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
