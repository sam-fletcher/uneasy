import { describe, it, expect } from 'vitest';
import type { PrologueSheet, PrologueClaim, PlayerCardRow } from '$lib/api';
import { openCount, heldCardSet } from './choosing';

function sheet(type: PrologueSheet['type'], names: string[]): PrologueSheet {
	return {
		type,
		display_name: type,
		choice_asset_type: 'Holding',
		choices: names.map((name) => ({
			name,
			description: '',
			cards: [
				{ suit: 'H', value: 'A' },
				{ suit: 'D', value: 'K' },
			],
		})),
	};
}

function claim(sheet_type: PrologueSheet['type'], choice_name: string, player_id = 1): PrologueClaim {
	return { sheet_type, choice_name, player_id, turn_number: 1 };
}

describe('openCount', () => {
	it('counts all choices open when nothing is claimed', () => {
		const s = sheet('hailing_from', ['A', 'B', 'C']);
		expect(openCount(s, [])).toBe(3);
	});

	it('subtracts only claims on the matching sheet', () => {
		const s = sheet('titles', ['A', 'B', 'C']);
		const claims = [claim('titles', 'A'), claim('hailing_from', 'A')];
		expect(openCount(s, claims)).toBe(2);
	});

	it('reaches zero when every choice on the sheet is claimed', () => {
		const s = sheet('laws_rumors', ['A', 'B']);
		const claims = [claim('laws_rumors', 'A'), claim('laws_rumors', 'B')];
		expect(openCount(s, claims)).toBe(0);
	});
});

describe('heldCardSet', () => {
	it('is empty with no cards', () => {
		expect(heldCardSet([])).toEqual(new Set());
	});

	it('keys by suit::value, deduping cards held by different players', () => {
		const cards: PlayerCardRow[] = [
			{ id: 1, game_id: 1, player_id: 1, card_suit: 'H', card_value: 'A' },
			{ id: 2, game_id: 1, player_id: 2, card_suit: 'H', card_value: 'A' },
			{ id: 3, game_id: 1, player_id: 1, card_suit: 'D', card_value: 'K' },
		];
		expect(heldCardSet(cards)).toEqual(new Set(['H::A', 'D::K']));
	});
});
