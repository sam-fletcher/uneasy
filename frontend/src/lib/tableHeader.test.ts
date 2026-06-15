import { describe, it, expect } from 'vitest';
import {
	rankTriplesByPlayer,
	topRanks,
	atRiskCountByPlayer,
	typingIndicatorLabel,
} from './tableHeader';
import type { Ranking, Asset, Marginalium, RankingCategory } from '$lib/api';

// Minimal builders; only the fields the helpers read matter.
function rank(player_id: number | null, category: RankingCategory, rank: number): Ranking {
	return { id: 0, game_id: 1, player_id, category, rank } as Ranking;
}
function marg(position: number, is_torn = false): Marginalium {
	return { id: position, asset_id: 1, position, text: '', is_torn } as Marginalium;
}
function asset(owner_id: number, marginalia: Marginalium[], is_destroyed = false): Asset {
	return { id: 1, owner_id, marginalia, is_destroyed } as Asset;
}

describe('rankTriplesByPlayer', () => {
	it('groups a player\'s three ranks into one triple', () => {
		const triples = rankTriplesByPlayer([
			rank(1, 'power', 2),
			rank(1, 'knowledge', 4),
			rank(1, 'esteem', 1),
		]);
		expect(triples.get(1)).toEqual({ power: 2, knowledge: 4, esteem: 1 });
	});

	it('leaves un-ranked tracks null', () => {
		const triples = rankTriplesByPlayer([rank(7, 'power', 3)]);
		expect(triples.get(7)).toEqual({ power: 3, knowledge: null, esteem: null });
	});

	it('skips placeholder rankings with a null player_id', () => {
		const triples = rankTriplesByPlayer([rank(null, 'power', 1), rank(2, 'power', 2)]);
		expect(triples.has(2)).toBe(true);
		expect(triples.size).toBe(1);
	});
});

describe('topRanks', () => {
	it('returns the lowest-numbered rank held by any player per track', () => {
		const triples = rankTriplesByPlayer([
			rank(1, 'power', 3),
			rank(2, 'power', 2),
			rank(1, 'esteem', 5),
		]);
		expect(topRanks(triples)).toEqual({ power: 2, knowledge: null, esteem: 5 });
	});

	it('is all-null for an empty map', () => {
		expect(topRanks(new Map())).toEqual({ power: null, knowledge: null, esteem: null });
	});
});

describe('atRiskCountByPlayer', () => {
	it('counts only needlessly-at-risk assets, grouped by owner', () => {
		const counts = atRiskCountByPlayer([
			asset(1, []), // new asset → at risk
			asset(1, [marg(1)]), // 1 intact + empty slots → at risk
			asset(1, [marg(1), marg(2)]), // 2 intact → safe
			asset(2, [], true), // destroyed → excluded
		]);
		expect(counts.get(1)).toBe(2);
		expect(counts.has(2)).toBe(false);
	});
});

describe('typingIndicatorLabel', () => {
	it('is empty with nobody typing', () => {
		expect(typingIndicatorLabel([])).toBe('');
	});
	it('names a single writer', () => {
		expect(typingIndicatorLabel(['Ada'])).toBe('Ada is writing…');
	});
	it('joins two writers', () => {
		expect(typingIndicatorLabel(['Ada', 'Bo'])).toBe('Ada and Bo are writing…');
	});
	it('collapses three or more to a generic label', () => {
		expect(typingIndicatorLabel(['Ada', 'Bo', 'Cy'])).toBe('Several people are writing…');
	});
});
