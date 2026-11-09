import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import DiceRollPanel from './DiceRollPanel.svelte';
import type { DiceRollDie, RollIntent, RollParticipant } from '$lib/api';

// The footer tally counts players by side, in the same unit as the
// "N of M ready" beside it. It once counted dice in each pool, so the
// actor's two base dice read as "2 aid" and a player who had picked
// interfere but not yet committed a die read as "0 interfere".

const ACTOR = 2;
const players = [
	{ id: 1, display_name: 'otto' },
	{ id: 2, display_name: 'wren' },
	{ id: 3, display_name: 'pia' },
	{ id: 4, display_name: 'quin' },
] as never;
const playerNameMap = new Map([[1, 'otto'], [2, 'wren'], [3, 'pia'], [4, 'quin']]);

let nextDie = 1;
const die = (player_id: number, extra: Partial<DiceRollDie> = {}): DiceRollDie =>
	({
		id: nextDie++, roll_id: 9, player_id, is_interference: false,
		leveraged_asset_id: null, face: null, is_cancelled: false,
		cancelled_by_die_id: null, ...extra,
	}) as DiceRollDie;
const baseDice = () => [die(ACTOR), die(ACTOR)];

const participant = (player_id: number, intent: RollIntent | null, is_ready = false) =>
	({ roll_id: 9, player_id, intent, is_ready }) as RollParticipant;

function footer(participants: RollParticipant[], dice: DiceRollDie[]): string {
	const { body } = render(DiceRollPanel, {
		props: {
			roll: {
				id: 9, game_id: 1, plan_id: 1, row_number: 5, is_shake_up: false,
				actor_id: ACTOR, difficulty: 2, adjusted_difficulty: null,
				result: null, outcome: null, stage: 'leverage',
			} as never,
			dice,
			votes: [],
			participants,
			bankedDice: [],
			assets: [],
			currentPlayerID: 4,
			players,
			playerNameMap,
		},
	});
	const m = body.match(/<p class="footer-summary[^"]*">([\s\S]*?)<\/p>/);
	if (!m) throw new Error('no footer summary rendered');
	return m[1].replace(/<!--[\s\S]*?-->/g, '').replace(/\s+/g, ' ').trim();
}

describe('DiceRollPanel footer tally', () => {
	it('counts a side picked before any die is committed', () => {
		const text = footer(
			[participant(1, 'interfere'), participant(ACTOR, 'aid'), participant(3, 'aid'), participant(4, 'aid')],
			baseDice(),
		);
		expect(text).toBe('3 aid · 1 interfere · 0 of 4 ready');
	});

	it('counts players, not dice, once dice are committed', () => {
		const text = footer(
			[participant(1, 'interfere', true), participant(ACTOR, 'aid'), participant(3, 'aid')],
			[
				...baseDice(),
				die(3, { leveraged_asset_id: 50 }), die(3, { leveraged_asset_id: 51 }),
				die(1, { is_interference: true, leveraged_asset_id: 60 }),
				die(1, { is_interference: true, leveraged_asset_id: 61 }),
				die(1, { is_interference: true, leveraged_asset_id: 62 }),
			],
		);
		expect(text).toBe('2 aid · 1 interfere · 1 of 3 ready');
	});

	it('counts the actor as aid and an undecided player on neither side', () => {
		const text = footer(
			[participant(1, null), participant(ACTOR, null), participant(3, null, true)],
			baseDice(),
		);
		expect(text).toBe('1 aid · 0 interfere · 1 of 3 ready');
	});
});
