import { describe, it, expect } from 'vitest';
import type { PrologueSheet, PrologueClaim, PlayerCardRow, Asset, Player } from '$lib/api';
import {
	openCount,
	cardHoldStates,
	ownedCardCount,
	sheetTrackProfile,
	stealPreview,
	trackLabel,
	assetTypeLabel,
} from './choosing';

function sheet(
	type: PrologueSheet['type'],
	names: string[],
	cards: PrologueSheet['choices'][number]['cards'] = [
		{ suit: 'H', value: 'A' },
		{ suit: 'D', value: 'K' },
	]
): PrologueSheet {
	return {
		type,
		display_name: type,
		choice_asset_type: 'Holding',
		choices: names.map((name) => ({ name, description: '', cards })),
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

describe('cardHoldStates', () => {
	const cards: PlayerCardRow[] = [
		{ id: 1, game_id: 1, player_id: 1, card_suit: 'H', card_value: 'A' },
		{ id: 2, game_id: 1, player_id: 2, card_suit: 'S', card_value: 'Q' },
		{ id: 3, game_id: 1, player_id: 1, card_suit: 'D', card_value: 'K' },
	];

	it('is empty with no cards', () => {
		expect(cardHoldStates([], 1).size).toBe(0);
	});

	it("marks the viewer's own cards mine and everyone else's steal", () => {
		const states = cardHoldStates(cards, 1);
		expect(states.get('H::A')).toBe('mine');
		expect(states.get('D::K')).toBe('mine');
		expect(states.get('S::Q')).toBe('steal');
	});

	it('flips with the viewer', () => {
		const states = cardHoldStates(cards, 2);
		expect(states.get('S::Q')).toBe('mine');
		expect(states.get('H::A')).toBe('steal');
	});

	// A spectator (or a not-yet-resolved viewer) owns nothing, so every held
	// card is somebody else's — never a dead slot.
	it('marks everything steal when there is no viewer', () => {
		const states = cardHoldStates(cards, null);
		expect([...states.values()]).toEqual(['steal', 'steal', 'steal']);
	});

	it('omits unheld cards entirely, so callers default them to fresh', () => {
		expect(cardHoldStates(cards, 1).has('C::2')).toBe(false);
	});
});

describe('ownedCardCount', () => {
	const choice = {
		name: 'The Monarch',
		description: '',
		cards: [
			{ suit: 'C', value: 'K' },
			{ suit: 'D', value: 'K' },
		],
	} as PrologueSheet['choices'][number];

	function states(...owned: string[]) {
		return new Map(owned.map((k) => [k, 'mine' as const]));
	}

	it('is zero when the viewer holds neither card', () => {
		expect(ownedCardCount(choice, new Map())).toBe(0);
	});

	// A steal is not waste — the tile still hands over a real asset.
	it('does not count cards held by other players', () => {
		expect(ownedCardCount(choice, new Map([['C::K', 'steal' as const]]))).toBe(0);
	});

	it('counts one dead card', () => {
		expect(ownedCardCount(choice, states('C::K'))).toBe(1);
	});

	it('counts both when the tile grants the viewer no card assets at all', () => {
		expect(ownedCardCount(choice, states('C::K', 'D::K'))).toBe(2);
	});
});

describe('sheetTrackProfile', () => {
	it('reports the tracks a sheet feeds, in track order', () => {
		const s = sheet('titles', ['A', 'B'], [
			{ suit: 'D', value: 'K' },
			{ suit: 'C', value: 'A' },
		]);
		expect(sheetTrackProfile(s).tracks).toEqual(['C', 'D']);
	});

	// The headline fact this helper exists for: every real sheet has a hole.
	it('reports the ranked suit no box on the sheet supplies', () => {
		const s = sheet('titles', ['A'], [
			{ suit: 'C', value: 'K' },
			{ suit: 'D', value: 'K' },
		]);
		expect(sheetTrackProfile(s).missing).toEqual(['S']);
	});

	it('flags a wild heart separately from the ranked tracks', () => {
		const withHeart = sheet('laws_rumors', ['A'], [
			{ suit: 'H', value: 'A' },
			{ suit: 'S', value: '2' },
		]);
		expect(sheetTrackProfile(withHeart).wild).toBe(true);
		expect(sheetTrackProfile(withHeart).tracks).toEqual(['S']);

		const noHeart = sheet('titles', ['A'], [
			{ suit: 'C', value: 'K' },
			{ suit: 'D', value: 'K' },
		]);
		expect(sheetTrackProfile(noHeart).wild).toBe(false);
	});

	it('unions across every box on the sheet, not just the first', () => {
		const s: PrologueSheet = {
			type: 'hailing_from',
			display_name: 'Hailing From',
			choice_asset_type: 'Holding',
			choices: [
				{ name: 'A', description: '', cards: [{ suit: 'S', value: '2' }, { suit: 'S', value: '3' }] },
				{ name: 'B', description: '', cards: [{ suit: 'D', value: '4' }, { suit: 'H', value: '5' }] },
			],
		};
		const p = sheetTrackProfile(s);
		expect(p.tracks).toEqual(['D', 'S']);
		expect(p.missing).toEqual(['C']);
		expect(p.wild).toBe(true);
	});
});

describe('suit meaning labels', () => {
	it('maps ranked suits to their track', () => {
		expect(trackLabel('C')).toBe('Power');
		expect(trackLabel('D')).toBe('Knowledge');
		expect(trackLabel('S')).toBe('Esteem');
	});

	// Hearts are declared as a suit during the hearts step; on their own they
	// rank nothing, so there is no track to name.
	it('gives the wild heart no track', () => {
		expect(trackLabel('H')).toBe('');
	});

	it('maps suits to the asset type they make, lowercased for running text', () => {
		expect(assetTypeLabel('C')).toBe('holding');
		expect(assetTypeLabel('D')).toBe('resource');
		expect(assetTypeLabel('S')).toBe('artifact');
		expect(assetTypeLabel('H')).toBe('peer');
		expect(assetTypeLabel('?')).toBe('asset');
	});
});

describe('stealPreview', () => {
	function player(id: number, name: string): Player {
		return {
			id,
			game_id: 1,
			account_id: id,
			display_name: name,
			joined_at: '2026-01-01T00:00:00Z',
			is_facilitator: false,
			token_color: null,
			seat_order: null,
		};
	}

	function asset(overrides: Partial<Asset> = {}): Asset {
		return {
			id: 1,
			name: 'Blood of Kings',
			linked_card_suit: null,
			linked_card_value: null,
			is_destroyed: false,
			...overrides,
		} as Asset;
	}

	const players = [player(1, 'alice'), player(2, 'carol')];

	it('returns null for a fresh card nobody holds', () => {
		const cards: PlayerCardRow[] = [];
		expect(stealPreview('H', 'K', cards, [], players)).toBeNull();
	});

	it('resolves the owner and the linked asset for a held card', () => {
		const cards: PlayerCardRow[] = [
			{ id: 1, game_id: 1, player_id: 2, card_suit: 'H', card_value: 'K' },
		];
		const assets = [asset({ linked_card_suit: 'H', linked_card_value: 'K' })];
		expect(stealPreview('H', 'K', cards, assets, players)).toEqual({
			ownerID: 2,
			ownerName: 'carol',
			assetName: 'Blood of Kings',
		});
	});

	it('falls back to owner-only wording when the linked asset is destroyed', () => {
		const cards: PlayerCardRow[] = [
			{ id: 1, game_id: 1, player_id: 2, card_suit: 'H', card_value: 'K' },
		];
		const assets = [
			asset({ linked_card_suit: 'H', linked_card_value: 'K', is_destroyed: true }),
		];
		expect(stealPreview('H', 'K', cards, assets, players)).toEqual({
			ownerID: 2,
			ownerName: 'carol',
			assetName: null,
		});
	});

	// Card pairs repeat across tiles, so a card the viewer already holds shows
	// up on tiles that are still open. Callers key off ownerID to avoid
	// offering a take from yourself.
	it('reports the viewer as the owner for a card they already hold', () => {
		const cards: PlayerCardRow[] = [
			{ id: 1, game_id: 1, player_id: 1, card_suit: 'H', card_value: 'K' },
		];
		const assets = [asset({ linked_card_suit: 'H', linked_card_value: 'K' })];
		expect(stealPreview('H', 'K', cards, assets, players)).toEqual({
			ownerID: 1,
			ownerName: 'alice',
			assetName: 'Blood of Kings',
		});
	});

	it('falls back to owner-only wording when no matching asset exists at all', () => {
		const cards: PlayerCardRow[] = [
			{ id: 1, game_id: 1, player_id: 2, card_suit: 'H', card_value: 'K' },
		];
		expect(stealPreview('H', 'K', cards, [], players)).toEqual({
			ownerID: 2,
			ownerName: 'carol',
			assetName: null,
		});
	});
});
