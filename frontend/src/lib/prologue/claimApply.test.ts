import { describe, it, expect } from 'vitest';
import { applyCards, applyClaim, isApplicableClaim } from './claimApply';
import type { PlayerCardRow, PrologueClaim } from '$lib/api';

const card = (id: number, player_id: number, suit: PlayerCardRow['card_suit'], value: string): PlayerCardRow => ({
	id, game_id: 1, player_id, card_suit: suit, card_value: value,
});
const claim = (choice_name: string, player_id = 1, turn_number = 1): PrologueClaim => ({
	sheet_type: 'titles', choice_name, player_id, turn_number,
});

describe('applyClaim', () => {
	it('appends a new claim', () => {
		expect(applyClaim([claim('Lady')], claim('Lord', 2))).toEqual([claim('Lady'), claim('Lord', 2)]);
	});
	it('is idempotent for the same tile — WS echo and HTTP reply both carry it', () => {
		const once = applyClaim([], claim('Lady'));
		expect(applyClaim(once, claim('Lady'))).toEqual(once);
	});
	it('does not mutate its input', () => {
		const before = [claim('Lady')];
		applyClaim(before, claim('Lord'));
		expect(before).toHaveLength(1);
	});
});

describe('applyCards', () => {
	it('adds made cards and moves taken ones by suit+value', () => {
		const hand = [card(10, 1, 'C', 'K'), card(11, 2, 'D', 'Q')];
		// Player 3 makes S|A and takes D|Q from player 2.
		const next = applyCards(hand, [card(12, 3, 'S', 'A'), card(11, 3, 'D', 'Q')]);
		expect(next).toEqual([card(10, 1, 'C', 'K'), card(11, 3, 'D', 'Q'), card(12, 3, 'S', 'A')]);
	});
	it('re-applying the same rows changes nothing', () => {
		const once = applyCards([], [card(12, 3, 'S', 'A')]);
		expect(applyCards(once, [card(12, 3, 'S', 'A')])).toEqual(once);
	});
	it('returns the same array for an empty delta', () => {
		const hand = [card(10, 1, 'C', 'K')];
		expect(applyCards(hand, [])).toBe(hand);
	});
});

describe('isApplicableClaim', () => {
	it('accepts the new payload shape', () => {
		expect(isApplicableClaim({ player_id: 1, sheet_type: 'titles', choice_name: 'Lady', turn_number: 1, cards: [] })).toBe(true);
	});
	it('rejects a payload from a server that predates cards, so the caller reloads', () => {
		expect(isApplicableClaim({ player_id: 1, sheet_type: 'titles', choice_name: 'Lady', turn_number: 1 })).toBe(false);
		expect(isApplicableClaim(undefined)).toBe(false);
		expect(isApplicableClaim(null)).toBe(false);
	});
});
