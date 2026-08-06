import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import type {
	PrologueSheet,
	PrologueClaim,
	PrologueTrack,
	PlayerCardRow,
	Asset,
	Player,
} from '$lib/api';
import {
	cardHoldStates,
	cardHolders,
	cardWeight,
	codeForTrack,
	labelForTrack,
	ownedCardCount,
	sheetTrackProfile,
	spentByCategory,
	stealPreview,
	trackLabel,
	trackCode,
	assetTypeLabel,
	assetTypeFor,
	MAX_CARD_WEIGHT,
	SUIT_MEANINGS,
} from './choosing';
import { cardRank } from './refund';

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

describe('spentByCategory', () => {
	it('has nothing spent before the viewer claims anything', () => {
		const s = spentByCategory([], 1);
		expect(s.total).toBe(0);
		expect(s.bySheet.size).toBe(0);
	});

	it('ignores other players’ claims', () => {
		const claims = [claim('titles', 'A', 2), claim('titles', 'B', 3)];
		expect(spentByCategory(claims, 1).total).toBe(0);
	});

	// The misread this replaces: three panels each showing "12 open" read as a
	// checklist, one pick per category. Three pips on one sheet is a legal
	// character ("if you want three titles, take three titles").
	it('lets all three land on one category', () => {
		const claims = [
			claim('titles', 'A'),
			claim('titles', 'B'),
			claim('titles', 'C'),
		];
		const s = spentByCategory(claims, 1);
		expect(s.total).toBe(3);
		expect(s.bySheet.get('titles')).toBe(3);
		expect(s.bySheet.has('hailing_from')).toBe(false);
	});

	it('splits across categories and always adds back to the total', () => {
		const claims = [
			claim('titles', 'A'),
			claim('laws_rumors', 'B'),
			claim('titles', 'C', 2),
		];
		const s = spentByCategory(claims, 1);
		expect(s.bySheet.get('titles')).toBe(1);
		expect(s.bySheet.get('laws_rumors')).toBe(1);
		expect([...s.bySheet.values()].reduce((a, b) => a + b, 0)).toBe(s.total);
	});

	// A spectator holds no choices at all, so neither home draws a pip.
	it('is empty with no viewer', () => {
		const s = spentByCategory([claim('titles', 'A')], null);
		expect(s.total).toBe(0);
		expect(s.bySheet.size).toBe(0);
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

	// The codes replace the suits on screen (Round 2, decision 1), so every
	// suit needs one — including the heart, which gets a word inside the same
	// pattern rather than a glyph outside it.
	it('maps every suit to a three-letter track code', () => {
		expect(trackCode('C')).toBe('POW');
		expect(trackCode('D')).toBe('KNO');
		expect(trackCode('S')).toBe('EST');
		expect(trackCode('H')).toBe('ANY');
		expect(trackCode('?')).toBe('');
	});

	it('starts each ranked code with the first letters of its own track', () => {
		for (const suit of ['C', 'D', 'S']) {
			expect(trackLabel(suit).toUpperCase().startsWith(trackCode(suit))).toBe(true);
		}
	});

	// The declare step addresses the same table by track rather than by suit —
	// it holds a step name, not a card. The two lookups have to land on the
	// same row or a locked ANY card would name a different track than the
	// board column it was spent on.
	it('maps every track to the same code its suit gets', () => {
		expect(codeForTrack('power')).toBe(trackCode('C'));
		expect(codeForTrack('knowledge')).toBe(trackCode('D'));
		expect(codeForTrack('esteem')).toBe(trackCode('S'));
	});

	it('maps every track to its title-case name', () => {
		expect(labelForTrack('power')).toBe('Power');
		expect(labelForTrack('knowledge')).toBe('Knowledge');
		expect(labelForTrack('esteem')).toBe('Esteem');
	});

	// The lookup lowercases SUIT_MEANINGS' title-case `track` to match the
	// API's lowercase PrologueTrack. Guard the join itself: retitle a row and
	// both helpers would silently return '' rather than failing loudly.
	it('resolves every ranked row from its own track name', () => {
		for (const m of SUIT_MEANINGS) {
			if (!m.track) continue;
			const track = m.track.toLowerCase() as PrologueTrack;
			expect(codeForTrack(track)).toBe(m.code);
			expect(labelForTrack(track)).toBe(m.track);
		}
	});

	it('maps suits to the icon/API asset-type union', () => {
		expect(assetTypeFor('C')).toBe('holding');
		expect(assetTypeFor('D')).toBe('resource');
		expect(assetTypeFor('S')).toBe('artifact');
		expect(assetTypeFor('H')).toBe('peer');
	});

	// assetTypeFor casts SUIT_MEANINGS' prose label into the union rather than
	// keeping a second table that could drift from it. This is what makes the
	// cast honest: rename a row's assetType to something AssetTypeIcon can't
	// draw and the guard fails here, not silently as a resource icon on screen.
	it('keeps every prose asset type inside the union the icon can draw', () => {
		const union = ['peer', 'holding', 'artifact', 'resource'];
		for (const m of SUIT_MEANINGS) {
			expect(union).toContain(m.assetType.toLowerCase());
			expect(assetTypeFor(m.suit)).toBe(m.assetType.toLowerCase());
		}
	});
});

describe('cardWeight', () => {
	it('lands the prologue deck on 1–4 with no gaps', () => {
		expect(cardWeight('J')).toBe(1);
		expect(cardWeight('Q')).toBe(2);
		expect(cardWeight('K')).toBe(3);
		expect(cardWeight('A')).toBe(4);
	});

	// The number exists to explain a tie-break, so it must agree with the
	// ordering that actually breaks ties (refund.ts's cardRank, mirroring
	// game/prologue_refund.go). Heavier card ⇒ heavier weight, always.
	it('orders the same way the tie-break does', () => {
		const deck = ['J', 'Q', 'K', 'A'];
		for (const a of deck) {
			for (const b of deck) {
				expect(cardWeight(a) > cardWeight(b)).toBe(cardRank(a) > cardRank(b));
			}
		}
	});

	// Card.Value is documented as accepting "A","2"–"10","J","Q","K" in
	// general even though the prologue sheets only ever use the four faces.
	// Degrade to the bottom segment; never throw, and never draw a meter with
	// zero or five segments lit.
	it('degrades rather than throwing on values the prologue never deals', () => {
		for (const v of ['10', '2', '7', '', 'joker', 'K ']) {
			expect(cardWeight(v)).toBeGreaterThanOrEqual(1);
			expect(cardWeight(v)).toBeLessThanOrEqual(MAX_CARD_WEIGHT);
		}
	});

	/*
	 * The guard the plan asks for (Round 2, §2b): every value in the *real*
	 * sheet data has to land inside 1–MAX_CARD_WEIGHT, distinctly. The data
	 * is Go constants (there is no TS copy — the sheets arrive over the API),
	 * so the test reads the source of truth directly. If a "10" or a "2" ever
	 * enters a card pair, the meter silently stops distinguishing it from a
	 * jack, and this fails first.
	 */
	it('covers every value in the real sheet data', () => {
		const goSrc = join(__dirname, '..', '..', '..', '..', 'game', 'prologue_sheets.go');
		const src = readFileSync(goSrc, 'utf8');
		const values = [...src.matchAll(/\{Suit[A-Za-z]+,\s*"([^"]+)"\}/g)].map((m) => m[1]);

		// 36 boxes × 2 cards. A regex that matched nothing would otherwise
		// make this whole test a no-op.
		expect(values.length).toBe(72);

		const seen = new Set(values);
		for (const v of seen) {
			expect(cardRank(v), `unknown card value ${v}`).toBeGreaterThan(10);
			expect(cardWeight(v)).toBeGreaterThanOrEqual(1);
			expect(cardWeight(v)).toBeLessThanOrEqual(MAX_CARD_WEIGHT);
		}
		expect(new Set([...seen].map(cardWeight)).size).toBe(seen.size);
	});
});

describe('cardHolders', () => {
	const cards: PlayerCardRow[] = [
		{ id: 1, game_id: 1, player_id: 1, card_suit: 'H', card_value: 'A' },
		{ id: 2, game_id: 1, player_id: 2, card_suit: 'S', card_value: 'Q' },
	];

	it('keys holders the same way cardHoldStates keys states', () => {
		const holders = cardHolders(cards);
		const states = cardHoldStates(cards, 1);
		expect([...holders.keys()].sort()).toEqual([...states.keys()].sort());
		expect(holders.get('S::Q')).toBe(2);
	});

	// Absent, not null: an unheld card has no corner dot at all, because
	// claiming it makes a new asset rather than taking one.
	it('omits cards nobody holds', () => {
		expect(cardHolders(cards).has('C::K')).toBe(false);
		expect(cardHolders([]).size).toBe(0);
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
