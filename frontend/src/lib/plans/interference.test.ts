import { describe, expect, it } from 'vitest';
import type { DiceRollDie, Ranking } from '$lib/api';
import { interferers, interferenceLine, counterPropLine, type Interferer } from './interference';

// Spread Propaganda's mar option "the top interferer spreads propaganda of their
// own" goes to whoever leveraged the most assets against the roll. The picker names
// them and shows the count, working it out from the dice the same way the server
// does (most interference dice, ties to the better esteem rank), so what the preparer
// reads is who will actually be asked.

const die = (player: number, over: Partial<DiceRollDie> = {}): DiceRollDie => ({
	id: Math.random(), roll_id: 1, player_id: player, is_interference: true, leveraged_asset_id: 1,
	face: 3, is_cancelled: false, cancelled_by_die_id: null, ...over,
});
const esteem = (player: number, rank: number) =>
	({ id: player, game_id: 1, player_id: player, category: 'esteem', rank }) as Ranking;
/** A tally row as interferers() returns it, for the line tests. */
const row = (playerID: number, dice: number, esteemRank: number | null = null): Interferer =>
	({ playerID, dice, esteemRank });
const name = (id: number) => ({ 2: 'sol', 3: 'mel', 4: 'kit' })[id as 2 | 3 | 4] ?? '?';

describe('interferers', () => {
	it('is empty when nobody interfered', () => {
		expect(interferers([die(1, { is_interference: false })], [])).toEqual([]);
		expect(interferers([], [])).toEqual([]);
	});

	it('counts interference dice only, most first, and counts a cancelled die too', () => {
		const dice = [
			die(2), die(2, { is_cancelled: true }), die(2),
			die(3),
			die(1, { is_interference: false }), die(1, { is_interference: false }),
		];
		expect(interferers(dice, [])).toEqual([row(2, 3), row(3, 1)]);
	});

	it('breaks a tie by the better esteem rank, as the server does', () => {
		const dice = [die(2), die(3)];
		// Rank 1 is the highest: mel outranks sol.
		expect(interferers(dice, [esteem(2, 3), esteem(3, 1)]).map(i => i.playerID)).toEqual([3, 2]);
		expect(interferers(dice, [esteem(2, 1), esteem(3, 3)]).map(i => i.playerID)).toEqual([2, 3]);
	});

	it('puts a player with no rank behind the ranked ones, then falls back to the lower id', () => {
		const dice = [die(4), die(3), die(2)];
		expect(interferers(dice, [esteem(4, 2)]).map(i => i.playerID)).toEqual([4, 2, 3]);
	});
});

describe('interferenceLine', () => {
	it('is empty when nobody interfered, so the option says nothing it cannot back', () => {
		expect(interferenceLine([], name)).toBe('');
	});

	it('names a lone interferer and their dice', () => {
		expect(interferenceLine([row(2, 3)], name)).toBe('Top interferer: sol, with 3 dice.');
		expect(interferenceLine([row(2, 1)], name)).toBe('Top interferer: sol, with 1 die.');
	});

	it('shows the rest of the tally behind the top', () => {
		expect(interferenceLine([row(2, 3), row(3, 1), row(4, 1)], name))
			.toBe('Top interferer: sol, with 3 dice. Also interfered: mel, with 1 die; kit, with 1 die.');
	});

	it('says when the top is a tie settled by esteem', () => {
		expect(interferenceLine([row(3, 2, 1), row(2, 2, 3)], name))
			.toBe('Top interferer: mel, who ties sol at 2 dice but ranks higher on esteem.');
		// Ranked against unranked is still esteem that settled it.
		expect(interferenceLine([row(3, 2, 1), row(2, 2, null)], name))
			.toBe('Top interferer: mel, who ties sol at 2 dice but ranks higher on esteem.');
	});

	it('names every player tied with the top', () => {
		expect(interferenceLine([row(3, 2, 1), row(2, 2, 2), row(4, 2, 3)], name))
			.toBe('Top interferer: mel, who ties sol and kit at 2 dice but ranks higher on esteem.');
	});

	it('does not call a tie between unranked players an esteem ranking: the player id settled it', () => {
		const t = interferenceLine([row(2, 2), row(3, 2)], name);
		expect(t).toBe('Top interferer: sol, who ties mel at 2 dice and goes first on the tie-break.');
		expect(t).not.toContain('esteem');
	});

	it('keeps the rest of the tally after a tie', () => {
		expect(interferenceLine([row(3, 2, 1), row(2, 2, 3), row(4, 1, 2)], name))
			.toBe('Top interferer: mel, who ties sol at 2 dice but ranks higher on esteem. Also interfered: kit, with 1 die.');
	});
});

describe('counterPropLine', () => {
	const dice = [die(2), die(2), die(3)];
	const nameOf = name;

	it('shows the tally on a plan whose counter-propaganda option is open', () => {
		expect(counterPropLine({ isCounter: false, dice, rankings: [], nameOf }))
			.toBe('Top interferer: sol, with 2 dice. Also interfered: mel, with 1 die.');
	});

	it('shows nothing on a counter-propaganda plan, where the option is shut', () => {
		expect(counterPropLine({ isCounter: true, dice, rankings: [], nameOf })).toBe('');
	});

	it('shows nothing until the dice have loaded', () => {
		expect(counterPropLine({ isCounter: false, dice: null, rankings: [], nameOf })).toBe('');
	});
});
